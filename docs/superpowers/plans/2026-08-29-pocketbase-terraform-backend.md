# PocketBase Terraform HTTP Backend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the Gin/S3/KMS/Redis backend with a PocketBase application that serves the existing Terraform HTTP backend endpoints using encrypted, versioned PocketBase file records and record-backed locks.

**Architecture:** A PocketBase application registers `/ping` and the five existing `/state/{group}/{name}` methods in `OnServe`. A group-scoped Basic Auth middleware resolves the immutable group slug and verifies its native PocketBase password. Route handlers use PocketBase records and transaction-scoped apps to persist logical states, immutable encrypted versions, and 35-minute leases; record hooks protect persistence invariants.

**Tech Stack:** Go 1.26.2, PocketBase v0.40.1, PocketBase SQLite/file storage/migrations/hooks, Terraform HTTP backend, AES-GCM through PocketBase security tooling, `crypto/sha256`.

**Spec:** `docs/superpowers/specs/2026-08-29-pocketbase-terraform-backend-design.md`

## Global Constraints

- Preserve exactly these Terraform endpoints: `GET`, `POST`, `DELETE`, `LOCK`, and `UNLOCK` at `/state/{group}/{name}`, plus `/ping`.
- Do not migrate S3 data; operators switch Terraform backends separately.
- Do not retain Gin, AWS SDK/S3/KMS, Redis, LocalStack, nginx, raw SQL/direct SQLite, or a separate internal state service layer.
- Perform persistence only with PocketBase record/file APIs; use the transaction callback’s `core.App` for every operation inside `RunInTransaction`.
- Require `UNITED_STATE_MASTER_KEY` to be a base64-encoded 32-byte key; never log credentials, keys, plaintext, or ciphertext.
- Use a random per-group state key, wrapped by the master key; group password rotation must not affect state encryption.
- Retain immutable historical state versions. Tombstoned logical states and their versions are retained; a later retention-cleanup feature may hard-delete them after a user-selected age.
- Lock lease duration is exactly 35 minutes, calculated from server UTC time. Do not use the client `LockInfo.Created` value.
- Ensure `groups` has no enabled built-in password-auth/token endpoint. Its password field is used only through native `SetPassword` and `ValidatePassword` in custom Terraform-route Basic Auth.
- Keep source MPL-2.0 headers. Do not commit generated `pb_data`, state, credential, cache, or binary files.
- Do not create Git commits unless the user explicitly requests them.

---

## Planned File Structure

| File | Responsibility |
| --- | --- |
| `main.go` | Construct and start PocketBase; register embedded migrations, migration command, routes, and hooks. |
| `config.go` | Parse and validate the sole application secret, `UNITED_STATE_MASTER_KEY`. Keep Terraform `LockInfo`. |
| `routes.go` | Register `/ping` and all custom Terraform routes; map route outcomes to safe HTTP responses. |
| `auth.go` | Parse Basic Auth, resolve the route group, and validate its native PocketBase password. |
| `state.go` | Package-local record helpers for state/version lookup, lock state, expiry, record mutation, and typed route outcomes. |
| `crypto.go` | Generate/wrap/unwrap group keys; encrypt/decrypt state documents; calculate and verify plaintext metadata. |
| `hooks.go` | Register record hooks that protect immutable relations, current-version integrity, tombstones, lock consistency, and group identity/key invariants. |
| `migrations/1787961600_initial_pocketbase_schema.go` | Create `users`, `groups`, `states`, and `statefiles` collections with private rules and indexes. |
| `test_helpers_test.go` | Create disposable PocketBase test apps, isolated `pb_data`, groups, credentials, states, and HTTP requests. |
| `crypto_test.go` | Unit-test state key handling and ciphertext/plaintext metadata behavior. |
| `auth_test.go` | Test Basic Auth parsing, validation, group isolation, and disabled group token authentication. |
| `hooks_test.go` | Test collection/record invariants and group ownership behavior. |
| `routes_test.go` | Test all Terraform endpoint, versioning, lock, expiry, deletion, integrity, and concurrency semantics. |
| `tests/Makefile` | Start a temporary PocketBase backend, provision Terraform credentials via a test-only command, run the Terraform lifecycle, and inspect PocketBase records without AWS tooling. |
| `tests/main.tf` | Keep the HTTP backend lifecycle fixture, reading endpoint/credentials from environment. |
| `tests/provision.sh` | Create the test owner/group using the application’s test-only provisioning command; write no credentials to the repository. |
| `Makefile` | Build/run/test PocketBase locally without Docker, LocalStack, Redis, AWS CLI, or an AWS profile. |
| `README.md` | Document PocketBase runtime data, master key, group credentials, security model, and updated development workflow. |
| `.gitignore` | Ignore `pb_data/` and the integration test’s temporary persistent data directory. |
| `.github/workflows/terrafrom-test.yml` | Replace Docker/AWS setup with a generated test master key and PocketBase/Terraform test execution. |
| `docker-compose.yml`, `handlers.go`, `middlewares.go`, `util.go` | Delete after their responsibilities are replaced. |

