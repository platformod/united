# Test Harness Modernization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make PocketBase API checks top-level Go tests and reduce the Terraform integration harness to a single, locking-enabled client run.

**Architecture:** Fixture-backed PocketBase test apps created with `tests.NewTestApp("test_pb_data")` provide isolated collection API scenarios using the committed human-user and superuser records. The shell harness starts one server at port 8090 using a cleaned fixed runtime directory, performs only human-user authentication and group creation, then verifies Terraform behavior with normal locking enabled.

**Tech Stack:** Go 1.27, PocketBase v0.40.1 `tests.TestApp` and `tests.ApiScenario`, Testify, Bash, Terraform HTTP backend, curl, jq, OpenSSL, Make.

**Spec:** `docs/superpowers/specs/2026-08-30-test-harness-modernization-design.md`

## Global Constraints

- Preserve MPL-2.0 headers in modified source and shell files.
- Keep master keys, production credentials, tokens, and state plaintext out of logs and committed fixtures.
- Treat `test_pb_data` as immutable test input; do not mutate it during any test.
- Generate an ephemeral base64-encoded 32-byte master key only for the integration runtime data directory.
- Preserve Terraform HTTP backend paths, group route-slug semantics, and state `group/name` identity.
- Use PocketBase v0.40.1's `tests.NewTestApp` and `tests.ApiScenario` for fixture-backed API tests.
- Run `gofmt` on changed Go files and `go test ./...` for Go changes.
- Run `make test` when Terraform, curl, jq, and OpenSSL are installed; report missing prerequisites rather than claiming the integration test passed.

---

## File Structure

| File | Responsibility |
| --- | --- |
| `test_helpers_test.go` | Provide fixture-backed `tests.TestApp` creation, route binding, and known-account token helpers without changing direct-app helpers used by white-box tests. |
| `pocketbase_api_test.go` (new) | Test human-user authentication, group creation/ownership, and protected state/state-version collection access with `tests.ApiScenario`. |
| `tests/Makefile` | Keep one `test` target; remove harness-regression orchestration and parallel-run configuration. |
| `tests/run.sh` | Start a single server at `127.0.0.1:8090`, use fixed cleaned runtime data, bootstrap one group through two API calls, run locking-enabled Terraform lifecycle, and retain direct lock checks. |
| `tests/provision.sh` | Reduce bootstrap to authenticate the supplied human user and create the group; return its ID for state inspection. |
| `tests/harness_regression_test.sh` (delete) | Remove implementation-detail checks that do not validate behavior. |
| `Makefile` / `README.md` | Describe `go test ./...` as top-level Go/PocketBase API coverage and `make test` as the single-instance Terraform integration harness. |
| `docs/pocketbase-follow-ups.md` | Remove the obsolete deferred normal-locking item and record the narrow remaining `force-unlock` investigation, if the content still needs a follow-up pointer. |

### Task 1: Add fixture-backed PocketBase API test infrastructure

**Files:**
- Modify: `test_helpers_test.go:5-27`
- Test: `test_helpers_test.go`

**Interfaces:**
- Consumes: `test_pb_data`, `Config`, `registerHooks`, `registerRoutes`.
- Produces: `newFixtureTestApp(t testing.TB) *tests.TestApp`, `fixtureAuthToken(t testing.TB, app core.App, collection, email string) string`, and `bindTestRoutes(t testing.TB, app core.App) http.Handler`.

- [ ] **Step 1: Write a focused test that opens two fixture-backed applications and proves fixture state is isolated**

```go
func TestFixtureTestAppUsesIsolatedCopy(t *testing.T) {
    first := newFixtureTestApp(t)
    second := newFixtureTestApp(t)

    firstUser, err := first.FindAuthRecordByEmail("users", "user@example.com")
    require.NoError(t, err)
    secondUser, err := second.FindAuthRecordByEmail("users", "user@example.com")
    require.NoError(t, err)
    require.Equal(t, firstUser.Id, secondUser.Id)
}
```

- [ ] **Step 2: Run the test to verify the fixture helper is absent**

