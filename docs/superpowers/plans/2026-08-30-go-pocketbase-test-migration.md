# Remaining Go PocketBase Test Migration Plan

**Goal:** Move ordinary PocketBase and Terraform HTTP request/response coverage to the fixture-backed suite while retaining direct tests whose assertions require application internals, controlled persistence, filesystem corruption, transactions, concurrency, or restart lifecycle.

**Architecture:** `pocketbase_api_test.go` is the home for fixture-backed `tests.ApiScenario` coverage. Each scenario creates an isolated copy of `test_pb_data` with `newFixtureTestApp`, binds United hooks and routes through `fixtureAppWithUnitedRoutes`, and uses fixture identities or scenario setup. Direct tests continue to use `newTestApp` only when the behavior cannot be expressed as a public request/response scenario without modifying application state or lifecycle.

**Tech Stack:** Go, PocketBase v0.40.1 `tests.TestApp` and `tests.ApiScenario`, Testify, `net/http`, `go test`.

**Spec:** `docs/superpowers/specs/2026-08-30-test-harness-modernization-design.md`

## Global Constraints

- Treat `test_pb_data` as immutable fixture input; each scenario must use its isolated test-app copy.
- Preserve Terraform HTTP backend paths, the immutable group route slug, logical-state `group/name` identity, encrypted state versions, and state-lock HTTP semantics.
- Keep test-only fixture credentials out of production source and logs.
- Do not broaden the Terraform integration harness or change production behavior during this migration.
- Run `gofmt` on modified Go files and `go test ./...` after every migration slice.

---

## Assessment and Boundaries

### Existing fixture/API scenarios

| Current test | Classification and disposition | Reason |
| --- | --- | --- |
| `pocketbase_api_test.go:TestFixtureUserCanAuthenticateWithPassword` | Fixture/API scenario; keep unchanged. | It verifies fixture human-user password authentication through PocketBase's collection API. |
| `pocketbase_api_test.go:TestFixtureUserCanAuthenticateAndCreateOwnedGroup` | Fixture/API scenario; keep unchanged. | It verifies human-user group creation and service-assigned group ownership. |
| `pocketbase_api_test.go:TestStateCollectionRejectsUnauthenticatedRequests` | Fixture/API scenario; keep unchanged. | It verifies the protected logical-state collection API boundary. |
| `pocketbase_api_test.go:TestStatefileCollectionRejectsUnauthenticatedRequests` | Fixture/API scenario; keep unchanged. | It verifies the protected state-version collection API boundary. |
| `pocketbase_api_test.go:TestSuperuserCanInspectRetainedStateVersions` | Fixture/API scenario with white-box setup and persistence assertions; keep it in `pocketbase_api_test.go` and normalize its setup. | Its superuser collection request is public API coverage, while setup posts state documents and the final assertions inspect the retained logical state and state versions. Replace its direct group construction and hardcoded `postFixtureState` helper with `fixtureGroup` and `fixtureStateRequest`; retain the persistence assertions. |

### Fixture/API-scenario candidates

