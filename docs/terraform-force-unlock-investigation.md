# Terraform force-unlock request investigation

## Current blocker

The PocketBase integration harness can create state, acquire a lock, and reach:

```sh
terraform force-unlock -force integration-lock
```

United currently responds with `400` and returns the active lock ID. The server accepts the lock-ID request forms covered by unit tests, so the remaining discrepancy is the precise request Terraform sends in this scenario.

## Capture requested

Run the failing command against the test backend while capturing only request metadata. Do **not** record or share Basic Auth credentials, Terraform state contents, encryption keys, or full lock payloads.

Capture:

- HTTP method
- request path and query-string parameter names
- `Content-Type`
- content length
- body encoding/shape only (for example: JSON object, JSON string, empty, form-encoded); redact values except a known test lock ID
- whether Terraform sends a retry or a different request after the first `UNLOCK`
- response status and public response body

The resulting request shape should become a route-level regression fixture before changing `requestLockInfo` again.