Run: `go test ./... -run '^TestFixtureTestAppUsesIsolatedCopy$'`

Expected: FAIL because `newFixtureTestApp` is undefined.

- [ ] **Step 3: Implement minimal fixture helpers in `test_helpers_test.go`**

```go
const testDataDir = "test_pb_data"

func newFixtureTestApp(t testing.TB) *tests.TestApp {
    t.Helper()
    app, err := tests.NewTestApp(testDataDir)
    require.NoError(t, err)
    t.Cleanup(app.Cleanup)
    return app
}
```

Bind United hooks/routes using a `core.ServeEvent` and `apis.NewRouter`, following the existing `newTestHandler` implementation. Implement `fixtureAuthToken` by calling `app.FindAuthRecordByEmail(collection, email)` and `record.NewAuthToken()`.

- [ ] **Step 4: Run the focused test and all Go tests**

Run: `gofmt -w test_helpers_test.go && go test ./...`

Expected: PASS. The direct-app tests remain unchanged and the fixture helper creates isolated test-app copies.

- [ ] **Step 5: Commit the focused infrastructure change**

```bash
git add test_helpers_test.go
git commit -m "test: add PocketBase fixture test helpers"
```

### Task 2: Move PocketBase collection API checks into top-level scenarios

**Files:**
- Create: `pocketbase_api_test.go`
- Modify: `test_helpers_test.go` only if Task 1 exposes a missing generic request helper
- Test: `pocketbase_api_test.go`

**Interfaces:**
- Consumes: `newFixtureTestApp`, `fixtureAuthToken`, `bindTestRoutes`, the fixture accounts, and `tests.ApiScenario`.
- Produces: fixture-backed request/response coverage for user authentication, group creation, owner assignment, and protected state/statefile collection access.

- [ ] **Step 1: Add failing API scenarios for fixture login and owner-assigned group creation**

```go
func TestFixtureUserCanAuthenticateAndCreateOwnedGroup(t *testing.T) {
    userToken := fixtureAuthToken(t, newFixtureTestApp(t), "users", "user@example.com")
    scenario := tests.ApiScenario{
        Name: "authenticated user creates a group owned by themselves",
        Method: http.MethodPost,
        URL: "/api/collections/groups/records",
        Headers: map[string]string{"Authorization": userToken},
        Body: strings.NewReader(`{"email":"fixture-tf@terraform.invalid","username":"fixture-tf","password":"fixture-password","passwordConfirm":"fixture-password","slug":"fixture","displayName":"Fixture","owner":"another-owner"}`),
        ExpectedStatus: http.StatusOK,
        ExpectedContent: []string{`"slug":"fixture"`},
        TestAppFactory: fixtureAppWithUnitedRoutes,
    }
    scenario.Test(t)
}
```

Add a scenario for `POST /api/collections/users/auth-with-password` with `user@example.com` / `foofoofoo`, and use `AfterTestFunc` to load the created group and assert its `owner` is the fixture human user's ID despite the supplied `owner` body value.

- [ ] **Step 2: Run the new scenario test to establish the expected route-binding or helper failure**

Run: `go test ./... -run '^TestFixtureUserCanAuthenticateAndCreateOwnedGroup$'`

Expected: FAIL until the scenario factory binds United hooks and routes correctly, or until expected PocketBase response content is adjusted from observed behavior.

- [ ] **Step 3: Implement `pocketbase_api_test.go` with scenario factories and protected-data coverage**

Use a distinct group slug/username per scenario because each scenario owns one isolated test app. Add:

```go
func fixtureAppWithUnitedRoutes(t testing.TB) *tests.TestApp {
    app := newFixtureTestApp(t)
    bindTestRoutes(t, app)
    return app
}
```

Use `fixtureAuthToken` for a superuser token and add `tests.ApiScenario` checks that unauthenticated requests to `/api/collections/states/records` and `/api/collections/statefiles/records` are rejected. Use an authenticated superuser scenario plus `AfterTestFunc` only after setting up an encrypted state through United’s `/state/:group/:name` route in that same app; assert a state record points to its current state version and the state-version list retains all versions after a second write and logical-state deletion.