| Current test | Destination and replacement | Reason |
| --- | --- | --- |
| `auth_test.go:TestStateRoutesRequireMatchingGroupCredentials` | `pocketbase_api_test.go`; `tests.ApiScenario` with a fixture-created group and an invalid Basic Auth header | Public unauthorized response and challenge only. |
| `auth_test.go:TestCrossGroupCredentialsCannotAccessAnotherGroupURL` | `pocketbase_api_test.go`; `tests.ApiScenario` with fixture-created `platform` and `operations` groups | Public cross-group credential rejection only. |
| `auth_test.go:TestEveryStateRouteRequiresBasicAuth` | `pocketbase_api_test.go`; table of `tests.ApiScenario` requests for `GET`, `POST`, `DELETE`, `LOCK`, and `UNLOCK` | Public unauthenticated behavior only. |
| `auth_test.go:TestGroupCredentialsCannotUsePocketBaseAuthEndpoint` | `pocketbase_api_test.go`; `tests.ApiScenario` for `POST /api/collections/groups/auth-with-password` | Public collection-auth rejection only. |
| `hooks_test.go:TestGroupOwnerAPIRestrictsGroupManagementToItsOwner` | `pocketbase_api_test.go`; fixture-user authenticated group create, list, view, and update scenarios, plus a second fixture-created human user for inaccessible list/view scenarios | Group-management API authorization and forced ownership are public contracts. |
| `hooks_test.go:TestGroupSoftDeleteRetainsTheRecord` | `pocketbase_api_test.go`; authenticated fixture-user `DELETE /api/collections/groups/records/:id` scenario with `AfterTestFunc` tombstone assertion | The logical deletion is an API action; record inspection verifies retention. |
| `hooks_test.go:TestDeletedGroupCannotBeUpdatedOrRevivedThroughTheAPI` | `pocketbase_api_test.go`; scenario sequence for owner deletion, rejected display-name update, and rejected tombstone clearing | Group-management API behavior only. |
| `routes_test.go:TestLockCreatesMissingStateAndConflictsReturn423` ordinary missing-logical-state lock and conflicting-lock cases | `pocketbase_api_test.go`; individual `LOCK /state/:group/:name` scenarios with `AfterTestFunc` state-lock assertion | A lock can create the logical state through the public route and conflict through a prior public lock request. |
| `routes_test.go:TestMalformedLockAndUnlockPayloadsReturn400` | `pocketbase_api_test.go`; table of `LOCK` and `UNLOCK` scenarios for malformed, empty, and whitespace-only lock IDs | Public request validation; remove the unnecessary pre-created logical state. |
| `routes_test.go:TestUnlockAndDeleteHonorOwnershipAndExpiry` ordinary missing-unlock, matching-unlock, wrong-unlock, and lock-owner delete cases | `pocketbase_api_test.go`; separate request scenarios prepared by public `LOCK` requests | Public state-lock and logical-state deletion responses. |
| `routes_test.go:TestFirstPostCreatesEncryptedVersionAndGetReturnsOriginalBody` | `pocketbase_api_test.go`; `POST` and `GET` scenarios with `AfterTestFunc` that verifies the logical state's current state version and encrypted stored bytes | Public state document round trip plus persisted state-version properties. |
| `routes_test.go:TestGetReturnsNotFoundForMissingDeletedAndVersionlessStates` missing-logical-state case | `pocketbase_api_test.go`; `GET /state/:group/missing` scenario | Public missing-state response only. |

### Direct-unit and white-box tests to preserve

