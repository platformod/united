# Test Harness Modernization Design

## Goal

Move PocketBase collection API verification out of the shell-based Terraform harness and into top-level Go tests that use PocketBase's documented test harness, while retaining a minimal single-instance Terraform integration test of United's HTTP backend contract and locking.

## Context

United stores encrypted Terraform state versions and lock metadata in PocketBase. The public Terraform state routes preserve the HTTP backend contract. PocketBase collection APIs are used separately for human-user group management.

The repository contains a committed `test_pb_data` fixture. It supplies:

- a superuser authenticated as `test@example.com` / `testtesttest`;
- a human user authenticated as `user@example.com` / `foofoofoo`.

The fixture intentionally contains no group records. Tests must not mutate it in place.

## Architecture

### Top-level PocketBase API tests

Add fixture-backed top-level Go tests using PocketBase v0.40.1's `tests.NewTestApp("test_pb_data")` and `tests.ApiScenario`.

Each scenario receives an isolated test-app instance created from `test_pb_data`; it binds United's `registerHooks` and `registerRoutes` before serving requests. Helpers generate PocketBase auth tokens from the fixture's superuser and human-user records without putting credentials in production code.

These tests own verification of PocketBase collection API behavior that is currently performed through shell `curl` calls:

- human-user authentication;
- authenticated, owner-assigned group creation;
- protected administrative inspection of logical states and state versions;
- group-management authorization and retention/tombstone behavior where fixture-backed API scenarios give stronger coverage.

Assertions may inspect records through the test app when HTTP response bodies alone cannot reliably establish encryption, current-version linkage, retained history, or a tombstone field. This stays inside `go test` and preserves the route/API boundary.

### Terraform integration harness

The `tests/` directory remains a standalone Terraform client integration harness. It starts the built United binary once on `127.0.0.1:8090`, using a fixed, cleaned runtime data directory that is distinct from the committed `test_pb_data` fixture.

The harness retains only the minimal PocketBase API bootstrap needed to establish Terraform's required group credential:

1. authenticate `user@example.com` with `foofoofoo`;
2. create the test group through the owner-scoped PocketBase collection API.

It uses Terraform's normal locking behavior: `terraform apply` and `terraform destroy` must not pass `-lock=false`. Existing explicit `LOCK` / conflict / `UNLOCK` HTTP checks remain until a Terraform-native force-unlock request fixture is captured and the documented technical debt is resolved.

The harness does not support parallel runs. It uses a fixed port and exits cleanly after one run. Its runtime directory is removed before the run and during cleanup so committed fixture data and prior integration state cannot affect results.

### Removal and simplification

Delete `tests/harness_regression_test.sh` and its Make target. It only asserts shell-script implementation details and duplicate unavailable-command checks; it does not test United behavior.

Remove obsolete environment guards and configuration whose sole purpose is randomized, parallel harness execution. Keep the shell script focused on start, two-call bootstrap, Terraform lifecycle, lock verification, state deletion, and persisted-data restart verification.

Update root and test Make targets and README text so `go test ./...` is documented as the top-level PocketBase API/unit suite and `make test` remains the single-instance Terraform integration harness.

## Test Boundaries

| Layer | Test responsibility | Mechanism |
| --- | --- | --- |
| Pure unit | Configuration validation, encryption, lock-state functions | Direct Go tests |
| PocketBase API/unit | Collection authentication/authorization, management hooks, custom routes, record persistence assertions | `tests.NewTestApp`, `tests.ApiScenario`, app record APIs |
| Terraform integration | Terraform client compatibility, actual `LOCK`/`UNLOCK` lifecycle, encrypted state persistence across a server restart | `make test`, Terraform, minimal bootstrap `curl` |

## Constraints

- Preserve the MPL-2.0 license header in changed source and shell files.
- Do not expose, log, or add production credentials, plaintext state, or master keys. The supplied fixture credentials are test-only fixture data.
- Do not mutate `test_pb_data` during tests.
- Use a generated ephemeral base64-encoded 32-byte master key only for the integration runtime directory.
- Preserve group route-slug and state-path semantics and the Terraform HTTP backend contract.
- Run `gofmt` on changed Go files and `go test ./...` for Go changes.
- Run `make test` for the Terraform HTTP backend, PocketBase persistence, authentication, and locking changes when local Terraform prerequisites are available.

## Follow-up: Existing Go Test Migration Plan

After this change, complete a separate incremental migration plan for top-level tests:

1. Keep `config_test.go`, `crypto_test.go`, and the direct state-function assertions in `state_test.go` as pure units.
2. Replace the custom `httptest` request construction shared by `auth_test.go`, `hooks_test.go`, and `routes_test.go` with fixture-backed PocketBase test-app scenario helpers where `tests.ApiScenario` can express the request and response contract.
3. Keep direct app/record setup only for corruption, transaction-failure, concurrent-lock, migration-order, and restart cases that require white-box state mutation or controlled filesystem lifecycle.
4. Keep `schema_test.go` migration assertions direct unless a fixture test verifies a user-visible collection API rule more clearly.
5. Remove duplicated helper code only after route and hook test behavior remains fully covered by the new scenario helpers.