- [ ] **Step 4: Run focused and complete Go validation**

Run: `gofmt -w pocketbase_api_test.go test_helpers_test.go && go test ./...`

Expected: PASS. The tests use the committed fixture as read-only input and every scenario has an isolated application copy.

- [ ] **Step 5: Commit the API scenario suite**

```bash
git add pocketbase_api_test.go test_helpers_test.go
git commit -m "test: cover PocketBase APIs with fixture scenarios"
```

### Task 3: Simplify the Terraform harness and enable normal locking

**Files:**
- Modify: `tests/Makefile:1-26`
- Modify: `tests/run.sh:1-142`
- Modify: `tests/provision.sh:1-55`
- Delete: `tests/harness_regression_test.sh`
- Test: `tests/run.sh` through `make test`

**Interfaces:**
- Consumes: `../dist/united`, fixture account credentials, `curl`, `jq`, `openssl`, Terraform, port `8090`.
- Produces: one non-parallel Terraform integration run that creates its group through the PocketBase API and verifies locking-enabled lifecycle behavior.

- [ ] **Step 1: Remove the obsolete script regression target and test**

Replace `tests/Makefile` targets with one test target:

```make
.PHONY: test

test: $(UNITED_BIN)
	@UNITED_BIN="$(UNITED_BIN)" ./run.sh
```

Delete `tests/harness_regression_test.sh`.

- [ ] **Step 2: Reduce `provision.sh` to the required two API calls**

Keep only a fixed test-user login request and an authenticated group creation request. Accept `<base-url> <group-slug> <username> <password>` arguments, authenticate `user@example.com` / `foofoofoo` against `users/auth-with-password`, create the group with the token, and print the returned group ID. Do not create a superuser or another human user.

- [ ] **Step 3: Replace random parallel-execution setup in `run.sh` with one fixed lifecycle**

Set constants in the script:

```bash
readonly api_url="http://127.0.0.1:8090"
readonly data_dir="$(dirname "$0")/tmp/pb_data"
readonly group_slug="integration"
readonly state_name="terraform"
readonly tf_http_username="integration-tf"
readonly tf_http_password="integration-test-password"
```

Before start, remove and recreate `tests/tmp/pb_data`. Generate and export `UNITED_STATE_MASTER_KEY` with `openssl rand -base64 32`; start `"$united_bin" serve --dir="$data_dir" --http="127.0.0.1:8090"`; retain a bounded `/ping` readiness loop and cleanup trap. Do not randomize ports, directories, user accounts, group credentials, or retry alternate ports.

- [ ] **Step 4: Use normal Terraform locks and preserve explicit lock checks**

Replace:

```bash
terraform apply -lock=false -auto-approve
terraform apply -lock=false -var changer=bar -auto-approve
terraform destroy -lock=false -auto-approve
```

with:

```bash
terraform apply -auto-approve
terraform apply -var changer=bar -auto-approve
terraform destroy -auto-approve
```

Keep the direct authenticated `LOCK`, conflicting `LOCK`, `UNLOCK`, re-lock, and `DELETE` requests. Do not add `terraform force-unlock` because its request shape is documented as unresolved.

- [ ] **Step 5: Run shell syntax and the integration harness**

Run: `bash -n tests/run.sh tests/provision.sh && make build && make test`

Expected: shell syntax passes; Terraform initializes, applies twice with locks enabled, destroys with locking enabled, verifies encrypted retained versions and tombstones, exercises direct lock conflict/unlock, and verifies persisted state after restart. If Terraform or required utilities are unavailable, report the exact missing prerequisite.

- [ ] **Step 6: Commit the harness simplification**

```bash
git add tests/Makefile tests/run.sh tests/provision.sh
git rm tests/harness_regression_test.sh
git commit -m "test: simplify Terraform integration harness"
```

### Task 4: Update test documentation and remove obsolete follow-up language

**Files:**
- Modify: `Makefile:22-23`
- Modify: `README.md:90-100`
- Modify: `docs/pocketbase-follow-ups.md:1-9`
- Test: documentation and Make target inspection