### Task 1: Replace the runtime and dependency boundary

**Files:**
- Modify: `go.mod`, `go.sum`, `main.go`, `config.go`
- Delete: `handlers.go`, `middlewares.go`, `util.go`
- Create: `routes.go`, `auth.go`, `state.go`, `crypto.go`, `hooks.go`
- Test: `config_test.go`

**Interfaces:**
- Produces `type Config struct { StateMasterKey []byte }` and `func LoadConfig() (Config, error)`.
- Produces `func NewApp(cfg Config) *pocketbase.PocketBase`, used by tests and `main`.
- Adds `github.com/stretchr/testify` as a direct test-only dependency for the `require` assertions used throughout this plan.
- Retains `type LockInfo struct` with Terraform’s exact JSON field names.

- [ ] **Step 1: Write failing configuration tests**

```go
func TestLoadConfigRejectsMissingOrInvalidMasterKey(t *testing.T) {
    t.Setenv("UNITED_STATE_MASTER_KEY", "")
    _, err := LoadConfig()
    require.Error(t, err)

    t.Setenv("UNITED_STATE_MASTER_KEY", base64.StdEncoding.EncodeToString(make([]byte, 31)))
    _, err = LoadConfig()
    require.Error(t, err)
}

func TestLoadConfigDecodes32ByteMasterKey(t *testing.T) {
    key := bytes.Repeat([]byte{1}, 32)
    t.Setenv("UNITED_STATE_MASTER_KEY", base64.StdEncoding.EncodeToString(key))
    cfg, err := LoadConfig()
    require.NoError(t, err)
    require.Equal(t, key, cfg.StateMasterKey)
}
```

- [ ] **Step 2: Run the configuration tests to verify they fail**

Run: `go test ./... -run 'TestLoadConfig'`

Expected: FAIL because `LoadConfig` and the new configuration type do not yet exist.

- [ ] **Step 3: Replace bootstrap and configuration**

```go
type Config struct {
    StateMasterKey []byte
}

func LoadConfig() (Config, error) {
    encoded := os.Getenv("UNITED_STATE_MASTER_KEY")
    key, err := base64.StdEncoding.DecodeString(encoded)
    if err != nil || len(key) != 32 {
        return Config{}, errors.New("UNITED_STATE_MASTER_KEY must be a base64-encoded 32-byte key")
    }
    return Config{StateMasterKey: key}, nil
}

func NewApp(cfg Config) *pocketbase.PocketBase {
    app := pocketbase.New()
    registerHooks(app, cfg)
    registerRoutes(app, cfg)
    return app
}
```

Construct `app := NewApp(cfg)` in `main`, register `migratecmd.MustRegister`, and terminate with `log.Fatal(app.Start())`. Task 2 adds the migrations package and its blank import once it exists. Update `go.mod` to require `github.com/pocketbase/pocketbase v0.40.1` and `github.com/stretchr/testify`; remove Gin, AWS, Redis, validator, and Gin-health dependencies with `go mod tidy`. Delete the three AWS/Gin/Redis implementation files only after the new package compiles.

- [ ] **Step 4: Run formatting and focused tests**

Run: `gofmt -w main.go config.go routes.go auth.go state.go crypto.go hooks.go config_test.go && go test ./... -run 'TestLoadConfig'`

Expected: PASS.

- [ ] **Step 5: Review the worktree without committing**

Run: `git --no-pager diff --check && git --no-optional-locks status --short`

Expected: no whitespace errors. Do not commit unless explicitly asked.

### Task 2: Define the private PocketBase schema and startup migrations

**Files:**
- Create: `migrations/1787961600_initial_pocketbase_schema.go`, `schema_test.go`, `test_helpers_test.go`
- Modify: `main.go`, `.gitignore`

**Interfaces:**
- Consumes: `NewApp(Config)`.
- Produces collections named `users`, `groups`, `states`, and `statefiles`; helper code may rely on the field names in the approved spec.
- Produces `func newTestApp(t *testing.T) *pocketbase.PocketBase`, which creates a temporary persistent PocketBase data directory, bootstraps the application, and applies embedded migrations before returning the app.

- [ ] **Step 1: Write a failing schema test**

