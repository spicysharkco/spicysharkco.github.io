---
layout: post
title: "Agents on shared machines: four ways they quietly hurt the host"
date: 2026-10-14
tags: [field-notes, agents, docker]
---

I run a fleet of coding agents on machines I own. Most are shared: agents, my own work, and a few long-running services on the same box. Agents finish the task in front of them and do not notice what it did to the machine.

None of the four problems below failed a test. Each showed up elsewhere: a process table, a disk counter, a frozen terminal, a loop that never ended. Each became a rule checked before the command runs, not a note written after it breaks.

## 1. Zombies from containers without an init

**What happened.** One container, started by an agent with a plain `docker run`, ended up with 790 defunct `git` processes. They used no CPU, but they filled the process table and the count kept climbing.

**Why.** A zombie is a child that has exited while its parent has not called `wait()` to collect its exit status. When a parent dies, its children are reparented to PID 1, and on a normal host PID 1 is an init that reaps them. Inside a container, PID 1 is whatever your entrypoint is, and most programs were never written to do init's job. An agent's shell spawns `git`, a timeout kills the shell, the orphan lands on PID 1, and nobody reaps it. You cannot kill a zombie, because it is already dead. You can only get its parent to reap it, or end the parent.

**The fix.** Run containers with an init:

```
docker run --init --rm --pids-limit 512 my-image my-command
```

`--init` makes Docker run a tiny init as PID 1 that starts your command, forwards signals, and reaps orphans. In Compose it is `init: true`. `--pids-limit` is a seatbelt: a leak hits a ceiling instead of taking the host. Find zombies with:

```
ps -eo stat,ppid,args | awk '$1 ~ /^Z/'
```

**How others solve it.** Docker's `--init` is built on tini, and you can put tini or dumb-init in the image with `ENTRYPOINT ["tini", "--"]`. That is the portable answer, since Kubernetes has no `--init` field on a container. Setting `shareProcessNamespace: true` on a pod makes its `pause` process PID 1 for every container, and it reaps orphans, at the price of containers seeing each other's processes. Init containers run once before the app starts and reap nothing.

## 2. A test database that wore down an SSD

**What happened.** Agents kept starting a throwaway Postgres to test against. For about a week it wrote roughly 1.1 TB a day to an SSD. Every test passed, so nothing prompted anyone to look.

**Why.** That is only about 13 MB/s on average, which looks unremarkable on a graph, but it is around 400 TB a year, more than many consumer drives are rated for. Postgres is built to survive a crash: every commit flushes the write-ahead log with `fsync`, and the first change to a page after a checkpoint writes the whole page to the log (`full_page_writes`). A suite committing thousands of tiny transactions pays that cost every time, to protect data deleted a minute later.

**The fix.** Keep test databases in RAM and turn off the durability you do not need:

```
docker run --init --rm -p 127.0.0.1::5432 \
  --tmpfs /var/lib/postgresql/data:size=1g \
  -e POSTGRES_PASSWORD=test \
  postgres:17 \
  -c fsync=off -c synchronous_commit=off -c full_page_writes=off
```

On the official Postgres 18 images the data directory moved, so mount the tmpfs at `/var/lib/postgresql` instead. Binding to `127.0.0.1` keeps the database off the network; the empty host port lets Docker pick one (`docker port` shows it). The Postgres documentation is blunt that `fsync=off` can corrupt a cluster after a crash. That is fine here: on tmpfs a crash loses the database anyway.

To catch a heavy writer early, `iostat -dx 5` shows write throughput per device and `pidstat -d 5` per process. For the long view, `smartctl -A` on an NVMe drive reports "Data Units Written" with a terabyte figure beside it. Record it daily and diff it.

**How others solve it.** CI systems commonly run service databases on tmpfs for the same reason. Many suites also roll back a transaction per test, or clone a template database with `CREATE DATABASE ... TEMPLATE`, so there is less to write.

## 3. One benchmark froze a shared box

**What happened.** A benchmark an agent launched used about 35 GB on a machine without that much to spare. There was no tidy out-of-memory kill. The machine froze for everyone on it, including services unrelated to the benchmark. A timeout in an agent's tool layer does not help: it can abandon a command without killing it.

**Why.** Linux does not act the moment memory runs out. It reclaims page cache, then swaps, and a box thrashing swap can be unusable for a long time before the OOM killer picks a victim. The victim is chosen system-wide, not necessarily the process that caused the problem. A cgroup limit changes the scope: when a cgroup hits its limit, the OOM killer runs inside that cgroup.

**The fix.** Run heavy jobs in a transient systemd scope with a hard cap:

```
systemd-run --user --scope --collect \
  -p MemoryMax=8G -p MemorySwapMax=0 -- ./run-benchmark
```

`MemoryMax` is the hard limit; per the systemd docs, the OOM killer is invoked inside the unit when usage cannot be held under it. `MemorySwapMax=0` keeps the job out of swap, so it dies quickly instead of dragging the machine down. `--collect` unloads the unit even if it failed, so failed scopes do not pile up. Pick the smallest cap that does the job. On a cgroup v2 distro this normally works as a user; if not, use `sudo` for a system scope. `MemoryHigh` is the softer option: it throttles instead of killing.

**How others solve it.** Docker has `--memory=8g --memory-swap=8g`; equal values mean no swap. Kubernetes has `resources.limits.memory`, and a container over it is OOM-killed. Underneath, it is all the cgroup v2 files `memory.max`, `memory.high` and `memory.swap.max`. A host-wide backstop like `systemd-oomd` or `earlyoom` helps, but a per-job cap keeps the failure small.

## 4. Waiting forever

**What happened.** Agents wait on things: a build, a deploy, a service coming up. Their default is a loop of `sleep`, check, `sleep`, check. The loop had an exit for success and none for anything else, so a job that had failed, or never started, was polled until I noticed. Every check was also a model call, so waiting cost money too.

**Why.** The loop encodes one hope. Nothing in it says how long is too long, and chained short sleeps are the same loop.

**The fix.** Every wait gets a deadline and a growing delay, and waits on the event where possible:

```
wait_for() {   # bash
  local deadline=$((SECONDS + 180)) delay=2
  until "$@"; do
    [ "$SECONDS" -ge "$deadline" ] && { echo "gave up: $*" >&2; return 1; }
    sleep "$delay"
    delay=$(( delay * 2 > 30 ? 30 : delay * 2 ))
  done
}
wait_for curl -fsS http://localhost:8080/health
```

When the system can answer, ask it: `kubectl wait --for=condition=Ready pod/my-pod --timeout=120s` blocks until the condition holds or the timeout hits. `timeout 240 <command>` caps anything, and exit status 124 means the cap was hit.

**How others solve it.** Most infrastructure has deadlines built in: Kubernetes Jobs have `activeDeadlineSeconds`, CI systems have per-job timeouts, systemd has `TimeoutStartSec`. Retrying clients use exponential backoff, often with jitter so a crowd does not retry in step.

## The common thread

Each of these is something a tired human could also have done. Agents do it faster, more often, with nobody watching the machine. So I stopped writing postmortems and started writing preconditions: the container has `--init`, the test database is on tmpfs, the heavy job has a memory cap, the wait has a deadline. Each is checked before the command runs.

If you run agents on machines you care about, I would like to hear what yours have done to them. What is the strangest resource burn you have caught? Reply on [X](https://x.com/spicysharkco).
