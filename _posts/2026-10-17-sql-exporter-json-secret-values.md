---
layout: post
title: "One number broke a string map: JSON secrets in sql_exporter"
date: 2026-10-17
project: sql_exporter
pr: https://github.com/burningalchemist/sql_exporter/pull/1118
status: merged
tags: [observability-data, secrets, json]
---

A database secret from AWS made sql_exporter use the whole JSON text as its connection string. I fixed the lookup in [PR #1118](https://github.com/burningalchemist/sql_exporter/pull/1118), merged 2026-10-17, under 27 minutes after I opened it. A map of strings decoded the secret. One number broke that decode, and a fallback hid the error.

## When it happens

A `data_source_name` can point at a secret, such as `awssecretsmanager://prod/db?region=eu-west-1`. If the secret is JSON, the `key` parameter picks one field. The default is `data_source_name`.

[AWS documents database secrets](https://docs.aws.amazon.com/secretsmanager/latest/userguide/reference_secret_json_structure.html) with a bare number for `port` and says "You can add more key/value pairs". A secret in that shape plus a `data_source_name` key is ordinary:

```json
{"host": "db.example.com", "port": 5432, "data_source_name": "postgres://u:p@db.example.com:5432/app"}
```

With it, `key` is ignored. The whole JSON text becomes the DSN, or replaces `DSN_VALUE` in a `template`. The target fails when it opens, typically with `first path segment in URL cannot contain colon`. A plain-string secret, or a JSON object whose values are all strings, was never affected. A boolean does the same. AWS and GCP return the secret as stored, so they are affected. Kubernetes and Vault build a string map themselves.

## How the key is read

`extractKey` in `config/secret_resolver.go` decodes the secret into `map[string]string`. On success it takes the `key` field. On failure it treats the secret as a plain string. JSON with one number is valid JSON but not a valid `map[string]string`, so it took the plain-string path.

## Why the gap existed

[PR #918](https://github.com/burningalchemist/sql_exporter/pull/918) added the resolver with that map type. [PR #1057](https://github.com/burningalchemist/sql_exporter/pull/1057) added `template` and kept it.

A string map looks safe: every value is already a string. The cost was the fallback. "Not JSON" and "JSON with a number or boolean" took the same path.

## The fix

Decode into `map[string]any`, and require only the selected key to be a string. The diff of `config/secret_resolver.go`:

```diff
@@ -78,10 +78,11 @@ func (r *secretResolver) resolve(ctx context.Context, value string) (string, err
 	return extractKey(raw.(string), u, value)
 }
 
-// extractKey pulls the appropriate key from a raw secret value (JSON or plain string).
+// extractKey pulls the appropriate key from a raw secret value (JSON or plain string). Only the selected key has to be
+// a string, other values of the JSON object can be of any type (e.g. the numeric "port" of a database secret).
 func extractKey(raw string, u *url.URL, originalValue string) (string, error) {
 	var resolved string
-	var payload map[string]string
+	var payload map[string]any
 	if jsonErr := json.Unmarshal([]byte(raw), &payload); jsonErr == nil {
 		key := u.Query().Get("key")
 		if key == "" {
@@ -91,7 +92,11 @@ func extractKey(raw string, u *url.URL, originalValue string) (string, error) {
 		if !ok {
 			return "", fmt.Errorf("key %q not found in secret %q", key, originalValue)
 		}
-		resolved = fieldValue
+		str, ok := fieldValue.(string)
+		if !ok {
+			return "", fmt.Errorf("key %q in secret %q is not a string", key, originalValue)
+		}
+		resolved = str
 	} else {
 		resolved = raw
 	}
```

A selected number now fails with `key "port" in secret "..." is not a string`. Plain strings and `template` work as before, and text that is not a JSON object still counts as a plain string.

## The test

`TestSecretResolver_JSONSecretValueTypes` in `config/secret_resolver_test.go` serves a fixed secret through a test provider. The secret has a numeric `port` and a boolean `ssl`. Five cases: the default key, `key=reader` with a template, a missing key, `key=port`, and a plain string.

Before the fix, 4 of 5 cases failed. Even the missing key returned the whole JSON with no error. My PR records all 5 passing after. A rerun gave 10 of 10 failing runs before and 10 of 10 passing after.

## What the maintainer did

@burningalchemist approved with “Yeah, makes sense to me. 👍 I guess, the original string-to-string mapping was introduced as a safe variant. There is a room for improvement 💯” and merged 19 seconds later. It was my fourth merged sql_exporter PR, after [#1115](https://github.com/burningalchemist/sql_exporter/pull/1115), [#1116](https://github.com/burningalchemist/sql_exporter/pull/1116) and [#1117](https://github.com/burningalchemist/sql_exporter/pull/1117).

## The lesson

- A typed decode with a silent fallback hides type errors. Make each failure path say which case it was.
- Decode loosely, then check only the field you use. The other fields belong to someone else's schema.
- Test with the input the provider documents: a numeric `port` reproduces this.

If a fallback in your own code hides an input you did not expect, reply on [X](https://x.com/spicysharkco).
