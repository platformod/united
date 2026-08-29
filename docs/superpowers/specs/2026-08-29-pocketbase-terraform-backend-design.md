# PocketBase Terraform HTTP Backend

**Status:** Accepted
**Date:** 2026-08-29

## Goal

Replace United’s S3/KMS state storage, Redis locking, and Gin runtime with a single PocketBase application while preserving the Terraform HTTP backend contract:

```text
GET      /state/{group}/{name}
POST     /state/{group}/{name}
DELETE   /state/{group}/{name}
LOCK     /state/{group}/{name}
UNLOCK   /state/{group}/{name}
```

`/ping` remains available. Existing S3 data does not need to be read or migrated by the application. Operators migrate Terraform backend configuration separately by switching the backend.

## Architectural boundary

PocketBase is the sole application runtime and owns the database, record operations, file storage, custom routes, and lifecycle hooks.

Routes are registered through PocketBase’s `OnServe` hook. Custom handlers perform request parsing, Basic Auth validation, state lookup, PocketBase record operations, transaction handling, encryption, and HTTP response mapping.

There is no Gin server, AWS S3/KMS client, Redis client, raw SQLite query, direct SQLite access, or separate internal service layer. Small package-local helpers are allowed for repeated or independently testable operations such as Basic Auth parsing, group lookup, key wrapping, state encryption, metadata calculation, lock parsing, and response mapping.

Custom handlers use PocketBase application APIs, including record lookup, `Save`, `Delete`, `RunInTransaction`, and filesystem/file-field operations. Transaction callbacks always use the callback’s transaction-scoped app. Model/record hooks enforce persistence invariants; request hooks are not relied on because these are custom routes rather than built-in record API requests.

## Domain model

### Users

PocketBase’s normal `users` auth collection represents human identities. A user may own multiple groups.

Human users authenticate to PocketBase and manage their groups through owner-scoped PocketBase APIs. User renames do not affect group ownership because ownership is represented by a relation to the user record.

### Groups

`groups` is a PocketBase auth collection used for native password hashing and validation, not for human application login.

Each group has:

- one owner relation to a `users` record
- an immutable, unique, path-safe route slug
- an immutable, unique Basic Auth identity
- a mutable display name
- a replaceable native password
- a hidden wrapped per-group state-encryption key

The group’s built-in guest auth/token route is denied. Terraform callers use only the custom Basic Auth route and never receive a PocketBase token.

The route slug and Basic Auth identity are separate fields. A display-name or human-user rename does not change either one. A password rotation does not change either one.

### Logical states

`states` contains one logical state per `(group, name)` pair.

A logical state contains:

- its immutable group relation
- its immutable state name
- an optional relation to the current `statefiles` version
- a permanent soft-delete marker
- lock fields

A state is not created by `LOCK`. The first successful `POST` creates it.

### State versions

`statefiles` contains one immutable version for each successful state write. Each version is related to exactly one logical state and one group. The group relation is redundant with the state relation but is retained for explicit ownership checks and efficient filtering; hooks must enforce that the two relations agree.

The file field contains encrypted ciphertext only. Historical versions are retained and are not exposed through the Terraform routes or ordinary public PocketBase APIs.

### Deleted logical states

`DELETE` permanently tombstones the logical state, retains all versions, and prevents later `POST` resurrection. A later backend migration or explicit administrative operation is required to reuse the path.

A future retention-cleanup operation may hard-delete deleted logical states, their versions, and files after a user-selected age. It is out of scope for this implementation.

## Collections and fields

### `groups` auth collection

- `username`: required immutable unique Basic Auth identity; configure group password authentication to use this field as its identity
- native `password`: replaceable, never returned
- `owner`: required single relation to `users`
- `slug`: required immutable unique text route identifier
- `displayName`: required mutable text label
- `wrappedStateKey`: hidden internal text containing the wrapped group data key

The collection’s guest authentication behavior must deny group credentials from obtaining PocketBase tokens. Collection rules must prevent Terraform callers from viewing or modifying group records. Human management remains owner-scoped.

### `states` base collection

- `group`: required immutable single relation to `groups`
- `name`: required immutable text state name
- `currentVersion`: optional single relation to `statefiles`
- `deletedAt`: optional date; empty while active and permanent once set
- `lockID`: optional text; empty while unlocked
- `lockInfo`: optional JSON containing serialized Terraform `LockInfo`
- `lockExpiresAt`: optional date in PocketBase’s canonical UTC format