**Interfaces:**
- Consumes: the completed top-level Go API suite and `tests/` Terraform harness.
- Produces: accurate developer-facing test entry point documentation.

- [ ] **Step 1: Update Make and README test descriptions**

Change the root `test` target description to identify it as the standalone single-instance Terraform integration harness. In README development text, document `go test ./...` for Go unit and PocketBase API scenarios, and `make test` for Terraform-client integration on its fixed local port with an ephemeral integration data directory.

- [ ] **Step 2: Remove stale lock-coverage deferral**

Delete the `Deferred: Terraform client locking coverage` section from `docs/pocketbase-follow-ups.md`, because the harness now exercises normal Terraform locks. Retain a concise pointer to `terraform-force-unlock-investigation.md` only if the follow-up document still adds information beyond that dedicated investigation.

- [ ] **Step 3: Validate the documented commands and changed Markdown links**

Run: `go test ./... && make help && grep -R -n -- '-lock=false\|harness-regression\|harness_regression' Makefile README.md tests docs/pocketbase-follow-ups.md`

Expected: Go tests and Make help pass; grep produces no stale harness-regression or lock-disable references except historical documentation intentionally retained in a clearly labeled context (prefer none).

- [ ] **Step 4: Commit documentation changes**

```bash
git add Makefile README.md docs/pocketbase-follow-ups.md
git commit -m "docs: clarify test suite boundaries"
```

### Task 5: Perform the requested Go-test migration assessment

**Files:**
- Create: `docs/superpowers/plans/2026-08-30-go-pocketbase-test-migration.md`
- Test: plan review against `*_test.go`

**Interfaces:**
- Consumes: the post-refactor files `auth_test.go`, `hooks_test.go`, `routes_test.go`, `schema_test.go`, `state_test.go`, `config_test.go`, and `crypto_test.go`.
- Produces: a separate concrete plan that identifies each test as fixture/API-scenario candidate or direct-unit/white-box test and gives an implementation order.

- [ ] **Step 1: Inventory test categories after the suite refactor**

Classify each test by its required boundary:

```text
Direct units: config_test.go, crypto_test.go, state_test.go lock helpers.
Fixture/API scenarios: authentication, PocketBase collection authorization, group-management routes.
White-box route tests: tampered files/keys, injected Save failures, concurrent locks, restart persistence.
Migration/schema tests: migration ordering and unsafe pre-existing slugs.
```

- [ ] **Step 2: Write the follow-up plan with one task per safe migration slice**

The plan must name exact helper replacements and preserve white-box cases. Its first migration slice should consolidate ordinary authenticated HTTP route tests in `auth_test.go` and group API tests in `hooks_test.go`; its second should convert ordinary custom state-route request/response cases in `routes_test.go`; later slices must leave corruption, concurrency, transaction failure, restart, and migration ordering direct.

- [ ] **Step 3: Check the plan has no ambiguous migrations or accidental scope expansion**

Run: `grep -nE 'TBD|TODO|implement later|similar to|appropriate' docs/superpowers/plans/2026-08-30-go-pocketbase-test-migration.md`

Expected: no matches.

- [ ] **Step 4: Commit the follow-up plan**

```bash
git add docs/superpowers/plans/2026-08-30-go-pocketbase-test-migration.md
git commit -m "docs: plan remaining PocketBase test migration"
```

## Plan Self-Review

- **Spec coverage:** Task 1 creates isolated fixture helpers; Task 2 moves collection API and persistence inspection coverage into top-level Go tests; Task 3 removes parallel shell complexity, fixes port/data lifecycle, removes `-lock=false`, and deletes the regression script; Task 4 updates externally visible test documentation; Task 5 supplies the requested final sweep plan for existing Go tests.
- **No placeholders:** The plan specifies concrete paths, commands, requested API routes, fixture identities, data ownership, and each safe migration boundary.
- **Interface consistency:** All fixture scenario tasks consume `newFixtureTestApp`, `fixtureAuthToken`, and route binding established in Task 1. The Terraform harness owns only bootstrap and actual Terraform interactions; collection API tests own PocketBase API assertions.
