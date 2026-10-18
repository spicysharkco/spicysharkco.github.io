---
layout: post
title: "CreateUser echoed the request, not the record: a stored-limits fix in AIBrix"
date: 2026-10-17
project: AIBrix
pr: https://github.com/vllm-project/aibrix/pull/3137
status: merged
tags: [model-serving, api, testing]
---

Calling `POST /CreateUser` again for an existing user returned the limits from the request, not the ones in the store. I fixed it in [PR #3137](https://github.com/vllm-project/aibrix/pull/3137), merged the same day, a little under twelve hours after I opened it. The handler returned the wrong object.

## When it happens

A user is stored with 50 RPM and 500 TPM. A second `POST /CreateUser` for the same name carries `rpm: 100` and `tpm: 1000`. It returns 200, the message `User: <name> exists` and a `user` showing 100 and 1000. The store still holds 50 and 500, and the gateway reads the stored record when it checks limits.

The docs tell users to create a user with this call (`gateway-plugins.rst` and `development/app/README.md`), so any script that runs it twice for one name reaches this branch. I found no code that reads the returned `user`. The effect is a misleading response, not a wrong quota.

## How the handler answers

`create_user` in `python/aibrix/aibrix/metadata/api/v1/users.py` asks the store whether the key exists. If it does, it returns `UserResponse(..., user=user)`, where `user` is the request body (line 103 before the fix). The docstring says it “returns existing user without modification”. `/ReadUser` in the same file reads the stored value and parses it.

## Why the gap existed

The Go handler that this one replaced printed only `User: <name> exists`. The Python rewrite in [#1639](https://github.com/vllm-project/aibrix/pull/1639) gave the response a `user` field and filled it from the request on this branch. The existing test `test_create_user_already_exists` already stored 50/500 and posted 100/1000, but it checked only that the message contained `exists`.

## The fix

Read the record instead of testing for it, and return what was read. The hunk from `users.py`:

```diff
@@ -98,9 +98,12 @@ async def create_user(request: Request, user: User) -> UserResponse:
     key = _gen_key(user.name)
 
     # Check if user exists
-    if await store.exists(key):
+    data = await store.get(key)
+    if data is not None:
         logger.info(f"User already exists: {user.name}")
-        return UserResponse(message=f"User: {user.name} exists", user=user)
+        return UserResponse(
+            message=f"User: {user.name} exists", user=User.model_validate_json(data)
+        )
 
     # Store user as JSON
     await store.set(key, user.model_dump_json())
```

Creating an existing user still leaves its stored limits unchanged. A non-empty stored value that is not valid user JSON now raises, as `/ReadUser` already does.

## The test

I extended `test_create_user_already_exists`. It now checks that `data["user"]` equals the stored 50/500 user, and that the stored bytes are unchanged.

Before the fix it failed. After it, it passed, and the metadata suite passed all 108 tests.

## What the maintainer did

@varungup90 left a review that said “The fix is correct” and merged the PR. The review reports that, against the old `users.py`, the extended test fails with `{'rpm': 100, 'tpm': 1000} != {'rpm': 50, 'tpm': 500}`.

It also probed broken stored values in a test client. An empty one gave 200 before and 500 now, while `/ReadUser` gives 404 for it. The review suggested choosing one behaviour on purpose and adding a small test. My PR body described the raise but no test pinned it. I left the PR as it was.

## The lesson

- Give the stored value and the request different numbers, then check the response body, not only the message.
- Assert that the side effect did not happen. Here, the stored bytes did not change.
- A behaviour change you state in the description is one more case to test.

If an endpoint in your own code echoes the request where it should echo the record, reply on [X](https://x.com/spicysharkco).