| Current test | Classification | Preservation reason |
| --- | --- | --- |
| `config_test.go` all tests | Direct unit | Environment parsing and `NewApp` construction do not exercise an HTTP API. |
| `crypto_test.go` all tests | Direct unit | Encryption, wrapping, digest, and ciphertext checks are pure crypto behavior. |
| `main_test.go:TestNoTestProvisionCommand` | Direct unit | The root-command assertion is neither a PocketBase collection API nor a Terraform route scenario. |
| `state_test.go` all tests | Direct unit | `findState`, `activeLock`, `clearExpiredLock`, and `clearLock` inspect or mutate logical-state lock fields directly. |
| `test_helpers_test.go:TestFixtureTestAppUsesIsolatedCopy` | Fixture infrastructure test | It proves fixture isolation and remains next to the fixture helpers. |
| `auth_test.go:TestDeletedGroupReturnsGoneOnlyAfterValidBasicAuth` | White-box route test | It directly tombstones a group to exercise the valid-credential `410 Gone` distinction. |
| `auth_test.go:TestStateAccessSurvivesGroupDisplayAndUserNameChanges` | White-box route test | It directly mutates a group display name and human-user name between state-document requests. |
| `hooks_test.go:TestGroupSlugMustBeOnePathSafeSegment` | Direct hook test | It saves valid and invalid group route slugs directly to test model-level validation. |
| `hooks_test.go:TestGroupCreationGeneratesKeyAndRejectsIdentityChanges` | Direct hook test | It inspects generated wrapped-key state and attempts an immutable identity mutation. |
| `hooks_test.go:TestStateIdentityCannotChange` | Direct hook test | It attempts a direct logical-state identity mutation. |
| `hooks_test.go:TestStatefileMustBelongToStatesGroup` | Direct hook test | It constructs a cross-group state-version record that the public API cannot create. |
| `hooks_test.go:TestStatefileCannotChange` | Direct hook test | It attempts a direct immutable state-version mutation. |
| `hooks_test.go:TestStateCurrentVersionMustBelongToState` | Direct hook test | It assigns a logical state to another logical state's state version. |
| `hooks_test.go:TestStateLockInfoMustBeJSONWithMatchingID` | Direct hook test | It injects invalid stored state-lock fields. |
| `hooks_test.go:TestStateLockFieldsMustBeAllEmptyOrAllPresent` | Direct hook test | It creates partial stored state-lock fields that public requests cannot write. |
| `routes_test.go:TestConcurrentLockAttempts` | White-box concurrency test | It coordinates two simultaneous state-lock requests with `sync.WaitGroup`. |
| `routes_test.go:TestPostCreatesHistoryAndLockedPostRequiresMatchingQueryID` | White-box route test | It directly installs a state lock before the locked state-document write. |
| `routes_test.go:TestGetReturnsNotFoundForMissingDeletedAndVersionlessStates` deleted and versionless cases | White-box route test | They directly create a deleted or versionless logical state. |
| `routes_test.go:TestPostRejectsDeletedStateAndInvalidLockIDUsage` | White-box route test | It directly creates deleted, unlocked, and active-lock logical states. |
| `routes_test.go:TestExpiredLockRejectsStaleIDAndAllowsNoIDPost` | White-box route test | It directly installs an expired state lock. |
| `routes_test.go:TestTamperedStateVersionsReturn503` | White-box filesystem-corruption test | It deletes/uploads encrypted state-version files and bypasses hooks to corrupt a digest or wrapped group key. |
| `routes_test.go:TestPasswordRotationRejectsOldPasswordAndDecryptsOldVersion` | White-box route test | It directly rotates the group credential between requests. |
| `routes_test.go:TestRestartReadsCurrentVersionFromExistingDataDirectory` | White-box lifecycle test | It controls the PocketBase data directory and application shutdown/restart sequence. |
| `routes_test.go:TestFailedPostPreservesCurrentVersion` | White-box transaction-failure test | It injects an `OnRecordUpdate("states")` save failure. |
| `schema_test.go:TestGroupSlugMigrationRejectsExistingUnsafeSlug` | Migration-order test | It runs a selected migration against an intentionally unsafe pre-existing group route slug. |
| `schema_test.go:TestInitialSchemaCreatesPrivateStateCollections` | Direct schema test | It inspects schema objects and collection API rules directly. |

## File Structure

| File | Responsibility after migration |
| --- | --- |
| `pocketbase_api_test.go` | Fixture-backed PocketBase collection and ordinary Terraform route request/response scenarios. |
| `test_helpers_test.go` | Fixture scenario helpers and existing direct-app helpers; direct-app helpers remain until no preserved test uses them. |
| `auth_test.go` | Only the two white-box tests that mutate a group or human user. |
| `hooks_test.go` | Only direct hook invariant tests. |
| `routes_test.go` | Only white-box filesystem corruption, injected failure, direct state-lock mutation, concurrency, and restart lifecycle tests. |

### Task 1: Add reusable fixture scenario setup for ordinary route and group API coverage

**Files:**
- Modify: `test_helpers_test.go`
- Modify: `pocketbase_api_test.go`
- Test: `pocketbase_api_test.go`

**Interfaces:**
- Consumes: `newFixtureTestApp(t testing.TB) *tests.TestApp`, `fixtureAuthToken(t testing.TB, app core.App, collection, email string) string`, and `fixtureAppWithUnitedRoutes(t testing.TB) *tests.TestApp`.
- Produces: `fixtureGroup(t testing.TB, app core.App, slug, username, password string) *core.Record`, `fixtureBasicAuth(username, password string) string` for `tests.ApiScenario.Headers`, and `fixtureStateRequest(t testing.TB, handler http.Handler, method string, group *core.Record, password, name string, body []byte, queryID string) *httptest.ResponseRecorder` for scenario setup requests.

- [ ] **Step 1: Add fixture setup helpers without changing direct-app helpers**