A unique composite index on `(group, name)` enforces one logical state per group/path.

### `statefiles` base collection

- `state`: required immutable single relation to `states`
- `group`: required immutable single relation to `groups`
- `file`: required protected single file field containing ciphertext
- `originalContentLength`: pre-encryption byte length
- `originalContentType`: pre-encryption MIME type
- `originalSHA256`: SHA-256 of the original plaintext request body
- automatic creation timestamp

Statefile records are immutable after creation. Public CRUD access is denied.

## Encryption and key handling

The required `UNITED_STATE_MASTER_KEY` environment variable contains a base64-encoded 32-byte master key. Startup validates its presence, encoding, and length.

When a group is created:

1. Generate a cryptographically random 32-byte group data key.
2. Wrap it with the master key using PocketBase’s AES-GCM helper.
3. Store only the wrapped value in `groups.wrappedStateKey`.

Each state version is encrypted with its group data key. The group key is independent of the Basic Auth password, so password rotation does not affect state history.

`POST` computes original length, MIME type, and SHA-256 before encryption. It then encrypts the raw body and attaches the ciphertext to the new `statefiles` file field.

`GET` loads the current version, decrypts the file with the group key, verifies the original SHA-256, and returns the original bytes, original MIME type, and original content length.

The master key, group keys, wrapped keys, plaintext state, ciphertext, and credentials must never be logged or returned.

PocketBase record transactions make the SQLite-side state/version/current-pointer changes atomic. PocketBase’s file lifecycle uses compensating cleanup around record persistence rather than a true cross-filesystem ACID transaction. If a file becomes unreachable after a failure or process crash, it may remain as an orphan; this is an accepted first-phase risk and a future cleanup concern.

## Authentication and authorization

Every Terraform state route requires Basic Auth. Missing, malformed, unknown, or mismatched credentials return `401` with the existing Basic Auth challenge and a generic error.

The custom route:

1. Parses Basic Auth safely.
2. Resolves the group by immutable route slug.
3. Verifies that the supplied identity belongs to that group.
4. Calls the native group-record password verifier.
5. Continues only for the matching group.

The group credential authenticates access to every state under that group. It does not identify a human user and does not create a PocketBase session or token.

`states` and `statefiles` are unavailable through public collection/file APIs. Their lock fields, state metadata, and ciphertext files are hidden or denied. Future user-facing state APIs are explicitly out of scope.

## Terraform request behavior

### `GET`

- missing logical state → `404`
- deleted logical state → `404`
- state with no current version → `404`
- valid current version → decrypted original body with original metadata
- file/key/decryption/integrity/PocketBase failure → `503`

Reads do not require or acquire a state lock.

### `POST`

Terraform supplies a lock identifier as uppercase query parameter `?ID=...` after a successful lock. It is not a path parameter.

Inside a PocketBase transaction:

1. Resolve the group and logical state.
2. Reject a deleted logical state with a non-success deleted-state error (`410 Gone`).
3. If an active lock exists, require the exact current lock ID.
4. If no active lock exists, require that no `ID` was supplied. A stale or invalid supplied ID is an error.
5. If an expired lock is observed, clear it and warn-log the event; an ID-bearing write remains invalid, while a write without an ID may proceed as unlocked.
6. Read the raw body and calculate original metadata.
7. Encrypt the body with the group key.
8. Create and save an immutable `statefiles` record through PocketBase record/file operations.
9. Create the logical state first when this is the first write, then set and save `currentVersion`.

A successful write creates a new version even when locking is disabled. Concurrent unlocked writes may both succeed; the version whose transaction commits last becomes current.

If encryption, file handling, record creation, or pointer update fails, the previous current version remains authoritative and the request is non-success. An unreachable uploaded file may remain for later cleanup.

### `LOCK`

The lock body is Terraform’s JSON `LockInfo`. Missing/invalid JSON or an empty lock ID returns `400`.

Inside a transaction:

- missing or deleted logical state → `404`; no logical state is created
- unlocked existing state → save lock fields and return `200` with the requested lock ID as JSON
- active lock, including same-ID reacquisition → `423 Locked`, including the existing ID where applicable
- expired lock → warn-log, replace the lock, and return `200`
- PocketBase/database failure → `503`