```go
func TestInitialSchemaCreatesPrivateStateCollections(t *testing.T) {
    app := newTestApp(t)
    for _, name := range []string{"users", "groups", "states", "statefiles"} {
        _, err := app.FindCollectionByNameOrId(name)
        require.NoError(t, err, name)
    }

    groups, _ := app.FindCollectionByNameOrId("groups")
    require.False(t, groups.PasswordAuth.Enabled)
    states, _ := app.FindCollectionByNameOrId("states")
    require.Nil(t, states.ListRule)
    require.Nil(t, states.ViewRule)
}
```

- [ ] **Step 2: Run it to verify the migrations are absent**

Run: `go test ./... -run 'TestInitialSchema'`

Expected: FAIL because the collections do not exist.

- [ ] **Step 3: Add the initial migration**

Create the four collections using `core.NewAuthCollection` for `users` and `groups`, and `core.NewBaseCollection` for `states` and `statefiles`. Add the fields and types from the spec exactly. Configure `groups.PasswordAuth.Enabled = false`; the custom middleware will call `ValidatePassword` directly, so no group token endpoint is available. Add a unique index for `groups.slug`, a unique index for `groups.username`, and `states.AddIndex("idx_states_group_name", true, "group, name", "")`.

Set all `states` and `statefiles` CRUD rules to `nil` (superuser-only) so ordinary APIs and protected file URLs cannot expose state. Configure `groups` owner-scoped human-management rules and deny unauthenticated access; use collection rules for owner scoping and a record request hook in Task 4 to set/validate `owner` when a human creates a group. Mark `wrappedStateKey` and `statefiles.file` hidden/protected. Add `pb_data/` and `tests/tmp/` to `.gitignore`.

- [ ] **Step 4: Run the schema test and migration command**

Run: `gofmt -w migrations/1787961600_initial_pocketbase_schema.go schema_test.go && go test ./... -run 'TestInitialSchema' && go run . migrate up`

Expected: PASS; the migration applies to the disposable local `pb_data` only.

- [ ] **Step 5: Review migration effects without committing**

Run: `git --no-pager diff --check && git --no-optional-locks status --short`

Expected: no generated `pb_data` files are tracked.

### Task 3: Implement per-group encryption and state document metadata

**Files:**
- Create: `crypto_test.go`
- Modify: `crypto.go`, `config.go`

**Interfaces:**
- Produces `func GenerateWrappedGroupKey(masterKey []byte) (string, error)`.
- Produces `func UnwrapGroupKey(masterKey []byte, wrapped string) ([]byte, error)`.
- Produces `type StateDocument struct { Ciphertext []byte; ContentLength int64; ContentType string; SHA256 string }`.
- Produces `func EncryptState(plaintext, groupKey []byte, contentType string) (StateDocument, error)` and `func DecryptState(document StateDocument, groupKey []byte) ([]byte, error)`.

- [ ] **Step 1: Write failing cryptography tests**

```go
func TestStateEncryptionRoundTripPreservesOriginalMetadata(t *testing.T) {
    masterKey := bytes.Repeat([]byte{1}, 32)
    wrapped, err := GenerateWrappedGroupKey(masterKey)
    require.NoError(t, err)
    groupKey, err := UnwrapGroupKey(masterKey, wrapped)
    require.NoError(t, err)

    document, err := EncryptState([]byte(`{"version":4}`), groupKey, "application/json")
    require.NoError(t, err)
    require.Equal(t, int64(13), document.ContentLength)
    require.Equal(t, "application/json", document.ContentType)
    plaintext, err := DecryptState(document, groupKey)
    require.NoError(t, err)
    require.Equal(t, []byte(`{"version":4}`), plaintext)
}

func TestDecryptStateRejectsTamperedCiphertextOrDigest(t *testing.T) {
    // Encrypt a document, mutate Ciphertext, then mutate SHA256 in separate assertions.
}
```

- [ ] **Step 2: Run the crypto tests to verify they fail**

Run: `go test ./... -run 'TestStateEncryption|TestDecryptState'`

Expected: FAIL because the key and document functions do not exist.

- [ ] **Step 3: Implement bounded helpers using PocketBase AES-GCM tooling**

Generate exactly 32 random bytes with `crypto/rand`. Use PocketBase’s AES-GCM helper to wrap/unwrap the group key with `Config.StateMasterKey` and encode the wrapped value as text. Encrypt the raw body with the unwrapped group key; calculate `ContentLength`, preserve the request `Content-Type` (falling back to `application/octet-stream` when empty), and store the lowercase hex SHA-256 of plaintext. `DecryptState` must decrypt and compare the recomputed digest with `crypto/subtle.ConstantTimeCompare`, returning an error for any mismatch. Neither helper may log sensitive values.

