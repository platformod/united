# PocketBase Group Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let human users manage their own groups through PocketBase APIs, soft-delete groups safely, and replace test-only provisioning/inspection CLI commands with the same PocketBase API workflow.

**Architecture:** A new PocketBase migration adds group tombstones and owner-scoped API rules. Record request hooks derive ownership from the signed-in human user and convert owner deletion into a tombstone update. Terraform custom-route authentication distinguishes missing/invalid credentials (`401`) from valid credentials for a deleted group (`410`). The integration harness provisions human users and groups through PocketBase HTTP APIs after bootstrapping a local superuser.

**Tech Stack:** Go 1.27, PocketBase v0.40.1, PocketBase migrations/hooks/API rules, Terraform HTTP backend, curl.

**Spec:** `docs/superpowers/specs/2026-08-29-pocketbase-terraform-backend-design.md`

## Global Constraints

- Keep all Terraform state endpoints and their existing Basic Auth contract.
- A deleted group returns `410 Gone` only after its Basic Auth username/password have been validated; missing or invalid credentials remain generic `401`.
- Use only PocketBase record, migration, hook, and HTTP API operations—no raw SQLite access.
- A group tombstone retains the group, logical states, versions, and encrypted files; it cannot be modified or revived through normal APIs.
- Group `slug`, `username`, `owner`, and `wrappedStateKey` remain immutable.
- Delete `test-provision` and `test-inspect`; integration setup must use PocketBase HTTP APIs like ordinary users.
- Never commit test credentials, superuser credentials, tokens, keys, or state content.
- Preserve MPL-2.0 headers and require a commit per completed implementation task.

---

### Task 1: Add group tombstone schema and owner-scoped APIs

**Files:**
- Create: `migrations/1788048000_group_management.go`
- Modify: `hooks.go`, `hooks_test.go`, `auth.go`, `auth_test.go`

**Interfaces:**
- Adds optional `groups.deletedAt` date field.
- Adds owner-scoped `ListRule`, `ViewRule`, `CreateRule`, `UpdateRule`, and `DeleteRule` for authenticated `users` records.
- Produces group create/update/delete request hooks.

- [ ] **Step 1: Write failing tests**

Add tests proving a signed-in owner can create/list/view/update its group; another user cannot access it; creation ignores/rejects a supplied different owner; API deletion sets `deletedAt` rather than removing the record; and a tombstoned group cannot be updated or revived.

- [ ] **Step 2: Verify red**

Run: `go test ./... -run 'TestGroupOwnerAPI|TestGroupSoftDelete|TestDeletedGroupCannot'`

Expected: FAIL because the field, API rules, and request hooks do not exist.

- [ ] **Step 3: Add migration and hooks**

Register a migration that loads `groups`, adds `deletedAt`, and configures owner-scoped rules using `owner.id = @request.auth.id`; create requires `@request.auth.id != ""`. In `OnRecordCreateRequest("groups")`, require an authenticated `users` record, overwrite `owner` with its ID, and continue. In update/delete record-request hooks, require that same owner relation. Convert an owner delete request to setting `deletedAt` with server UTC time and saving through PocketBase record APIs; reject subsequent updates to tombstoned groups. Do not allow a normal API call to hard-delete the record.

- [ ] **Step 4: Add deleted-group Terraform behavior**

After successful Basic Auth validation, have the state-route middleware return `410 Gone` for a nonempty group `deletedAt`; preserve the exact generic `401` challenge for unknown groups, wrong usernames, and wrong passwords.

- [ ] **Step 5: Verify and commit**

Run: `gofmt -w migrations/1788048000_group_management.go hooks.go hooks_test.go auth.go auth_test.go && go test ./... && go vet ./... && golangci-lint run && pre-commit run --all-files`

Commit: `feat: add owner managed group tombstones`

### Task 2: Replace test CLI provisioning with PocketBase API setup

**Files:**
- Delete: `cli.go`, `cli_test.go`
- Modify: `main.go`, `tests/Makefile`, `tests/run.sh`, `tests/provision.sh`, `tests/main.tf`, `routes_test.go`

**Interfaces:**
- The United binary has no `test-provision` or `test-inspect` command.
- Test setup uses `superuser upsert "$ADMIN_EMAIL" "$ADMIN_PASSWORD"` only if `$DATA_DIR/data.db` is absent.
- The harness logs in to PocketBase through its HTTP API, then creates users/groups with bearer-authenticated curl calls.

- [ ] **Step 1: Write failing harness/API setup tests**

Add shell-level assertions that `cli.go` commands are absent, test credentials are generated at runtime, and provisioning requests use `/api/collections/...` with a PocketBase bearer token. Add route-level test coverage for valid deleted-group credentials receiving `410` while invalid credentials receive `401`.

- [ ] **Step 2: Verify red**

Run: `go test ./... -run 'TestDeletedGroup|TestNoTestProvisionCommand' && make test`

Expected: the CLI-dependent setup or expected deleted-group behavior fails before replacement.

- [ ] **Step 3: Implement API-only setup and assertions**

Remove CLI command registration from `main.go` and delete `cli.go`/`cli_test.go`. In `tests/run.sh`, set runtime-only `ADMIN_EMAIL`/`ADMIN_PASSWORD`; if `data_dir/data.db` does not exist, invoke the binary’s `superuser upsert` command before starting the server. Authenticate the superuser through PocketBase’s API, create the human user, authenticate that user, and create its group through the standard collection API. Use authenticated API list/view requests to inspect group/state metadata needed by the harness; never expose state plaintext. Retain temporary directory cleanup and dynamic-port/PID readiness checks.

- [ ] **Step 4: Verify and commit**

Run: `go test ./... && go vet ./... && golangci-lint run && pre-commit run --all-files && make test && make test && git --no-pager diff --check`

Commit: `test: provision PocketBase groups through API`

### Task 3: Update documentation and review

**Files:**
- Modify: `README.md`, `CONTEXT.md`, `docs/pocketbase-follow-ups.md`

- [ ] **Step 1: Document owner APIs and tombstones**

Document that human users manage only their own groups through PocketBase APIs; group deletion is a permanent API tombstone; valid deleted-group Terraform credentials receive `410`, while invalid credentials remain `401`; state/version retention is unchanged.

- [ ] **Step 2: Remove stale test CLI references**

Confirm README and follow-up notes contain no `test-provision` or `test-inspect` references. Keep the accepted Terraform native force-unlock technical debt separate.

- [ ] **Step 3: Validate and commit**

Run: `grep -nE 'test-provision|test-inspect' README.md Makefile tests .github || true && go build -o dist/united . && pre-commit run --all-files && make test`

Commit: `docs: describe PocketBase group management`

## Coverage Self-Review

- Task 1 covers owner-scoped APIs, immutable ownership, group tombstones, and the authenticated `410` distinction.
- Task 2 removes both test-only CLI seams and provisions users/groups solely through PocketBase HTTP APIs with runtime-generated credentials.
- Task 3 updates operators and domain terminology while retaining the separate force-unlock and developer-bootstrap follow-ups.
- No task requires raw SQLite access, committed secrets, or state plaintext exposure.