Add `fixtureGroup` to load fixture user `user@example.com` from `users`, create a group with the supplied immutable group route slug, group credential username, password, and display name, and save it through United's hooks. Add `fixtureBasicAuth` that returns the Basic Authorization value for a supplied group credential. Replace hardcoded `postFixtureState` with `fixtureStateRequest`: build `stateURL(group, name)`, append `?ID=` plus the supplied query ID only when `queryID` is nonempty, create the request with the supplied method and body, set `fixtureBasicAuth(group.GetString("username"), password)`, serve it through `handler`, and return the recorder. Pass the group password explicitly or store it in the scenario closure; do not retain the hardcoded `state-api-password` inside the helper. Map `authenticateUser` to `fixtureAuthToken`, migrated `createGroup` calls to `fixtureGroup`, and migrated Basic Auth request construction to `fixtureBasicAuth`. Retain `request`, `lock`, and `unlock` only for preserved direct tests; keep `createGroup`, `newTestApp`, and `newTestHandler` while direct tests still call them.

- [ ] **Step 2: Add fixture scenarios for ordinary authentication and group-management requests**

Keep the five existing fixture/API scenarios listed above. Move the four ordinary tests from `auth_test.go` and the three group-management API tests from `hooks_test.go` into `pocketbase_api_test.go`. Use `tests.ApiScenario` for each HTTP request, `fixtureAuthToken` in place of `authenticateUser` for fixture human-user authorization, `fixtureGroup` in place of migrated `createGroup` calls, `fixtureBasicAuth` in place of migrated Basic Auth request construction, and `BeforeTestFunc`/`AfterTestFunc` only for isolated scenario setup and record assertions. Normalize `TestSuperuserCanInspectRetainedStateVersions` to use `fixtureGroup` and parameterized `fixtureStateRequest` for its two `POST` requests and deletion setup; keep its white-box retained-version assertions. Preserve the two `auth_test.go` white-box tests and all direct hook invariants in `hooks_test.go`.

- [ ] **Step 3: Run focused fixture coverage**

Run: `gofmt -w test_helpers_test.go pocketbase_api_test.go auth_test.go hooks_test.go && go test ./... -run '^(TestFixture|TestStateRoutesRequireMatchingGroupCredentials|TestCrossGroupCredentialsCannotAccessAnotherGroupURL|TestEveryStateRouteRequiresBasicAuth|TestGroupCredentialsCannotUsePocketBaseAuthEndpoint|TestGroupOwnerAPIRestrictsGroupManagementToItsOwner|TestGroupSoftDeleteRetainsTheRecord|TestDeletedGroupCannotBeUpdatedOrRevivedThroughTheAPI)$'`

Expected: PASS; fixture scenarios use isolated copies and the preserved direct tests retain their existing helper paths.

### Task 2: Move ordinary custom state-route request/response cases to fixture scenarios

**Files:**
- Modify: `pocketbase_api_test.go`
- Modify: `routes_test.go`
- Test: `pocketbase_api_test.go`, `routes_test.go`

**Interfaces:**
- Consumes: Task 1's `fixtureGroup` and `fixtureBasicAuth`, plus `fixtureAppWithUnitedRoutes`.
- Produces: fixture-backed scenario coverage for ordinary logical-state retrieval, state-document writes, state-version encryption/history assertions, and state-lock request validation.

- [ ] **Step 1: Split mixed route tests at the white-box boundary**

In `routes_test.go`, retain named direct subtests for expired/deleted logical states, direct state-lock installation, concurrent state-lock attempts, state-version file/key corruption, injected state save failure, group credential rotation, and restart persistence. Split `TestPostCreatesHistoryAndLockedPostRequiresMatchingQueryID` into fixture-backed `TestFixturePostCreatesStateVersionHistory`, which performs two public `POST` requests and asserts the logical state's changed current state version plus two retained state versions, and direct `TestLockedPostRequiresMatchingQueryID`, which installs `LockInfo{ID: "lock-1"}` with `setLock` and asserts the wrong and matching `?ID=` responses. Remove the duplicate ordinary two-POST history and current-version assertions from the renamed direct test after `TestFixturePostCreatesStateVersionHistory` exists. Remove only the ordinary missing-logical-state lock conflict, malformed lock payload, missing unlock, matching/wrong unlock, lock-owner delete, first state-document write/read, and missing logical-state `GET` assertions after equivalent fixture scenarios exist.

- [ ] **Step 2: Add one-request `tests.ApiScenario` cases with controlled scenario setup**