- [ ] **Step 4: Run crypto validation**

Run: `gofmt -w crypto.go crypto_test.go && go test ./... -run 'TestStateEncryption|TestDecryptState'`

Expected: PASS.

- [ ] **Step 5: Inspect the module dependency graph without committing**

Run: `go mod why github.com/pocketbase/pocketbase && git --no-pager diff --check`

Expected: PocketBase is a direct runtime dependency and the diff has no whitespace errors.

### Task 4: Add record hooks and safe test fixtures

**Files:**
- Modify: `hooks.go`, `test_helpers_test.go`
- Create: `hooks_test.go`

**Interfaces:**
- Produces `func registerHooks(app core.App, cfg Config)`.
- Produces test helpers `func createUser(t *testing.T, app core.App) *core.Record` and `func createGroup(t *testing.T, app core.App, owner *core.Record, slug, username, password string) *core.Record`.
- A saved group always has a generated `wrappedStateKey`; state and statefile identity fields are immutable after their initial save.

- [ ] **Step 1: Write failing invariant tests**

```go
func TestGroupCreationGeneratesKeyAndRejectsIdentityChanges(t *testing.T) {
    app := newTestApp(t)
    group := createGroup(t, app, createUser(t, app), "platform", "platform-tf", testPassword(t))
    require.NotEmpty(t, group.GetString("wrappedStateKey"))

    group.Set("slug", "renamed")
    require.Error(t, app.Save(group))
}

func TestStatefileMustBelongToStatesGroupAndCannotChange(t *testing.T) {
    // Create two groups and a state in the first group; attempt a statefile from the second.
}
```

- [ ] **Step 2: Run the hook tests to verify they fail**

Run: `go test ./... -run 'TestGroupCreation|TestStatefile'`

Expected: FAIL because the test app and invariant hooks are not implemented.

- [ ] **Step 3: Register persistence and public-API protections**

Use `OnRecordCreate`/`OnRecordUpdate` hooks for all internal record saves. On group creation, overwrite any supplied `wrappedStateKey` with a newly generated wrapped random key. On group update, reject changes to `slug`, `username`, `owner`, and `wrappedStateKey`; permit native password changes and display-name changes. Add the group record-request hook needed for ordinary human group creation: take the authenticated `users` record as owner and reject requests that supply another owner.

Reject updates to `states.group`, `states.name`, or any nonempty `deletedAt` state. Reject all updates to `statefiles`. Validate that a statefile’s `group` equals its state’s group and that a nonempty `states.currentVersion` belongs to that state. Validate lock consistency as either all-empty or all-present (`lockID`, JSON payload, expiry). Use `OnRecordEnrich` to hide `wrappedStateKey` and all state/statefile sensitive fields defensively, in addition to the private collection rules.

- [ ] **Step 4: Run the focused hook test suite**

Run: `gofmt -w hooks.go test_helpers_test.go hooks_test.go && go test ./... -run 'TestGroupCreation|TestStatefile|TestState'`

Expected: PASS.

- [ ] **Step 5: Check test isolation without committing**

Run: `go test ./... -run 'TestGroupCreation' -count=2 && git --no-optional-locks status --short`

Expected: PASS twice and no persistent `pb_data` is created in the repository root by tests.

### Task 5: Implement Terraform Basic Auth and route registration

**Files:**
- Modify: `auth.go`, `routes.go`
- Create: `auth_test.go`

**Interfaces:**
- Produces `func requireGroupBasicAuth(next func(*core.RequestEvent, *core.Record) error) func(*core.RequestEvent) error`.
- Produces `func registerRoutes(app core.App, cfg Config)`.
- Registered paths use `{group}` and `{name}` with `e.Request.PathValue`; `POST` and `DELETE` read lock IDs from `e.Request.URL.Query().Get("ID")`.

- [ ] **Step 1: Write failing Basic Auth and endpoint-shape tests**

```go
func TestStateRoutesRequireMatchingGroupCredentials(t *testing.T) {
    app, group := newHTTPTestAppWithGroup(t)
    response := request(t, app, http.MethodGet, "/state/"+group.GetString("slug")+"/network", nil, invalidTestUsername(t), invalidTestPassword(t))
    require.Equal(t, http.StatusUnauthorized, response.Code)
    require.Equal(t, `Basic realm="Authorization Required", charset="UTF-8"`, response.Header().Get("WWW-Authenticate"))
}

func TestGroupCredentialsCannotUsePocketBaseAuthEndpoint(t *testing.T) {
    app, group := newHTTPTestAppWithGroup(t)
    response := requestJSON(t, app, http.MethodPost, "/api/collections/groups/auth-with-password", map[string]string{
        "identity": group.GetString("username"), "password": testPassword(t),
    })
    require.NotEqual(t, http.StatusOK, response.Code)
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./... -run 'TestStateRoutesRequire|TestGroupCredentialsCannot'`

