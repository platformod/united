# Amend the replatform decisions for leases, crypto imports, intents, and superuser trust

## Status

Accepted. Amends the decisions recorded in issues #413, #414, #415, #416, #420, and #562, and the implementation plan at `docs/plans/pocketbase-replatform.md`.

## Context

Adversarial review of the implementation plan found four places where a recorded decision, followed literally, either cannot work on PocketBase v0.40.3 or produces a worse outcome than the decision intended:

- A lock lease is never extended, but neither Terraform nor OpenTofu sends a heartbeat, and both persist intermediate state with the lock ID. An apply that outruns the lease gets `409` on its next write, loses mutual exclusion, and leaves the user an `errored.tfstate`.
- `internal/crypto` is forbidden from importing PocketBase, but its only sanctioned cipher lives in `github.com/pocketbase/pocketbase/tools/security`. depguard matches by prefix, so the lint gate fails in the slice that adds the rule.
- Owner-carried intent flags rely on hidden fields, but PocketBase removes hidden fields from non-superuser request data before request hooks run, so an owner can never set them.
- The plan reads as if collection rules, hidden fields, and record hooks contain a superuser. They do not: superusers bypass rules, and several superuser routes (`/api/sql`, impersonation, collection truncate/import, backup restore) bypass record hooks entirely.

## Decision

### 1. A state write extends its live lease

A successful state write (`POST` or `PUT`) admitted under a live lease resets that lease's expiry to the later of the current expiry and `now + effective duration`, where the effective duration is the group override or the system default, already bounded to 5 minutes through 24 hours. A write never shortens an expiry and never revives an expired lease. The extension commits in the same transaction that publishes the version.

Everything else in #414 stands:

- reads, repeated `LOCK`, and `UNLOCK` never extend;
- a successful write does not release the lease;
- a write carrying an ID when no live lease exists remains `409`;
- expiry still bounds abandoned locks, and the 5-minute to 24-hour bounds remain.

Purpose: the lease bounds abandoned work, not running work. A single resource that runs longer than the whole lease still loses it; that case remains a documented `state push errored.tfstate` recovery.

Supersedes #414's "Lease expiry is fixed at acquisition" and "Successful state updates do not release or renew the lease" for successful state writes.

Tests: a write under a live lease extends `lock_expires_at`; a write after expiry leaves the lease absent and returns `409`; an extension never shortens an existing expiry; the extension and the version commit atomically.

### 2. `internal/crypto` may import only PocketBase's `tools/security`

The dependency rule keeps its shape: `config`, `apperr`, `telemetry`, `crypto`, and `state` never import the PocketBase framework. The one exception is `github.com/pocketbase/pocketbase/tools/security`, which is pure Go and holds the sanctioned cipher.

The depguard rule denies `github.com/pocketbase/pocketbase` and allows that one package path; in `lax` mode the longer allow entry wins. #413's "one cipher, no competing construction" is unchanged.

Consequences:

- United must enforce `len(key) == 32` itself; `security.Encrypt` silently accepts 16- and 24-byte keys as AES-128 and AES-192.
- `security.Decrypt` panics on ciphertext shorter than its 12-byte nonce, so every call site guards the length and returns a classed error.

Refines #562. #413 and #415 are unchanged.

### 3. Intent flags are write-only visible fields

`terraform_credentials.rotate_credential` and `groups.confirm_destructive` are ordinary, non-hidden, non-system boolean fields. Owners set them through the generic PocketBase `PATCH`; the request hook validates the actor and target, consumes the flag, and saves it cleared in the same transaction. `OnRecordEnrich` strips both fields after `e.Next()` for every principal, superusers included, so they never appear in a response and never persist.

The administration boundary from #416 is unchanged: no custom administration routes beyond the Terraform state contract and the owner download. The one-time credential secret continues to travel in the `X-United-Credential` response header on create and rotate, and the deployment's CORS configuration must expose that header to browser clients.

Tests: an owner sets `rotate_credential` and the secret rotates once; the field is absent from list, view, and create responses; the field is `false` after the save; a concurrent second rotation gets `409`.

Refines #416's hidden-field mechanism. The rest of #416 stands.

### 4. Superusers are trusted, and the boundary is documented

United enforces no invariant as a security boundary against a PocketBase superuser. It keeps record validators for data correctness and to catch accidents on every save path, but they are not a containment mechanism:

- `/api/sql`, impersonation, collection truncate and import, and backup upload and restore stay available to a superuser.
- Audit events are append-only for anonymous, member, owner, and service code, and are not tamper-evident against a superuser.
- Direct state-file download remains denied for every principal, superusers included, because `OnFileDownloadRequest` fires for superuser requests and can block them; this is a correctness rule, not a trust boundary.

Deployment hardening is operator-owned and must be documented: the dashboard and superuser API are reachable only over TLS behind an isolated edge, and superuser access is used for setup and backup operations only, never routine administration. Recovery from superuser misuse is restore of the paired backup set, documented in the restore procedure.

Supersedes any plan or ticket wording that implies superuser containment. Refines #415, #416, and #420; #417's existing "dashboard unreachable from the edge and superuser API reachable only over TLS with documented hardening" becomes the operative requirement.

Tests are scoped to the principal they name: "append-only for non-superuser principals", "validators reject for every save path", "file download denied for every principal".

## Consequences

- The plan needs these edits before implementation:
  - global constraints: the `crypto` import exception and the superuser trust statement;
  - Task 1.6: the length guard around `Decrypt` and the 32-byte key check;
  - Task 1.8: drop `Hidden` and `System` from the two intent fields;
  - Task 2.3: extend the lease inside the publication transaction, with the three lease tests;
  - Task 2.7: strip intent fields in `OnRecordEnrich` for every principal;
  - Task 3.4 and Task 3.5: consume the visible intent fields;
  - Task 3.7: scope append-only assertions to non-superuser principals;
  - a new documentation task and release-review line for the operator trust boundary;
  - Task 5.2: re-encrypt the sentinel during global rekey (an unrelated blocker found in the same review).
- `CONTEXT.md` needs the lease entry to say a successful state write may extend a live lease, and the audit entry to record the superuser trust boundary.
- The amendments are posted as comments on #414, #416, and #562, and noted on the #405 map.

## Considered Options

- **Keep the fixed lease and document the failure.** Rejected: it leaves a correctness hole (two writers after expiry) and an avoidable mid-apply failure that the server can prevent.
- **Add a keepalive route.** Rejected: Terraform will not call it, so it changes nothing for the only client that matters.
- **Move encryption into the adapters to preserve the dependency rule.** Rejected: it creates an interface that exists only to satisfy a lint rule, against #562's own rule.
- **Read intent flags from the raw request body.** Rejected: it depends on PocketBase body re-encoding order and must be re-verified on every upgrade.
- **Custom action routes for intents.** Rejected: it moves administration off the collection surface that #416 chose to keep.
- **A route deny list and delete guard to contain superusers.** Rejected: the operator owns network isolation and tooling access; the application should not pretend to contain a trusted role.