For each ordinary custom route request, use `BeforeTestFunc` to create the fixture group with `fixtureGroup` and, when a preceding public operation is required, call `fixtureStateRequest` through the bound router with the exact method, group, group-credential password, logical-state name, body, and query ID. Use `fixtureBasicAuth` in scenario headers. Add `AfterTestFunc` assertions that a first `POST` produces an encrypted current state version, `TestFixturePostCreatesStateVersionHistory` performs two public `POST` requests and retains two state versions while changing `currentVersion`, and logical-state deletion retains its current state version. Do not create expired, deleted, versionless, or active-lock records in these scenarios.

- [ ] **Step 3: Run focused route coverage**

Run: `gofmt -w pocketbase_api_test.go routes_test.go && go test ./... -run '^(TestFixturePostCreatesStateVersionHistory|TestLockedPostRequiresMatchingQueryID)$'`

Expected: PASS; fixture scenarios cover public contracts and every white-box route assertion remains direct.

### Task 3: Remove duplicated request helpers only after the direct boundary is stable

**Files:**
- Modify: `auth_test.go`
- Modify: `hooks_test.go`
- Modify: `routes_test.go`
- Modify: `test_helpers_test.go`
- Test: all top-level Go tests

**Interfaces:**
- Consumes: fixture scenario helpers from Task 1 and preserved direct tests from Task 2.
- Produces: no duplicate helper that is unused by preserved direct tests.

- [ ] **Step 1: Identify actual remaining call sites**

Use `grep -nE '\b(newHTTPTestAppWithGroup|newTestHandler|requestJSONWithAuth|requestWithAuth|requestJSON|request)\b' -- *_test.go` and retain each helper still used by a direct or white-box test. Delete a helper only after its last call site is removed.

- [ ] **Step 2: Keep lifecycle and schema coverage direct**

Do not alter `config_test.go`, `crypto_test.go`, `main_test.go`, `state_test.go`, `schema_test.go`, `TestFixtureTestAppUsesIsolatedCopy`, or the preserved white-box tests listed above. In particular, keep migration ordering, unsafe pre-existing group route slugs, filesystem corruption, transaction failure, concurrency, and restart lifecycle outside `tests.ApiScenario`.

- [ ] **Step 3: Run complete Go validation**

Run: `gofmt -w auth_test.go hooks_test.go routes_test.go test_helpers_test.go pocketbase_api_test.go && go test ./...`

Expected: PASS; no fixture test mutates `test_pb_data`, public contracts are scenario-backed, and direct tests retain required application control.

- [ ] **Step 4: Commit the migration implementation**

```bash
git add auth_test.go hooks_test.go routes_test.go test_helpers_test.go pocketbase_api_test.go
git commit -m "test: migrate ordinary PocketBase route scenarios"
```

## Plan Self-Review

- **Coverage:** Every current top-level test is classified above, including all five existing `pocketbase_api_test.go` scenarios. Fixture candidates cover ordinary authentication, group-management, logical-state, state-document, state-version, and state-lock HTTP contracts; direct tests retain their required internal control.
- **Exact split:** `TestPostCreatesHistoryAndLockedPostRequiresMatchingQueryID` becomes fixture-backed `TestFixturePostCreatesStateVersionHistory` for public history/current-version coverage and direct `TestLockedPostRequiresMatchingQueryID` for installed-lock `?ID=` coverage, without duplicate ordinary history assertions.
- **Helper mapping:** Migrated `authenticateUser` calls use `fixtureAuthToken`; migrated `createGroup` calls use `fixtureGroup`; migrated Basic Auth construction uses `fixtureBasicAuth`; `postFixtureState` becomes parameterized `fixtureStateRequest`; `request`, `lock`, and `unlock` remain for direct tests only.
- **Scope:** The migration changes test organization and helper use only. It does not alter production routes, collection rules, state encryption, group route-slug semantics, logical-state identity, state-lock behavior, migrations, or the Terraform harness.
- **Boundary:** The plan explicitly preserves tests requiring app mutations, filesystem corruption, transaction failure, concurrency coordination, migration ordering, or restart lifecycle.
- **Placeholder scan:** This plan contains no incomplete migration steps or generic follow-up markers.