Expected: FAIL because no PocketBase custom routes or middleware exist.

- [ ] **Step 3: Register the route group and Basic Auth middleware**

In `OnServe`, register `GET /ping` to return `{"message":"pong"}`. Register the state routes under `/state` using PocketBase route patterns `/state/{group}/{name}` and bind the Basic Auth middleware to every state route, including `se.Router.Any("LOCK /state/{group}/{name}", ...)` and `se.Router.Any("UNLOCK /state/{group}/{name}", ...)`.

The middleware must parse `r.BasicAuth()`, require both values, resolve `groups.slug` from the route, require the submitted username to equal immutable `groups.username`, call `group.ValidatePassword(password)`, set the resolved group in the request store, and otherwise set the exact existing `WWW-Authenticate` challenge and return a generic `401`. It must not call `/api/collections/*`, issue a PocketBase token, or reveal whether the slug, username, or password was wrong.

- [ ] **Step 4: Run the authentication tests**

Run: `gofmt -w auth.go routes.go auth_test.go && go test ./... -run 'TestStateRoutesRequire|TestGroupCredentialsCannot'`

Expected: PASS.

- [ ] **Step 5: Run all tests accumulated so far**

Run: `go test ./...`

Expected: PASS.

### Task 6: Implement state lookup, expiry, and route outcome helpers

**Files:**
- Modify: `state.go`
- Create: `state_test.go`

**Interfaces:**
- Produces `const lockLease = 35 * time.Minute`.
- Produces `func findState(app core.App, groupID, name string) (*core.Record, error)`.
- Produces `func activeLock(state *core.Record, now time.Time) (active bool, expired bool, info LockInfo, err error)`.
- Produces `func clearLock(state *core.Record)` and `func setLock(state *core.Record, info LockInfo, now time.Time)`.
- Produces typed/sentinel errors used only by routes: `errStateMissing`, `errStateDeleted`, `errLockConflict`, `errInvalidWriteLockID`, and `errUnlockOwnership`.

- [ ] **Step 1: Write failing lock helper tests**

```go
func TestActiveLockUsesServerUTCAndExpiresAfter35Minutes(t *testing.T) {
    state := newStateRecord(t, app, group, "network")
    setLock(state, LockInfo{ID: "lock-1"}, time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC))
    require.True(t, activeAt(state, time.Date(2026, 8, 29, 12, 34, 59, 0, time.UTC)))
    require.False(t, activeAt(state, time.Date(2026, 8, 29, 12, 35, 0, 0, time.UTC)))
}

func TestActiveLockRejectsPartialOrInvalidStoredPayload(t *testing.T) {
    // Store lockID without payload/expiry and expect an error.
}
```

- [ ] **Step 2: Run the helper tests to verify they fail**

Run: `go test ./... -run 'TestActiveLock'`

Expected: FAIL because lock helpers are absent.

- [ ] **Step 3: Implement record helpers and safe expiry logging**

Use `FindFirstRecordByFilter` with bound filter parameters to find by group relation and name. Parse `lockInfo` JSON only when all lock fields are present. Store expiry from `now.UTC().Add(lockLease)` as PocketBase `types.DateTime`. Clear all three fields together. Route handlers must call a helper that logs only the group/state identifiers and expiry—not lock payload or credentials—when an expired lease is observed, then clear it with the transaction-scoped app.

- [ ] **Step 4: Run focused state helper tests**

Run: `gofmt -w state.go state_test.go && go test ./... -run 'TestActiveLock'`

Expected: PASS.

- [ ] **Step 5: Run static validation without committing**

Run: `go vet ./... && git --no-pager diff --check`

Expected: PASS with no whitespace errors.

### Task 7: Implement encrypted GET and transactional POST version writes

**Files:**
- Modify: `routes.go`, `state.go`
- Create: `routes_test.go`

**Interfaces:**
- Produces `func getState(e *core.RequestEvent, group *core.Record) error`.
- Produces `func postState(e *core.RequestEvent, group *core.Record, masterKey []byte) error`.
- A successful POST creates one `statefiles` record and updates `states.currentVersion`; an existing active lock requires the matching uppercase query parameter `ID`.

- [ ] **Step 1: Write failing GET/POST behavior tests**