### `UNLOCK`

The body contains the cached Terraform `LockInfo`, including its ID.

Inside a transaction:

- no lock or expired lock → `200` with `Lock Not Found. Expired. Probably.`
- active lock with a different ID → `400` with the current lock ID
- active lock with the matching ID → clear lock fields and return `200`
- PocketBase/database failure → `503`

Unlock is owner-only and idempotent for absent/expired locks.

### `DELETE`

`DELETE` is a state mutation and honors active locks.

Inside a transaction:

- missing or already deleted logical state → idempotent `200`
- active lock without the current `?ID=...` or with a different ID → `423 Locked`
- expired lock → clear logically, warn-log, and continue
- unlocked state → set `deletedAt`, retain all versions, and return `200`

A deleted state cannot be read, locked, or revived through the Terraform API.

## Lock semantics

Locks are stored on the logical state rather than in a separate lock collection. The lock includes the Terraform lock ID, serialized lock information, and expiry.

The lease is an absolute 35-minute duration calculated from the PocketBase server clock. PocketBase date values are persisted and compared in canonical UTC form:

```text
YYYY-MM-DD HH:mm:ss.SSSZ
```

The client-provided `LockInfo.Created` value is not used for expiry. Expired locks are treated as absent for an ID-less write or replacement lock, and every observed expiry emits a safe warning log. A supplied ID after expiry is invalid and returns an error because it indicates mixed locking mode.

All lock acquisition, expiry replacement, owner validation, unlock, and lock-aware state mutation use PocketBase record operations inside `RunInTransaction`. No raw SQL or direct SQLite access is permitted.

## Error mapping

- `400 Bad Request`: malformed lock body, missing required lock ID, invalid/stale supplied state-write ID, or lock ownership mismatch
- `401 Unauthorized`: missing or invalid Basic Auth
- `404 Not Found`: missing/deleted/uninitialized state where defined above
- `410 Gone`: `POST` against a permanently deleted state
- `423 Locked`: active lock conflict or mutation blocked by an active lock
- `503 Service Unavailable`: PocketBase, transaction, filesystem, key, encryption, decryption, or integrity failure

Responses must not disclose credentials, keys, plaintext state, or unnecessary lock data.

## Hooks and invariants

Record/model hooks should enforce invariants regardless of which internal record operation initiates the save:

- group route slug and Basic Auth identity cannot change
- state group and name cannot change
- deleted states cannot be modified or revived
- statefiles cannot be modified after creation
- each statefile’s group matches its state’s group
- current-version relations point to a statefile belonging to the same logical state
- lock fields are either consistently empty or contain a valid ID, lock payload, and expiry
- internal key and credential fields are not exposed through public serialization

Request-specific rules—Basic Auth, route slug matching, query ID interpretation, and Terraform response codes—remain in the custom handlers.

## Testing and acceptance

Focused tests use PocketBase test support and temporary persistent data directories. They cover:

- Basic Auth success/failure and group isolation
- group password rotation and immutable identity/slug behavior
- per-group key generation, wrapping, unwrapping, encryption, decryption, and invalid-key failures
- original length/MIME/SHA-256 metadata
- missing, uninitialized, valid, deleted, corrupt, and tampered states
- first `GET`, first `LOCK`, first `POST`, subsequent versions, and current-pointer behavior
- malformed lock bodies and missing IDs
- lock contention, same-ID reacquisition, owner-only unlock, expiry, and warning logging
- unlocked writes, stale supplied IDs, and lock-aware deletion
- failed writes preserving the previous current version
- permanent soft deletion and no-resurrection behavior
- concurrent lock attempts, verifying that only one lock becomes active
- group credentials being unable to obtain PocketBase tokens
- user/group ownership and user rename preservation
- restart persistence using the same `pb_data`

The Terraform integration harness runs against one persistent PocketBase process/data directory and verifies the full lifecycle, including state updates, state pull, lock contention, unlock, delete, restart recovery, retained versions, and no AWS/Redis/LocalStack/nginx dependencies.

Completion requires removing S3/KMS and Redis configuration, clients, dependencies, and local test prerequisites; updating runtime configuration, docs, and the test harness for PocketBase; and passing the focused and Terraform integration tests.
