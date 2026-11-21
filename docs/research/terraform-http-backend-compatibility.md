# Terraform HTTP backend compatibility requirements

**Research ticket:** [platformod/united#406](https://github.com/platformod/united/issues/406)
**Destination:** fresh implementation from `main`, replacing Gin/S3/KMS/Redis with one single-writer PocketBase server
**Scope:** compatibility for users switching Terraform backends; migrating existing S3 state is out of scope.
**Authority:** current HashiCorp Terraform and OpenTofu documentation/source. `nrh/plan-two` was not used as an authority.

## Executive summary

The PocketBase server must expose a Terraform HTTP backend endpoint that stores one state document per configured backend address and implements the standard state lifecycle:

- `GET` retrieves state.
- `POST` updates state by default; Terraform can be configured to use another update method.
- `DELETE` removes state.
- Optional locking uses `LOCK` and `UNLOCK` by default, with JSON lock information in the request body.
- A successful lock returns `200 OK`; an already-held lock returns `409 Conflict` or `423 Locked` and a JSON representation of the current lock.
- While a lock is held, Terraform adds the lock ID as query parameter `ID` to state-update requests. The server must validate this before accepting the write.
- State request bodies are JSON and carry `Content-Type: application/json`; state/lock bodies also carry a base64-encoded MD5 in `Content-MD5`.
- The endpoint must support HTTPS in production. Basic authentication is the portable baseline; current OpenTofu also supports configured extra headers and mTLS client credentials, but those are client capabilities rather than requirements for every server deployment.
- The server must serialize writes and lock transitions as one writer. PocketBase's transaction/concurrency model must make a lock acquisition plus state mutation atomic from the backend's perspective.

This is a wire-contract requirement, not a requirement to preserve the existing S3 key layout or migrate existing S3 objects.

## Authoritative sources

1. HashiCorp Terraform HTTP backend documentation: <https://developer.hashicorp.com/terraform/language/backend/http>
2. HashiCorp Terraform HTTP backend client source (`main`): <https://raw.githubusercontent.com/hashicorp/terraform/main/internal/backend/remote-state/http/client.go>
3. OpenTofu HTTP backend documentation: <https://opentofu.org/docs/language/settings/backends/http/>
4. OpenTofu HTTP backend configuration/source (`main`): <https://raw.githubusercontent.com/opentofu/opentofu/main/internal/backend/remote-state/http/backend.go>
5. OpenTofu HTTP backend client source (`main`): <https://raw.githubusercontent.com/opentofu/opentofu/main/internal/backend/remote-state/http/client.go>

The documentation establishes the public contract; the client sources establish request headers, accepted response codes, query parameters, and client-side behavior that the server must interoperate with.

## Required endpoint behavior

### State retrieval: `GET`

The configured `address` is the state endpoint. A successful existing state response is `200 OK` with the raw state bytes in the response body. The client accepts an empty state as no state. `204 No Content` and `404 Not Found` both mean that no state is currently available and must not be treated as a fatal backend error. `401 Unauthorized` and `403 Forbidden` have distinct authentication errors in the clients and should be used consistently by the server.

If returned, `Content-MD5` is base64-decoded by the clients and used as the state's digest; if absent, the clients calculate the digest locally. Returning the correct base64 MD5 is therefore recommended for compatibility and diagnostics, though not strictly required for successful reads.

### State update: `POST` by default

Terraform/OpenTofu sends the state bytes to `address` using `POST` unless `update_method` is configured. The client sets:

- `Content-Type: application/json`
- `Content-MD5: <base64(MD5(request body))>`

The server should accept the configured update method rather than assuming only `POST`; the normal deployment should document `POST` as the default. Successful updates are `200 OK`, `201 Created`, or `204 No Content`.

If a lock was acquired, the client copies the state URL and adds query parameter `ID=<lock ID>` to the update request. The server must reject a write with a missing, stale, or mismatched lock ID when locking is enabled. The exact error status for an invalid write lock token is not specified by the backend client; `409 Conflict` is the clearest interoperable choice and should include an actionable response body.

### State deletion: `DELETE`

The client sends `DELETE` to the state address. It considers only `200 OK` successful. The implementation should make deletion idempotent at the domain level, but must return `200 OK` for the Terraform client to regard the operation as successful. The server must decide whether deletion is permitted while a lock exists; safest behavior is to require the matching lock ID or reject the deletion with a conflict rather than silently bypassing the lock.

## Locking contract

Locking is enabled when lock and unlock addresses are configured. They may be the same URL as the state address. Defaults are:

- lock method: `LOCK`
- unlock method: `UNLOCK`

The lock and unlock methods are configurable by the client, so routing must not assume that only the literal methods are possible if the deployment chooses alternatives.

### `LOCK`

The request body is the JSON serialization of Terraform/OpenTofu's lock information. The client sends `Content-Type: application/json` and `Content-MD5`. The server must atomically check for an existing lock and create the new lock if none exists.

- New lock: return `200 OK`. The client records the requested lock ID locally.
- Existing lock: return `409 Conflict` or `423 Locked`, with the existing lock information as a JSON response body. The client parses that body to report the current lock ID and owner details.
- Auth failure: use `401 Unauthorized` for missing/required credentials and `403 Forbidden` for invalid/insufficient credentials, matching the client's explicit handling.
- Other status codes are errors to the client.

A lock record must preserve enough of the submitted lock payload for diagnostics and unlock validation, including at minimum its ID, operation, who, version, created time, and path when supplied by the client. The server must treat the lock ID as opaque rather than generating a replacement ID.

### `UNLOCK`

The client sends JSON lock information. A normal unlock requires the lock ID to match the lock acquired by that client. OpenTofu's current client also sends a synthesized payload for force-unlock because force unlock does not retain the original lock information; this means the server must validate the submitted ID and should not require every non-ID metadata field to match.

- Successful unlock: `200 OK`.
- Any other status is an error to the client.
- A missing or mismatched lock should not delete another client's lock. `409 Conflict` is the recommended response.

The server should make unlock and the subsequent state write/lock release transitions safe under concurrent requests. A single-writer design is appropriate, but the atomicity boundary must be explicit in the PocketBase implementation.

## Authentication, transport, and headers

The Terraform HTTP backend supports optional username/password Basic Authentication, with `TF_HTTP_USERNAME` and `TF_HTTP_PASSWORD` environment-variable equivalents. The server may use a fronting proxy or PocketBase authentication layer, but it must produce `401`/`403` responses compatible with the client behavior. Credentials should be supplied through environment/configuration, not committed backend configuration, because Terraform warns that backend configuration can appear in `.terraform` data and plan files.

HTTPS must be the normal production transport. The client has options for skipping certificate verification and configuring a CA and client certificate/key for mTLS. Skipping verification is a client-side escape hatch and should not become a server requirement or recommended configuration.

Current OpenTofu additionally supports a `headers` map for custom request headers. It rejects user-provided `Content-Type` and `Content-MD5` overrides and prevents an `Authorization` header from being combined with username/password. A PocketBase deployment may require a particular header or token, but should document it separately from the portable Terraform Basic Auth contract. If custom headers are used for tenancy/authentication, the server must define how they map to the authenticated PocketBase user and state namespace.

## Retry and response-body implications

The clients use retryable HTTP behavior with configurable retry count and wait bounds. The server should return deterministic 4xx responses for authentication, lock ownership, malformed lock bodies, and invalid state writes; transient 5xx responses may be retried by the client. Error bodies for lock conflicts should remain valid JSON lock information because the client parses them rather than merely displaying text.

The clients do not require a response body for successful POST, DELETE, LOCK, or UNLOCK. Avoid returning a new state representation that clients might accidentally interpret as state; status plus appropriate headers is sufficient.

## Compatibility test matrix for the fresh implementation

At minimum, test the following against both Terraform and OpenTofu versions selected for the replatform:

1. GET existing state returns `200` and exact bytes.
2. GET missing state returns `404` (and optionally verify `204` behavior).
3. POST creates state and accepts `200`, `201`, or `204` server behavior.
4. POST updates state and returns a correct `Content-MD5` on subsequent GET.
5. DELETE returns `200` and the next GET is missing.
6. LOCK succeeds with `200`, persists the full lock payload, and prevents a second lock.
7. A second LOCK returns `409` or `423` with parseable current lock JSON.
8. A locked state update includes `?ID=...`; the matching ID succeeds and a stale/missing ID fails.
9. UNLOCK with the matching ID succeeds with `200`; the next lock can succeed.
10. UNLOCK with another ID cannot remove the current lock.
11. Force-unlock's ID-only/synthesized payload is handled without requiring metadata equality.
12. Basic Auth produces expected `401`/`403` behavior, and credentials are not logged.
13. Concurrent lock attempts produce exactly one successful lock.
14. Concurrent writes are serialized by the single-writer server and cannot overwrite a newer state unexpectedly.
15. Custom update/lock/unlock methods work when configured.
16. TLS verification, proxying, and any PocketBase-specific authentication/header configuration are covered by deployment tests.

## Out of scope and migration boundary

- Reading or rewriting existing United S3/KMS objects.
- Preserving the old S3 key path semantics or Redis lock records.
- Supporting Terraform workspaces through the HTTP backend: the current backend implementation returns an error for names other than the default state and does not support workspaces.
- Treating `nrh/plan-two` as an implementation or compatibility authority.

Users will switch backend configuration and initialize a new PocketBase-backed state. Any migration workflow, if desired later, is a separate ticket and must define state export/import, locking, and rollback semantics.

## Newly surfaced decision questions

1. **Endpoint shape:** Will the PocketBase API expose one URL for state, lock, and unlock, or distinct URLs? The client supports both; one URL simplifies configuration, while distinct routes can make authorization and observability clearer.
2. **Tenant identity:** What authenticated principal and request field determine the PocketBase record namespace? The old user/group/name semantics must not be silently copied without an explicit new data model.
3. **Lock lifetime:** Are locks indefinite until unlock, or will the server add expiry/administrative recovery? Terraform's wire contract has no lease-renewal operation; expiry improves recovery but risks releasing a long-running operation.
4. **Write authorization:** Should DELETE require `ID` when a lock exists, even though the standard client does not append `ID` to DELETE? This affects safe destruction during an active lock.
5. **Conflict status:** Use `409` or `423` for lock conflicts? Both are accepted by current Terraform/OpenTofu clients; choose one as the documented canonical response.
6. **Authentication mode:** Basic Auth only, PocketBase token/header, mTLS, or a fronting identity proxy? The choice determines backend configuration examples and whether OpenTofu custom headers are needed.
7. **State representation:** Store raw Terraform state bytes (with server-side metadata separately) or parse the JSON state into PocketBase fields? Raw bytes best preserve compatibility and avoid accidental schema coupling.
8. **Corruption/integrity policy:** Should the server reject a body when `Content-MD5` is absent or mismatched, or use the header only as advisory metadata? The clients send it for bodies, but their success criteria do not mandate server validation.
9. **Concurrency boundary:** Which PocketBase transaction/SQLite locking mechanism guarantees atomic lock acquisition, lock-checked write, unlock, and delete operations under concurrent requests?
10. **Terraform/OpenTofu support window:** Which versions are supported, especially given OpenTofu's custom headers and stricter force-unlock behavior? Pin this before writing compatibility tests.

## Recommendation

Implement a narrow HTTP compatibility adapter around a raw state record and a lock record, with one serialized write path. Start with the documented Terraform contract (`GET`, `POST`, `DELETE`, `LOCK`, `UNLOCK`, Basic Auth over HTTPS), return exact accepted statuses, preserve JSON lock payloads, validate `ID` on lock-protected state mutation, and add OpenTofu custom-header support only if the chosen authentication model requires it. Keep all S3/KMS/Redis migration concerns out of this implementation.

## Addendum: client version deltas (November 2026)

Research for [platformod/united#418](https://github.com/platformod/united/issues/418). Versions examined: Terraform `v1.17.2` (2026-11-11), `v1.16.7`, `v1.15.9`, `v1.14.9`; OpenTofu `v1.13.9` (2026-11-05), `v1.12.12`, `v1.10.0`, `v1.9.0`, `v1.7.0`, `v1.6.0`.

### The wire contract is unchanged

Diffing the HTTP client across Terraform `v1.0.0` through `v1.17.2` and OpenTofu `v1.6.0` through `v1.13.9` shows no change to request methods, status codes, headers, query parameters, or body handling. The later changes are internal (contexts, interfaces, diagnostics, logging, error types) plus the client features below. A server that implements the original contract still works with every examined version.

### Differences that matter to United

| Behavior | Terraform | OpenTofu | United impact |
| --- | --- | --- | --- |
| Custom request headers | None | `headers` since `v1.7.0` ([PR #1487](https://github.com/opentofu/opentofu/pull/1487)) | Basic Auth remains the portable baseline; do not require a custom header for authentication. OpenTofu rejects `Content-Type`/`Content-MD5` overrides and refuses `Authorization` together with username/password. |
| mTLS client credentials | Since `v1.4.0` ([PR #31699](https://github.com/hashicorp/terraform/pull/31699)) | Inherited from the fork | Optional deployment choice; not required by either client. |
| Lock-conflict diagnostics | `v1.15.0`+ returns the holder's lock info; older versions return the requester's info ([commit `5e4cfd42`](https://github.com/hashicorp/terraform/commit/5e4cfd42)) | `v1.9.0`+ returns the holder's info (same shape) | Server response is unchanged: `409`/`423` plus the stored lock JSON. Older Terraform displays less useful metadata but still works. |
| Force-unlock payload | Broken through `v1.17.2`: `Unlock(id)` ignores `id` and sends the cached lock JSON, which is empty on a fresh force-unlock process | Since `v1.10.0`, synthesizes a lock payload containing the supplied `ID` ([PR #2381](https://github.com/opentofu/opentofu/pull/2381)) | Terraform force-unlock cannot convey the lock ID. A strict server must reject it; OpenTofu >= 1.10 can force-unlock normally. |

Terraform's force-unlock bug is upstream issue [#28421](https://github.com/hashicorp/terraform/issues/28421) (open since 2021) with PR [#34517](https://github.com/hashicorp/terraform/pull/34517) still open. The current client source confirms `id` is unused: <https://github.com/hashicorp/terraform/blob/v1.17.2/internal/backend/remote-state/http/client.go>.

An empirical capture against Terraform `v1.17.0` (`terraform force-unlock -force <id>`) showed:

```text
UNLOCK /state/g/n
Content-Type: application/json
Content-MD5: 1B2M2Y8AsgTpgAmY7PhCfg==   # base64(MD5(""))
Transfer-Encoding: chunked
body: <empty>
```

A normal `terraform plan` captured the standard LOCK payload:

```json
{"ID":"<uuid>","Operation":"OperationTypePlan","Info":"","Who":"user@host","Version":"1.17.0","Created":"...","Path":""}
```

`Info` and `Path` are empty in ordinary client traffic. A strict LOCK validator must require only the fields the clients actually populate (`ID`, `Operation`, `Version`, `Created`); requiring `Info`, `Path`, or even `Who` would reject legitimate clients.

OpenTofu's synthesized force-unlock payload is a normal `LockInfo` marshal with only `ID` set: `Operation`, `Info`, `Who`, `Version`, and `Path` are empty and `Created` is the zero time (`0001-01-01T00:00:00Z`). `UNLOCK` must therefore treat `ID` as the only meaningful field, exactly as the lock decision specifies.

### Status choices that constrain the server

- `LOCK` treats `409` and `423` specially: it reads the body and unmarshals it as `statemgr.LockInfo`. Invalid JSON produces an unmarshal error; a valid JSON object without an `ID` yields an empty lock ID. On Terraform before `v1.15.0` and on OpenTofu, the unmarshal error carries the requester's lock info with a non-empty ID, so `LockWithContext` retries until the command's lock timeout. Terraform `v1.15.0`+ returns no `Info` on parse failure and fails immediately.
- Therefore a lifecycle refusal on `LOCK` (tombstoned or purged state) must not reuse `409`/`423` unless it returns a valid stored lock payload. The current lock decision's `409` for tombstoned/purged states is a compatibility hazard; `410 Gone` (or `404`) reaches the clients' generic "Unexpected HTTP response code" path and fails immediately and clearly.
- `POST`/`PUT`, `DELETE`, and `UNLOCK` treat any non-success generically. `401` versus `403` changes diagnostics only for `GET` and `LOCK`. The credential decision's generic `401` for authentication failure and `403` for suspended/pending groups is client-compatible.
- `GET` must never return `404` for an authentication failure: both clients treat `404` as "no state" and may proceed to create new state. Authenticate before state lookup.
- State update accepts `200`, `201`, or `204`; `DELETE`, `LOCK`, and `UNLOCK` accept only `200`.
- Both clients send `Content-MD5` for non-empty bodies (Terraform also sends it for its empty force-unlock body) and parse it on `GET`; a malformed response header fails the read, and an absent header is replaced with a locally computed digest. Validate it when present on writes, and either return the correct base64 MD5 on reads or omit the header.
- Both clients expose `update_method`, `lock_method`, and `unlock_method`. At a single state URL, a custom lock/unlock method collides with the update method, so the supported surface should be `POST` (optionally `PUT`) for updates and the default `LOCK`/`UNLOCK` for locking. Do not infer operation type from an ambiguous body.
- Both clients retry `5xx` responses by default (`retry_max = 2`). Idempotent same-ID `LOCK` and absent-lease `UNLOCK` therefore matter; a retry after a committed write can otherwise create a duplicate version.

### Implication for the support target

There is no need for version-specific server branches. The useful policy is:

- Implement one wire contract and certify it against the current maintained release lines: Terraform `1.16.x`/`1.17.x` (with `1.15.x` optional) and OpenTofu `1.12.x`/`1.13.x`. The repository already pins Terraform `1.17.2` in `mise.toml`; the test harness would need an OpenTofu runner alongside it.
- Add OpenTofu `1.10.0` as the force-unlock feature floor, or document that force-unlock requires OpenTofu `>= 1.10`.
- Document Terraform force-unlock as unsupported until upstream [#28421](https://github.com/hashicorp/terraform/issues/28421) is fixed; normal Terraform operations are unaffected.
- Do not require OpenTofu custom headers or mTLS. Basic Auth + `POST`/`LOCK`/`UNLOCK` is the portable baseline for both clients.