```go
func TestFirstPostCreatesEncryptedVersionAndGetReturnsOriginalBody(t *testing.T) {
    app, group := newHTTPTestAppWithGroup(t)
    body := []byte(`{"version":4,"serial":1}`)
    post := request(t, app, http.MethodPost, stateURL(group, "network"), body, group.GetString("username"), testPassword(t))
    require.Equal(t, http.StatusOK, post.Code)

    get := request(t, app, http.MethodGet, stateURL(group, "network"), nil, group.GetString("username"), testPassword(t))
    require.Equal(t, http.StatusOK, get.Code)
    require.Equal(t, body, get.Body.Bytes())
    require.Equal(t, strconv.Itoa(len(body)), get.Header().Get("Content-Length"))
}

func TestPostCreatesHistoryAndLockedPostRequiresMatchingQueryID(t *testing.T) {
    // Verify two writes create two statefiles, newest is current, wrong ?ID= returns 400.
}
```

- [ ] **Step 2: Run GET/POST tests to verify they fail**

Run: `go test ./... -run 'TestFirstPost|TestPostCreatesHistory'`

Expected: FAIL because handlers are not implemented.

- [ ] **Step 3: Implement GET and POST with PocketBase file records**

For GET, find the group-scoped logical state, return `404` for missing/deleted/no-current-version, load the current `statefiles` record and ciphertext file through PocketBase’s filesystem APIs, unwrap the group key, decrypt, verify SHA-256, and use `e.Blob(http.StatusOK, originalContentType, plaintext)` with `Content-Length` set to `originalContentLength`. Convert file/key/decrypt/digest failures to generic `503`.

For POST, read the raw request body once before entering the short persistence transaction and derive `StateDocument`. In `RunInTransaction`, use `txApp` exclusively; resolve the state or create it only for this first POST. Reject a tombstone with `410`. Expire and warning-log stale locks. Permit no-ID writes only when no active lock exists; permit ID writes only for the active matching lock; reject a supplied ID without an active lock as `400`. Create the immutable `statefiles` record using `filesystem.NewFileFromBytes(document.Ciphertext, "state.enc")`, set plaintext metadata, save it through `txApp.Save`, then set and save `currentVersion` through `txApp.Save`.

If any persistence operation fails, return generic `503`; because the transaction rolls back, retain the earlier current version. Document in code near the save ordering that an uploaded, unreachable encrypted file is an accepted first-phase cleanup risk.

- [ ] **Step 4: Run GET/POST tests and the full Go suite**

Run: `gofmt -w routes.go state.go routes_test.go && go test ./... -run 'TestFirstPost|TestPostCreatesHistory' && go test ./...`

Expected: PASS.

- [ ] **Step 5: Add explicit failure preservation coverage**

Add a test that injects a failing file/state save after a valid first version and asserts a following GET still returns the first plaintext. Run: `go test ./... -run 'TestFailedPostPreservesCurrentVersion'`. Expected: PASS.

### Task 8: Implement LOCK, UNLOCK, and lock-aware DELETE

**Files:**
- Modify: `routes.go`, `state.go`, `routes_test.go`

**Interfaces:**
- Produces `func lockState(e *core.RequestEvent, group *core.Record) error`, `func unlockState(e *core.RequestEvent, group *core.Record) error`, and `func deleteState(e *core.RequestEvent, group *core.Record) error`.
- LOCK and UNLOCK decode the full JSON `LockInfo` body; DELETE reads optional uppercase `?ID=`.

- [ ] **Step 1: Write failing endpoint-semantic tests**

```go
func TestLockDoesNotCreateMissingStateAndConflictsReturn423(t *testing.T) {
    missing := lock(t, app, group, "network", LockInfo{ID: "first"})
    require.Equal(t, http.StatusNotFound, missing.Code)

    createState(t, app, group, "network")
    require.Equal(t, http.StatusOK, lock(t, app, group, "network", LockInfo{ID: "first"}).Code)
    require.Equal(t, http.StatusLocked, lock(t, app, group, "network", LockInfo{ID: "first"}).Code)
    require.Equal(t, http.StatusLocked, lock(t, app, group, "network", LockInfo{ID: "second"}).Code)
}

func TestUnlockAndDeleteHonorOwnershipAndExpiry(t *testing.T) {
    // Wrong unlock ID -> 400; absent/expired unlock -> 200 text; locked delete -> 423;
    // matching ?ID= tombstones; repeated delete -> 200; POST after deletion -> 410.
}
```

- [ ] **Step 2: Run the lock tests to verify they fail**

Run: `go test ./... -run 'TestLockDoesNot|TestUnlockAndDelete'`

Expected: FAIL because the custom-method handlers are incomplete.

- [ ] **Step 3: Implement all lock mutations in transactions**

