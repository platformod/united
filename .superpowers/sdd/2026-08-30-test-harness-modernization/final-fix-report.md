# Final fix report — test harness modernization

## Finding addressed

Removed the unused lazy handler machinery from `bindTestRoutes` in `test_helpers_test.go`.

`tests.ApiScenario` invokes the fixture factory, creates its own router, triggers `OnServe`, and builds the mux for each scenario. The handler formerly returned from `bindTestRoutes` was ignored by `fixtureAppWithUnitedRoutes`, so its local `sync.Once`, router construction, `OnServe` trigger, mux build, and handler wrapper could never be used.

## Changes

- Changed `bindTestRoutes(t testing.TB, app core.App)` to registration-only; it now only calls `registerHooks` and `registerRoutes`.
- Removed now-unused `net/http` and `apis` imports from `test_helpers_test.go`.
- Preserved the separate `newTestHandler` route-building path used by white-box tests.
- Preserved the SPDX header and fixture app cleanup lifecycle.

## Tests

Focused fixture scenario coverage:

```text
$ go test ./... -run 'Test(Fixture(UserCanAuthenticateWithPassword|UserCanAuthenticateAndCreateOwnedGroup|StateCollectionRejectsUnauthenticatedRequests|StatefileCollectionRejectsUnauthenticatedRequests|SuperuserCanInspectRetainedStateVersions)|FixtureTestAppUsesIsolatedCopy)$'
ok   github.com/platformod/united  0.740s
?    github.com/platformod/united/migrations  [no test files]
```

Full Go suite:

```text
$ go test ./...
ok   github.com/platformod/united  9.847s
?    github.com/platformod/united/migrations  [no test files]
```

## Self-review

- Confirmed `bindTestRoutes` has one call site, and that call site intentionally discards the old result.
- Inspected PocketBase `tests.ApiScenario.test`: it calls `TestAppFactory`, then constructs the router, manually triggers `OnServe`, and builds the mux itself.
- Confirmed `newTestHandler` remains unchanged for white-box tests that explicitly require a handler.
- Ran `gofmt` and `git diff --check`; no formatting or whitespace errors.

## Concerns

- No behavior change is intended; the removed code path was unreachable from the fixture scenario factory.
- Pre-existing unrelated workspace changes remain uncommitted and were not included: `.gitignore`, `test_pb_data/data.db`, `test_pb_data/data.db-shm`, `test_pb_data/data.db-wal`, `docs/superpowers/plans/2026-08-30-test-harness-modernization.md`, and other existing `.superpowers` artifacts.
