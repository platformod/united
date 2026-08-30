# Task 5 Report: Go-Test Migration Assessment

## Assessment

Reviewed every top-level `*_test.go` file after the fixture-backed `pocketbase_api_test.go` addition.

The migration plan moves ordinary public HTTP contracts to isolated PocketBase fixture scenarios:

- Terraform group credential authentication and cross-group rejection;
- human-user group-management authorization, owner assignment, group deletion, and tombstone protections;
- ordinary logical-state `GET`, state-document `POST`, state-version retention, and state-lock request/response behavior.

The plan retains direct and white-box coverage when a test needs state unavailable to a public client or needs controlled internals:

- direct units for configuration, cryptography, CLI commands, and logical-state lock helpers;
- direct model-hook invariants for group route slugs, group identity, logical-state identity, state-version ownership/immutability, current-version linkage, and stored state-lock validity;
- direct group/user mutations, expired/deleted/versionless logical-state setup, credential rotation, filesystem corruption, injected transaction failure, concurrent state-lock attempts, migration ordering with unsafe pre-existing group route slugs, and restart lifecycle.

The ordered implementation is: add fixture scenario helpers and migrate ordinary authentication/group API tests; migrate ordinary custom state-route cases after splitting mixed tests at their white-box boundary; remove helpers only when no preserved direct test calls them.

## File Changed

- Created `docs/superpowers/plans/2026-08-30-go-pocketbase-test-migration.md`.

## Validation Output

Command:

```text
grep -nE 'TBD|TODO|implement later|similar to|appropriate' docs/superpowers/plans/2026-08-30-go-pocketbase-test-migration.md
```

Result: no matches.

## Self-Review

- Classified all current top-level test functions, including fixture infrastructure and tests outside the seven source files named by the task brief.
- Named the fixture replacements: `newFixtureTestApp`, `fixtureAppWithUnitedRoutes`, `fixtureAuthToken`, `fixtureGroup`, and `fixtureBasicAuth`.
- Preserved every test requiring app mutations, filesystem corruption, transaction failure injection, concurrency control, migration ordering, or restart lifecycle.
- Used the domain terms group, group route slug, logical state, state version, and state lock.
- Kept the plan within test migration; it proposes no production, schema, migration, or Terraform-harness behavior change.

## Concerns

The working tree already contained unrelated modifications to `.gitignore` and `test_pb_data/data.db*`, plus untracked Task 1–4 plan/spec documents. They were not modified or included in the Task 5 commit.

## Round 1/5 Correction

### Changes

- Added an explicit classification for every existing `pocketbase_api_test.go` scenario. The first four remain fixture/API scenarios. `TestSuperuserCanInspectRetainedStateVersions` remains a fixture/API scenario with white-box setup and persistence assertions; the plan normalizes it to `fixtureGroup` and parameterized `fixtureStateRequest` while retaining its retained logical-state and state-version assertions.
- Specified the exact split for `TestPostCreatesHistoryAndLockedPostRequiresMatchingQueryID`: fixture-backed `TestFixturePostCreatesStateVersionHistory` owns two public state-document writes, changed current-state-version assertion, and retained-version count; direct `TestLockedPostRequiresMatchingQueryID` installs the state lock and checks wrong/matching `?ID=` behavior. The direct test drops ordinary history assertions after fixture coverage exists.
- Defined the helper mapping: `authenticateUser` becomes `fixtureAuthToken`; migrated `createGroup` calls become `fixtureGroup`; migrated Basic Auth construction becomes `fixtureBasicAuth`; `postFixtureState` becomes parameterized `fixtureStateRequest(t, handler, method, group, password, name, body, queryID)`; and `request`, `lock`, and `unlock` remain direct-test helpers.

### Tests and Output

Command:

```text
grep -nE 'TBD|TODO|implement later|similar to|appropriate' docs/superpowers/plans/2026-08-30-go-pocketbase-test-migration.md
```

Output: no matches (exit status 1, as expected for `grep` with no matching lines).

### Self-Review

- The corrected inventory includes all five existing fixture scenarios and states their disposition.
- The mixed history/locked-post test has one fixture responsibility and one retained direct responsibility with no duplicate ordinary history assertion.
- Scenario setup identifies concrete helper names, signatures, and migration/retention boundaries without expanding implementation scope.

## Round 2/5 Correction

### Changes

- Replaced the stale focused-validation regex alternative `TestPostCreatesHistoryAndLockedPostRequiresMatchingQueryID` with `TestLockedPostRequiresMatchingQueryID`.
- The focused route validation selects `TestFixturePostCreatesStateVersionHistory` and `TestLockedPostRequiresMatchingQueryID` by their complete names.

### Tests and Output

Command:

```text
grep -nE 'TBD|TODO|implement later|similar to|appropriate' docs/superpowers/plans/2026-08-30-go-pocketbase-test-migration.md
```

Output: no matches.

## Round 3/5 Correction

### Changes

- Replaced the anchored prefix alternative `TestFixture` in the focused route validation with the complete fixture test name `TestFixturePostCreatesStateVersionHistory`.
- The focused command now selects exactly `TestFixturePostCreatesStateVersionHistory` and `TestLockedPostRequiresMatchingQueryID`.

### Tests and Output

Command:

```text
grep -nE 'TBD|TODO|implement later|similar to|appropriate' docs/superpowers/plans/2026-08-30-go-pocketbase-test-migration.md
```

Output: no matches.