Decode JSON strictly enough to reject malformed input and empty `LockInfo.ID` with `400`. LOCK must find an existing active state or return `404`; it must replace only expired locks (warning logged) and return `423` for every active lock, including same-ID reacquisition. On success return the requested lock ID as JSON.

UNLOCK must treat missing or expired locks as `200` with `{"message":"Lock Not Found. Expired. Probably."}`. For an active different ID, return `400` and the existing ID; for a matching ID clear all lock fields and return `200` with `{"message":"ok"}`.

DELETE must be idempotent `200` for missing or already tombstoned logical states. For an active lock, require exact `?ID=` and otherwise return `423`; expiry clears with a warning and allows deletion. Set `deletedAt` to server UTC using PocketBase’s date type, retain all versions, and never clear the tombstone.

- [ ] **Step 4: Run lock and deletion validation**

Run: `gofmt -w routes.go state.go routes_test.go && go test ./... -run 'TestLockDoesNot|TestUnlockAndDelete|TestPostToDeleted' && go test ./...`

Expected: PASS.

- [ ] **Step 5: Add concurrency coverage**

Add a test that starts two concurrent LOCK requests against the same initialized state and asserts exactly one `200` and one `423`, then run `go test ./... -run 'TestConcurrentLockAttempts' -count=20`. Expected: PASS for every repetition.

### Task 9: Verify authorization, integrity, persistence, and migration edge cases

**Files:**
- Modify: `auth_test.go`, `hooks_test.go`, `routes_test.go`

**Interfaces:**
- Consumes all prior handlers and helpers.
- Produces regression coverage for every acceptance item in the design specification.

- [ ] **Step 1: Add missing behavior tests with explicit assertions**

Add tests for: cross-group URL access with valid other-group credentials (`401`); changed group display name and changed human user name preserving state access; password rotation accepting only the new password while GET still decrypts old versions; tampered stored ciphertext and SHA returning `503`; malformed LOCK/UNLOCK JSON and blank ID returning `400`; stale supplied POST ID after expiry returning `400`; no-ID POST after expiry succeeding; `GET` for deleted state returning `404`; and a process restart that opens the same test `pb_data` and reads the previous current version.

- [ ] **Step 2: Run the new regression tests to verify expected failures before filling gaps**

Run: `go test ./... -run 'TestCrossGroup|TestPasswordRotation|TestTampered|TestMalformed|TestExpired|TestRestart'`

Expected: any uncovered case fails with its specified status or persistence assertion.

- [ ] **Step 3: Make only the minimal fixes required by failing cases**

Keep response bodies generic. Ensure error conversion consistently maps malformed/ownership/stale-ID cases to `400`, missing/deleted reads and locks to `404`, tombstoned POST to `410`, conflicts to `423`, and all PocketBase/filesystem/cryptographic failures to `503`. Do not weaken record hooks or private collection rules to make tests pass.

- [ ] **Step 4: Run complete focused validation**

Run: `gofmt -w *_test.go && go test ./... -count=1 && go vet ./...`

Expected: PASS.

- [ ] **Step 5: Run lint if available**

Run: `pre-commit run --all-files`

Expected: PASS. If the tool is not installed, record that fact and run `golangci-lint run` only when it is available.

### Task 10: Replace the Terraform integration harness and local workflow

**Files:**
- Modify: `Makefile`, `tests/Makefile`, `tests/main.tf`, `.github/workflows/terrafrom-test.yml`, `.gitignore`, `.air.toml`
- Create: `tests/provision.sh`
- Delete: `tests/nginx.conf`, `docker-compose.yml`

**Interfaces:**
- `make run` starts exactly one PocketBase-backed United process with `UNITED_STATE_MASTER_KEY` and a persistent local `pb_data` directory.
- `make test` starts an isolated persistent backend, provisions a group credential only for the test process, runs Terraform, verifies PocketBase state/version records, performs lock/delete/restart checks, and cleans temporary data.

- [ ] **Step 1: Write a failing harness assertion for no AWS dependencies**

Add a shell assertion in `tests/Makefile` that fails if `AWS_PROFILE`, `aws`, Docker, LocalStack, Redis, or nginx is required by the integration target. Keep Terraform endpoint, username, and password values supplied through environment variables rather than checked-in configuration.

- [ ] **Step 2: Run the current integration target to establish the old dependency failure**

Run: `make test`

Expected: current harness either requires the removed services or fails the new no-AWS assertion.

- [ ] **Step 3: Rebuild the harness around PocketBase**

