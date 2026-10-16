---
layout: post
title: "Two goroutines, one receive: a leak in the k3s tunnel proxy"
date: 2026-10-16
---

k3s leaked a goroutine, its 32 KiB buffer and both connections whenever the two copy directions of a tunnel connection failed at about the same time. The reporter counted 254 stuck goroutines after about 20.5 hours on one single-node server. I sent the fix ([PR #14811](https://github.com/k3s-io/k3s/pull/14811), merged 2026-10-15): it deletes a `bool` and a method, and gives a channel a buffer of 2. @bheemareddy reported the bug in [issue #14787](https://github.com/k3s-io/k3s/issues/14787), with the cause, a standalone reproducer and a `sync.Once` fix.

## Which proxy

It is `Proxy(lconn, rconn io.ReadWriteCloser) error` in `pkg/daemons/control/proxy/proxy.go`. It copies bytes both ways between two connections, one goroutine and one 32 KiB buffer per direction. The caller is `TunnelServer.serveConnect` in `pkg/daemons/control/tunnel.go`. It answers HTTP `CONNECT`: dial the backend, hijack the client connection, call `proxy.Proxy`. It serves the apiserver egress selector, the tunnel the apiserver uses to reach kubelets ([k3s docs](https://docs.k3s.io/networking/basic-network-options)) and, in the reporter's setup, aggregated APIServices.

## The symptom

The issue has a goroutine dump from that server. Every leaked goroutine had the same stack, parked in `chan send`; one showed as `goroutine 17335244 [chan send, 467 minutes]:`. The count grew by roughly 11 per hour. The server log showed lines like `Proxy error: write failed: ... write: connection reset by peer`. I did not find the leak; these numbers are the report's.

## Why it never returned

The code before the fix ([lines 26-40](https://github.com/k3s-io/k3s/blob/22b0fb2309ab26e37d5ce93a3c70ada4e49a99ab/pkg/daemons/control/proxy/proxy.go#L26-L40)):

```go
	go p.pipe(p.lconn, p.rconn)
	go p.pipe(p.rconn, p.lconn)
	return <-p.errc
}

func (p *proxy) err(err error) {
	if p.done {
		return
	}
	if !errors.Is(err, io.EOF) {
		logrus.Warnf("Proxy error: %v", err)
	}
	p.done = true
	p.errc <- err
}
```

`errc` was unbuffered and `Proxy` receives from it once. `done` was meant to keep a second goroutine from sending, but it is a plain bool written by both goroutines. Both can pass `if p.done` before either sets it. The `Warnf` call sits in that gap.

The goroutine that leaks is not stuck in `Read` or `Write`. Those already failed. It is parked on `p.errc <- err`. The [Go spec](https://go.dev/ref/spec#Send_statements) says "A send on an unbuffered channel can proceed if a receiver is ready." `Proxy` has returned, so none ever is. The deferred `Close` calls wake a goroutine blocked in `Read` or `Write`, not one blocked on a channel. Per the PR description, it holds its buffer and both connections.

## The fix

From the merged diff of `pkg/daemons/control/proxy/proxy.go` (excerpt):

```diff
-		errc:  make(chan error),
+		errc:  make(chan error, 2), // one send per pipe goroutine, so neither can block
 	}
 
 	defer p.rconn.Close()
 	defer p.lconn.Close()
 	go p.pipe(p.lconn, p.rconn)
 	go p.pipe(p.rconn, p.lconn)
-	return <-p.errc
-}
-
-func (p *proxy) err(err error) {
-	if p.done {
-		return
-	}
-	if !errors.Is(err, io.EOF) {
+	err := <-p.errc
+	if !isConnTeardown(err) {
 		logrus.Warnf("Proxy error: %v", err)
 	}
-	p.done = true
-	p.errc <- err
+	return err
+}
+
+// isConnTeardown reports errors that are expected when one side closes or resets the connection.
+func isConnTeardown(err error) bool {
+	return errors.Is(err, io.EOF) ||
+		errors.Is(err, net.ErrClosed) ||
+		errors.Is(err, syscall.ECONNRESET) ||
+		errors.Is(err, syscall.EPIPE)
 }
```

In `pipe`, `p.err(errors.WithMessage(err, "read failed"))` became `p.errc <- err`, and the same for the write. Two slots for two senders means no send can block, whatever `Proxy` does. The guard is gone, so the race is gone. I kept "return on the first error" instead of waiting for both directions as remotedialer does. Per the PR description, that way `Proxy` cannot hang on a connection whose `Close` does not unblock a pending `Read`.

## What review changed

- @bheemareddy commented inline that the warning is noisy and should be debug, with a sketch that used a helper named `isConnTeardown`. Their own PR, #14810, took the remotedialer approach. They called this one "smaller fix and easily back portable", and @brandond went with it.
- @brandond liked filtering `ECONNRESET` and `EPIPE` and suggested dropping the `errors.WithMessage` wrap. They gave no reason. Their `isConnTeardown` used a type assertion, `err.(syscall.Errno)`.
- I replied with `errors.As`, because a TCP error is not a bare `syscall.Errno`. That part was my suggestion, not the reviewer's. On a loopback reset I got a `*net.OpError`, the assertion gave `false`, and `errors.Is(err, syscall.ECONNRESET)` gave `true`.
- Then brandond said the intent was to ignore normal closure, so these errors should not be logged even at debug. They pushed that and dropped the log-level test. The merged helper uses `errors.Is`, and it also keeps `net.ErrClosed`, which I had dropped.

## How the test catches it

`Test_UnitProxyBothDirectionsFail` uses a `failConn` whose `Read` and `Write` always fail. Both directions fail together:

```go
	conn := failConn{errors.New("i/o error")}
	before := runtime.NumGoroutine()
	for range 10000 {
		_ = Proxy(conn, conn)
	}
	g.Eventually(runtime.NumGoroutine, 5*time.Second).Should(gomega.BeNumerically("<=", before))
```

`Eventually` is there because the second goroutine exits just after `Proxy` returns. I ran this test against the old `proxy.go`. It failed five runs in five, leaving roughly 900 to 1,900 goroutines against a start of 2, depending on the machine, and `-race` printed `WARNING: DATA RACE`. Against the merged code, `go test -race -count=20` passed. With `GOMAXPROCS=1` the old code passed, as the PR warns, so run leak tests on more than one core.

## A leak check you can copy

The old and new shape in miniature. It makes 10,000 calls on failing connections and counts goroutines:

```go
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"time"
)

// failConn fails every Read and Write, like a connection that was reset.
type failConn struct{}

func (failConn) Read([]byte) (int, error)  { return 0, errors.New("reset") }
func (failConn) Write([]byte) (int, error) { return 0, errors.New("reset") }

func pipe(src, dst io.ReadWriter, report func(error)) {
	buf := make([]byte, 1<<15)
	for {
		n, err := src.Read(buf)
		if err == nil {
			_, err = dst.Write(buf[:n])
		}
		if err != nil {
			report(err)
			return
		}
	}
}

// proxy returns the first error from either direction. With old set it has
// the pre-fix shape: an unbuffered channel and a plain bool as the guard.
func proxy(old bool) error {
	a, b := failConn{}, failConn{}
	errc := make(chan error, 2)
	report := func(err error) { errc <- err }
	if old {
		errc = make(chan error)
		done := false
		report = func(err error) {
			if done {
				return
			}
			done = true
			errc <- err
		}
	}
	go pipe(a, b, report)
	go pipe(b, a, report)
	return <-errc
}

func main() {
	old := len(os.Args) > 1 && os.Args[1] == "old"
	before := runtime.NumGoroutine()
	for range 10000 {
		proxy(old)
	}
	time.Sleep(500 * time.Millisecond) // let goroutines that can exit, exit
	fmt.Printf("before=%d after=%d\n", before, runtime.NumGoroutine())
}
```

On Go 1.24.5, `go run . new` printed `before=1 after=1` in 15 of 15 runs. `go run . old` left a different count every run: 83 to 117 in one batch of 15, 12 to 64 in another. Any count above 1 is the leak. My copy leaks less than the real test. I did not test why; it omits the logging and error wrapping. `go run -race . old` reports the race.

## The lesson for io.Copy-style proxies

- Count senders and receivers on every channel, on every path. If the receiver may leave early, size the buffer for all senders.
- A plain bool shared by two goroutines is not a guard. Buffer the channel or use `sync.Once`.
- Test the failure path: connections that fail at once, many calls, then `runtime.NumGoroutine`. Use `-race` and several cores.

## How others solve it

`net/http/httputil.ReverseProxy` handles protocol upgrades with the same shape ([source](https://github.com/golang/go/blob/a5ae3a45aa44ad33e0c03d2d4093c1cd7bbd5a81/src/net/http/httputil/reverseproxy.go#L915-L964)): `errc := make(chan error, 1)`, two copier goroutines, each sending exactly once. The parent reads one value, and a second only if the first was `nil`. A buffer of 1 is enough because the parent always reads at least once. The k3s buffer of 2 does not depend on that. The copiers also pass on a half-close with `CloseWrite` when the connection supports it.

rancher/remotedialer's `pipe` uses no channel: a `sync.WaitGroup`, a `closePipe` that closes both ends, then `wg.Wait()` ([source](https://github.com/rancher/remotedialer/blob/1e594259f9fc168abf5c4311ebb4309892b1c6ab/client_dialer.go#L37-L62)). It waits for both directions. #14810 followed it.

If you have chased a leak like this one, reply on [X](https://x.com/spicysharkco).