Make `tests/provision.sh` call a narrowly scoped application CLI command implemented in `main.go` (for example `united test-provision --owner-email ... --group-slug ... --username ... --password ...`) that creates a human owner and group only through PocketBase record APIs. Have `tests/Makefile` use `mktemp -d`, generate a base64 32-byte key with `openssl rand -base64 32`, start the compiled process against that directory, wait for `/ping`, provision credentials, then run `terraform init`, two applies, `state pull`, `destroy`, lock contention/force-unlock checks, DELETE, restart against the same data directory, and PocketBase record/version assertions using a test-only CLI inspection command. Use a trap to terminate the child process and remove the temporary directory.

Remove Docker targets and AWS variables from the top-level Makefile. Make `run` require `UNITED_STATE_MASTER_KEY`, default `--dir=./pb_data`, and use Air only as a development wrapper. Remove nginx/localstack configuration and exclude `pb_data` from Air watching.

- [ ] **Step 4: Run the rewritten integration test**

Run: `make test`

Expected: PASS without Docker, AWS CLI/profile, Redis, LocalStack, or nginx.

- [ ] **Step 5: Verify restart persistence independently**

Run: `make test`

Expected: PASS a second time; its temporary persistent directory is cleaned afterwards.

### Task 11: Update deployment documentation and CI

**Files:**
- Modify: `README.md`, `.github/workflows/terrafrom-test.yml`, `Dockerfile`, `.goreleaser.yaml`, `docs/adr/0001-pocketbase-state-storage-and-locking.md`
- Test: documentation commands and CI configuration checks

**Interfaces:**
- Documents required `UNITED_STATE_MASTER_KEY`, PocketBase `pb_data` persistence/backup responsibility, Terraform group credentials, the unchanged endpoint paths, and removal of all AWS/Redis requirements.

- [ ] **Step 1: Draft documentation assertions as a checklist**

Add a Markdown checklist requiring the README to state: 32-byte base64 master key; persistent and backup-protected `pb_data`; group credentials are Terraform-only and cannot log in to PocketBase; encrypted historical versions and permanent tombstones; 35-minute leases; no automatic S3 migration; and the future hard-delete retention-cleanup limitation.

- [ ] **Step 2: Verify the old README and CI do not satisfy the checklist**

Run: `grep -nE 'S3|KMS|Redis|VALIDATE_AUTH|AUTH_URL|AWS_PROFILE|LocalStack' README.md .github/workflows/terrafrom-test.yml Makefile`

Expected: matches identify documentation and CI content to remove.

- [ ] **Step 3: Rewrite operator docs and release packaging**

Replace AWS/Redis configuration with a PocketBase run example such as `UNITED_STATE_MASTER_KEY="$(openssl rand -base64 32)" ./dist/united serve --dir=./pb_data`. Explain that `pb_data` holds SQLite data and protected state files and requires durable, access-controlled storage and backups. Preserve TLS/network-isolation guidance. Explain group slug/username immutability, password rotation behavior, and Terraform backend switch migration.

Update the Terraform workflow to install only Go and Terraform, generate an ephemeral test master key, and run `make test`. Confirm Docker packaging copies the PocketBase-enabled static binary and uses a writable `/pb_data` volume or documented `--dir` runtime argument. Update the ADR wording only if code-level implementation reveals a material decision change; otherwise leave its accepted decision intact.

- [ ] **Step 4: Validate docs, configuration, and build**

Run: `go build -o dist/united . && grep -nE 'S3|KMS|Redis|VALIDATE_AUTH|AUTH_URL|AWS_PROFILE|LocalStack' README.md Makefile .github/workflows/terrafrom-test.yml || true`

Expected: build succeeds and no obsolete operational requirement remains in those files.

- [ ] **Step 5: Run final local verification without committing**

Run: `go test ./... && go vet ./... && pre-commit run --all-files && make test && git --no-pager diff --check && git --no-optional-locks status --short`

Expected: all available checks pass, `make test` uses no AWS/Redis/LocalStack/nginx dependencies, and generated binaries/data are untracked. Do not commit unless the user asks.

## Coverage Self-Review

- **Spec coverage:** Tasks 1–2 replace runtime/config/dependencies and create private schema. Tasks 3–4 implement per-group encryption and persistence invariants. Tasks 5–9 implement and exhaustively test Basic Auth, Terraform route semantics, locking, tombstones, integrity, concurrency, and persistence. Tasks 10–11 replace external-service integration/CI and document operational security and the future cleanup limitation.
- **Placeholder scan:** This plan contains no deferred implementation markers or fill-in steps. The future retention cleanup is explicitly an out-of-scope documented limitation, not an implementation placeholder.
- **Type consistency:** Route handlers take `*core.RequestEvent` and the authenticated `*core.Record`; all record mutations are delegated to the helpers defined in Tasks 3, 4, and 6. `LockInfo` remains the single JSON representation for LOCK and UNLOCK.
