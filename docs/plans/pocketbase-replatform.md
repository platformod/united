# United PocketBase Replatform Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the Gin/S3/Redis Terraform backend with one embedded PocketBase process that preserves the Terraform HTTP backend contract while owning group authentication, encrypted versioned state, lock leases, audit, cleanup, and telemetry.

**Architecture:** One Go module. `cmd/united` boots PocketBase through `app.Execute()`; a single `wire.Register` attaches routes, hooks, and crons. Pure domain packages (`config`, `apperr`, `telemetry`, `crypto`, `state`) never import PocketBase. Adapters (`store`, `hooks`, `jobs`, `offline`, `migrations`, `wire`) do. Terraform routes live at `/state/{group}/{name}`; the owner download route lives at `/download/{group}/{name}/{version}`; `/api` remains PocketBase-only.

**Tech Stack:** Go 1.27.4, PocketBase v0.40.3, `github.com/sethvargo/go-envconfig` v2, OpenTelemetry Go v1.47.0 with `autoexport`, Hugo + Hextra for user docs.

**Spec:** [Plan United's PocketBase-only replatform](https://github.com/platformod/united/issues/405) and its 19 closed decision tickets. The plan is the execution artifact; the tickets remain the authority for every decision.

## Global Constraints

- Go 1.27.4, pinned in `mise.toml`. PocketBase is pinned to v0.40.3.
- All environment parsing uses `github.com/sethvargo/go-envconfig` v2 with `envconfig.Process`. No hand-rolled `os.Getenv` parsing.
- `UNITED_` prefixes new United settings. `TELEMETRY_ENABLED` and `LOG_LEVEL` stay unprefixed.
- `UNITED_MASTER_KEY` is standard base64 decoding to exactly 32 bytes. It comes from the environment only, with no file fallback.
- `UNITED_LOCK_LEASE_DURATION` uses Go duration syntax, defaults to `35m`, and must be between `5m` and `24h`.
- State names are one URL segment, normalized to lowercase, and match `^[a-z0-9][a-z0-9._-]{0,254}$`.
- The Terraform contract accepts `GET`, `POST`, `PUT`, `DELETE`, `LOCK`, and `UNLOCK` at `/state/{group}/{name}`; every other method returns `405` with `Allow`.
- `401` for every authentication failure, `403` only for a valid route-bound credential whose group is suspended or pending retirement, `409` for lock-ID mismatches and for writes to tombstoned states, `423` with the exact stored lock JSON for live contention, `410` for `LOCK` on tombstoned or purged states and for any write to a purged state.
- Never emit state documents, credentials, passwords, bcrypt hashes, lock IDs, `Who`, raw lock JSON, master keys, group data keys, wrapped keys, ciphertext, request bodies, SQL parameters, or state digests in logs or errors.
- All source files carry the MPL-2.0 header.
- One active process and one writer. No clustering, leader election, dual-run, dual-write, or compatibility shim.
- Every slice merges green: `go build ./...`, `go test ./...`, `golangci-lint run ./...`, and `mise run check:ci`.
- A `depguard` rule denies `github.com/pocketbase/pocketbase` from `internal/config`, `internal/apperr`, `internal/telemetry`, `internal/crypto`, and `internal/state`, with one exception: `internal/crypto` may import `github.com/pocketbase/pocketbase/tools/security` (ADR 0002).
- A successful state write extends its live lease to `max(current expiry, now + effective duration)`; it never shortens a lease and never revives an expired one. Reads, `LOCK` replays, and `UNLOCK` never extend (ADR 0002).
- `HEAD` and `OPTIONS` on state routes return `405` with `Allow`; the CORS middleware is unbound on the state route group.
- Custom state routes bind `apis.BodyLimit` to the base64 size of `UNITED_MAX_STATE_SIZE` plus 64 KiB of overhead. PocketBase's 32 MiB default body limit is not the state limit.
- Superusers are trusted. No collection rule, hook, or hidden field is a containment boundary against a superuser; validators exist for data correctness and accident prevention only (ADR 0002).
- Deactivating a user sets `disabled=true`, rotates the user's token key so every existing session fails immediately, preserves memberships and pending invitations, and is rejected when it would remove a final active owner (ADR 0003).

## Review Focus

1. **State-name and path handling.** Uppercase, dots, underscores, hyphens, leading digits, empty, 256 characters, Unicode, whitespace, `..`, `/`, and `\` must normalize or reject consistently and must never escape the `state_versions` storage prefix.
2. **Lock payload edge cases.** Empty `Info`, `Path`, and `Who`, missing `Version`, invalid `Created`, arbitrary `Operation`, duplicate JSON keys, a 1 MiB body, wrong `Content-Type`, and an empty `UNLOCK` body must produce the documented status rather than a `500`.
3. **Lease expiry and credential rotation races.** A same-ID `LOCK` replay, a stale `?ID=` write, a reader during publication, and a request authenticated before rotation must all resolve linearly with no half-published version.
4. **Missing, truncated, or tampered files.** A committed version whose file is absent, undecryptable, or digest-mismatched must fail closed with no state bytes, no fallback to older history, and no metadata mutation.
5. **Offline command misuse.** A wrong master key, wrong generation, missing sentinel, or a server still running must make `rekey`, `verify`, and forced cleanup refuse safely with no partial database change.

---

## File Structure

```text
cmd/united/main.go
cmd/united/devseed.go
internal/config/config.go
internal/apperr/apperr.go
internal/telemetry/registry.go
internal/telemetry/logger.go
internal/telemetry/redact.go
internal/telemetry/otel.go
internal/crypto/masterkey.go
internal/crypto/groupkey.go
internal/crypto/envelope.go
internal/crypto/sentinel.go
internal/state/name.go
internal/state/lifecycle.go
internal/state/retention.go
internal/state/lease.go
internal/state/clock.go
internal/migrations/202611210001_schema.go
internal/migrations/202611210002_rules.go
internal/store/store.go
internal/store/credentials.go
internal/store/states.go
internal/store/versions.go
internal/store/locks.go
internal/store/audit.go
internal/store/groups.go
internal/store/memberships.go
internal/store/credential_lifecycle.go
internal/store/groups_lifecycle.go
internal/store/users.go
internal/httpapi/router.go
internal/httpapi/auth.go
internal/httpapi/state.go
internal/httpapi/lock.go
internal/httpapi/download.go
internal/httpapi/errors.go
internal/httpapi/middleware.go
internal/hooks/records.go
internal/hooks/enrich.go
internal/hooks/files.go
internal/hooks/logs.go
internal/hooks/groups.go
internal/hooks/memberships.go
internal/hooks/credentials.go
internal/hooks/groups_lifecycle.go
internal/hooks/states_admin.go
internal/hooks/users.go
internal/jobs/guard.go
internal/jobs/jobs.go
internal/jobs/cleanup.go
internal/jobs/orphans.go
internal/jobs/purge.go
internal/jobs/integrity.go
internal/offline/root.go
internal/offline/rekey.go
internal/offline/verify.go
internal/offline/cleanup.go
internal/wire/wire.go
tests/integration/harness.go
tests/backends/main.tf
tests/restore/drill.sh
docs/releases/v0.2.0.md
tests/ (Terraform fixture)
site/
.github/workflows/smoke.yml
.github/workflows/test.yml
.github/workflows/terraform-v2.yml
.github/workflows/terraform-matrix.yml
.github/workflows/docs.yml
docs/adr/0001-embed-pocketbase-in-process.md
docs/plans/pocketbase-replatform.md
```

## Package Contracts

These names and signatures are fixed for the whole plan. Later tasks consume them without redefining them.

```go
// internal/config
type Config struct {
	MasterKey           []byte
	MasterKeyGeneration int
	MaxStateSize        int64
	DataDir             string
	ListenAddr          string
	TelemetryEnabled    bool
	LogLevel            slog.Level
	LockLeaseDuration   time.Duration
	Actor               string
}
type Mode int
const (
	ModeServe Mode = iota
	ModeCommand
)
func Load(ctx context.Context, mode Mode) (*Config, error)

// internal/apperr
type Class string
const (
	ClassValidation   Class = "validation"
	ClassNotFound     Class = "not_found"
	ClassConflict     Class = "conflict"
	ClassGone         Class = "gone"
	ClassUnauthorized Class = "unauthorized"
	ClassForbidden    Class = "forbidden"
	ClassStorage      Class = "storage"
	ClassDatabase     Class = "database"
	ClassCrypto       Class = "crypto"
	ClassInternal     Class = "internal"
)
type Error struct {
	Class Class
	Op    string
	Msg   string
	Err   error
	Safe  bool
}
func New(class Class, op string, err error) *Error
func NewSafe(class Class, op, msg string) *Error
func HTTPStatus(class Class) int
func ClassOf(err error) Class

// internal/state
type Status string
const (
	StatusEmpty      Status = "empty"
	StatusActive     Status = "active"
	StatusTombstoned Status = "tombstoned"
	StatusPurged     Status = "purged"
)
type LockRequest struct {
	ID        string
	Operation string
	Version   string
	Created   string
	Info      string
	Path      string
	Who       string
	Raw       string
}
type LockOutcome string
const (
	LockAcquired   LockOutcome = "acquired"
	LockIdempotent LockOutcome = "idempotent"
	LockContended  LockOutcome = "contended"
	LockGone       LockOutcome = "gone"
	LockInvalid    LockOutcome = "invalid"
	LockReleased   LockOutcome = "released"
	LockNotFound   LockOutcome = "not_found"
	LockMismatched LockOutcome = "mismatched"
)
type Lease struct {
	ID         string
	Payload    string
	AcquiredAt time.Time
	ExpiresAt  time.Time
}
func (l Lease) Live(now time.Time) bool
func Acquire(existing *Lease, req LockRequest, now time.Time, d time.Duration) (Lease, LockOutcome)
func Release(existing *Lease, id string, now time.Time) LockOutcome
func NormalizeName(s string) (string, error)
func CanTransition(from, to Status) bool
func RetentionEligible(nonCurrentAt *time.Time, now time.Time, days int) bool
type Clock interface {
	Now() time.Time
}

// internal/crypto
func ParseMasterKey(s string) ([]byte, error)
func NewSentinel(key []byte) (string, error)
func CheckSentinel(key []byte, sentinel string) error
func NewGroupKey() (string, error)
func WrapGroupKey(groupKey string, master []byte) (string, error)
func UnwrapGroupKey(wrapped string, master []byte) (string, error)
func EncryptState(plaintext []byte, groupKey string) (string, error)
func DecryptState(ciphertext, groupKey string) ([]byte, error)

// internal/store
type Actor struct {
	Type      string
	ID        string
	RequestID string
}
type Group struct {
	ID                     string
	Name                   string
	Status                 string
	RetentionDays          int
	LockLeaseDuration      string
	RetirementRequestedAt  *time.Time
}
type Credential struct {
	ID          string
	GroupID     string
	Username    string
	Enabled     bool
	GroupStatus string
}
type State struct {
	ID               string
	GroupID          string
	Name             string
	Status           state.Status
	CurrentVersionID string
	Lock             *state.Lease
}
type Version struct {
	ID             string
	StateID        string
	FileName       string
	PlaintextBytes int64
	SHA256         string
	NonCurrentAt   *time.Time
}
func (s *Store) VerifyCredential(ctx context.Context, username, password string) (Credential, error)
func (s *Store) ReadState(ctx context.Context, groupID, name string) ([]byte, Version, error)
func (s *Store) ReadVersion(ctx context.Context, groupID, name, versionID string) ([]byte, Version, error)
func (s *Store) Publish(ctx context.Context, groupID, name string, body []byte, lockID string, actor Actor) (Version, error)
func (s *Store) Tombstone(ctx context.Context, groupID, name string, lockID string, actor Actor) error
func (s *Store) AcquireLock(ctx context.Context, groupID, name string, req state.LockRequest, actor Actor) (state.LockOutcome, *state.Lease, error)
func (s *Store) ReleaseLock(ctx context.Context, groupID, name, id string, actor Actor) (state.LockOutcome, error)

// internal/httpapi
type CredentialVerifier interface {
	VerifyCredential(ctx context.Context, username, password string) (store.Credential, error)
}
type StateStore interface {
	ReadState(ctx context.Context, groupID, name string) ([]byte, store.Version, error)
	ReadVersion(ctx context.Context, groupID, name, versionID string) ([]byte, store.Version, error)
	Publish(ctx context.Context, groupID, name string, body []byte, lockID string, actor store.Actor) (store.Version, error)
	Tombstone(ctx context.Context, groupID, name string, lockID string, actor store.Actor) error
}
type LockStore interface {
	AcquireLock(ctx context.Context, groupID, name string, req state.LockRequest, actor store.Actor) (state.LockOutcome, *state.Lease, error)
	ReleaseLock(ctx context.Context, groupID, name, id string, actor store.Actor) (state.LockOutcome, error)
}
```

`httpapi` and `store` both accept `state.Clock`; neither defines its own clock type.

---

## Slice 1: Foundation

Slice gate: the app boots with `/api` only, migrations apply from empty, the key gate refuses mismatches, crypto and lease tests pass, telemetry is a true no-op when disabled, and the Hugo site builds.

### Task 1.1: Configuration package

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `Config`, `Load(ctx context.Context) (*Config, error)`.

- [ ] **Step 1: Write the failing table test**

Test names: `TestLoadDefaults`, `TestServeRequiresMasterKey`, `TestCommandAllowsMissingMasterKey`, `TestLoadRejectsMalformedMasterKey`, `TestLoadRejectsWrongLengthMasterKey`, `TestLoadRejectsZeroGeneration`, `TestLoadRejectsBadMaxStateSize`, `TestLoadRejectsBadLockLease`, `TestLoadRejectsBadLogLevel`.

Assertions:
- With `UNITED_MASTER_KEY` set to base64 of 32 bytes, `Load(ctx, ModeServe)` returns `MaxStateSize == 15_000_000`, `LockLeaseDuration == 35*time.Minute`, `TelemetryEnabled == false`, `LogLevel == slog.LevelInfo`, `DataDir == "./pb_data"`, `ListenAddr == "0.0.0.0:8090"`, `MasterKeyGeneration == 1`.
- `ModeServe` with a missing, non-base64, 31-byte, or 33-byte key returns an error naming `UNITED_MASTER_KEY`; `ModeCommand` with the key unset returns a config with a nil `MasterKey` and the same other values.
- `UNITED_MASTER_KEY_GENERATION` of `0` or `-1` fails.
- `UNITED_MAX_STATE_SIZE` of `0`, `-1`, or `536870913` fails; `536870912` passes.
- `UNITED_LOCK_LEASE_DURATION` of `4m59s`, `24h1s`, or `banana` fails; `5m`, `24h`, and unset pass.
- `LOG_LEVEL=verbose` fails.

Use `t.Setenv` and `envconfig.Process(context.Background(), &cfg)`. Reset with `t.Setenv` per case.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/config -v`
Expected: FAIL because `Config` and `Load` do not exist.

- [ ] **Step 3: Implement `internal/config/config.go`**

Use struct tags such as `env:"UNITED_MASTER_KEY"`, `env:"UNITED_MAX_STATE_SIZE,default=15000000"`, `env:"UNITED_LOCK_LEASE_DURATION,default=35m"`, `env:"TELEMETRY_ENABLED,default=false"`, `env:"LOG_LEVEL,default=info"`, `env:"UNITED_DATA_DIR,default=./pb_data"`, `env:"UNITED_LISTEN_ADDR,default=0.0.0.0:8090"`, `env:"UNITED_MASTER_KEY_GENERATION,default=1"`, and `env:"UNITED_ACTOR"`.

Use `envconfig.Base64Bytes` for `MasterKey`; a plain `[]byte` field receives the base64 text unchanged. Reject any decoded length other than 32. In `ModeServe`, require the key to be present and non-empty; in `ModeCommand` (offline commands, which receive their keys through flags) allow it to be absent. Parse `LOG_LEVEL` into `slog.Level`. Reject a generation below 1. Validate the max-size ceiling of `512 << 20` and the lease bounds. Configuration errors are bootstrap errors and use `fmt.Errorf` naming the setting; they do not use the HTTP error classes.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/config -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config
git commit -m "feat(config): add environment configuration and validation"
```

### Task 1.2: Error classes

**Files:**
- Create: `internal/apperr/apperr.go`
- Test: `internal/apperr/apperr_test.go`

**Interfaces:**
- Produces: `Class`, the class constants, `Error`, `New`, `NewSafe`, `HTTPStatus`, `ClassOf`.

- [ ] **Step 1: Write the failing tests**

Test names: `TestHTTPStatus`, `TestClassOf`, `TestSafeMessage`.

Assertions:
- `validation` → 400, `not_found` → 404, `conflict` → 409, `gone` → 410, `unauthorized` → 401, `forbidden` → 403, `storage` → 503, `database` → 503, `crypto` → 500, `internal` → 500.
- `ClassOf` unwraps `fmt.Errorf("wrap: %w", apperr.New(...))`.
- A `Safe` error exposes `Msg`; an unsafe error exposes only the class label, never `Err.Error()`.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/apperr -v`
Expected: FAIL because the package does not exist.

- [ ] **Step 3: Implement `internal/apperr/apperr.go`**

Implement `Error` with `Error()`, `Unwrap()`, and a `SafeMessage()` method. `ClassOf` uses `errors.As`. `HTTPStatus` is a switch with a default of 500.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/apperr -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/apperr
git commit -m "feat(apperr): add closed error classes and HTTP mapping"
```

### Task 1.3: Telemetry registry

**Files:**
- Create: `internal/telemetry/registry.go`
- Test: `internal/telemetry/registry_test.go`

**Interfaces:**
- Produces: `EventSpec`, `MetricSpec`, `RegisterEvent`, `RegisterMetric`, `Events`, `Metrics`, `Validate`.

```go
type EventSpec struct {
	Name       string
	Level      slog.Level
	Outcomes   []string
	Attributes []string
	Description string
}
type MetricSpec struct {
	Name        string
	Kind        string // counter, histogram, gauge
	Unit        string
	Attributes  []string
	Description string
}
```

- [ ] **Step 1: Write the failing tests**

Test names: `TestRegistryRejectsDuplicate`, `TestRegistryRejectsMalformedName`, `TestRegistryRequiresUnitForHistogramAndGauge`, `TestRegistryRejectsUndeclaredOutcome`, `TestRegistryRejectsForbiddenAttribute`, `TestRegistryIsDeterministic`.

Assertions:
- Duplicate event or metric names fail `Validate`.
- Names must match `^united\.[a-z0-9]+(\.[a-z0-9_]+)+$` or the approved `http.server.*` and `pb.*` names.
- Histograms and gauges require a unit; counters use unit `1`.
- Every outcome used by a constructor must be declared in its `EventSpec`.
- `state.document`, `state.name`, `group.id`, `credential.id`, `lock.id`, `lock.payload`, and `state.bytes` are rejected as metric attributes.
- `Events` and `Metrics` return sorted copies.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/telemetry -run TestRegistry -v`
Expected: FAIL because the registry does not exist.

- [ ] **Step 3: Implement `internal/telemetry/registry.go`**

Seed the registry with the event and metric catalogs from #561. Keep the registry in one file; adding a signal is one entry. Use `sync.RWMutex` so tests can reset it. Export `ResetForTest()` guarded by a build tag or a `testing` helper; prefer an unexported `reset` and a test in the same package.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/telemetry -run TestRegistry -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/telemetry/registry.go internal/telemetry/registry_test.go
git commit -m "feat(telemetry): add registry-backed event and metric catalog"
```

### Task 1.4: Logger and redaction

**Files:**
- Create: `internal/telemetry/logger.go`, `internal/telemetry/redact.go`
- Test: `internal/telemetry/logger_test.go`

**Interfaces:**
- Produces: `NewLogger(level slog.Level, otelEnabled bool, otelHandler slog.Handler) *slog.Logger`, `Event(ctx context.Context, logger *slog.Logger, name, outcome string, attrs ...slog.Attr)`, `Redact(key string) bool`.

- [ ] **Step 1: Write the failing tests**

Test names: `TestLoggerWritesJSONToStdout`, `TestLoggerUsesMultiHandlerWhenEnabled`, `TestEventSetsNameAndOutcome`, `TestRedactionStripsDenylistedKeys`, `TestRedactionAllowsApprovedKeys`.

Assertions:
- Disabled telemetry writes one JSON object per line with `time`, `level`, `msg`, `otel.event.name`, and `united.outcome`.
- Enabled telemetry uses `slog.MultiHandler` with the stdout handler and the supplied OTel handler; a recording handler sees the same record.
- `authorization`, `password`, `password:hash`, `lock.id`, `lock.payload`, `who`, `state.document`, `state.bytes`, `master_key`, `wrapped_key`, and `ciphertext` are stripped.
- `united.group.id`, `united.state.name`, `united.request.id`, `united.outcome`, and `error.type` pass through.
- The redaction handler is idempotent and preserves attribute order for allowed keys.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/telemetry -run 'TestLogger|TestRedaction|TestEvent' -v`
Expected: FAIL because the logger does not exist.

- [ ] **Step 3: Implement the logger and redactor**

Use `slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})`. Wrap it in a `redactingHandler` that drops denylisted keys recursively. `Event` attaches `slog.String("otel.event.name", name)` and `slog.String("united.outcome", outcome)`.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/telemetry -run 'TestLogger|TestRedaction|TestEvent' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/telemetry/logger.go internal/telemetry/redact.go internal/telemetry/logger_test.go
git commit -m "feat(telemetry): add stdout logger, event envelope, and redaction"
```

### Task 1.5: OpenTelemetry setup

**Files:**
- Create: `internal/telemetry/otel.go`
- Test: `internal/telemetry/otel_test.go`

**Interfaces:**
- Produces: `type Shutdown func(context.Context) error`, `Setup(ctx context.Context, cfg SetupConfig) (Shutdown, slog.Handler, error)`. The returned handler is the `otelslog` handler for the logger's `MultiHandler`, or `nil` when disabled.

```go
type SetupConfig struct {
	Enabled    bool
	ServiceName string
	Version    string
	InstanceID string
	Endpoint   string
}
```

- [ ] **Step 1: Write the failing tests**

Test names: `TestSetupDisabledIsNoOp`, `TestSetupRequiresEndpointWhenEnabled`, `TestSetupRejectsPrometheusExporter`, `TestSetupBuildsProviders`, `TestSetupReturnsOTelHandler`, `TestSetupRedactsExportedLogs`.

Assertions:
- Disabled returns a non-nil `Shutdown`, a nil handler, and leaves the global tracer, meter, and logger providers as the no-op providers, without calling `autoexport`.
- Enabled returns a non-nil `slog.Handler` that writes to the OTel log pipeline.
- Enabled with any signal exported over OTLP and no `OTEL_EXPORTER_OTLP_ENDPOINT` or per-signal endpoint returns an error naming the variable. An unset `OTEL_*_EXPORTER` defaults to `otlp`, so the check covers unset as well as explicit `otlp` values, and it runs before `autoexport` is called.
- `OTEL_METRICS_EXPORTER=prometheus` returns an error.
- Enabled with `OTEL_TRACES_EXPORTER=otlp`, `OTEL_METRICS_EXPORTER=otlp`, `OTEL_LOGS_EXPORTER=otlp`, and an endpoint builds providers; `Shutdown` is idempotent.
- A log record carrying `password` never reaches the in-memory exporter.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/telemetry -run TestSetup -v`
Expected: FAIL because `Setup` does not exist.

- [ ] **Step 3: Implement `internal/telemetry/otel.go`**

Use `autoexport.NewSpanExporter`, `NewMetricReader`, and `NewLogExporter` from `go.opentelemetry.io/contrib/exporters/autoexport`. Reject `prometheus` explicitly. Build a `resource.Resource` with `service.name`, `service.version`, `service.instance.id`, and the SDK attributes. Install `autoprop.NewTextMapPropagator()`. Register a redacting log processor before the batch processor. Return a `Shutdown` that flushes traces, metrics, and logs with a 5-second timeout.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/telemetry -run TestSetup -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/telemetry/otel.go internal/telemetry/otel_test.go
git commit -m "feat(telemetry): add optional OTel setup with autoexport"
```

### Task 1.6: Crypto

**Files:**
- Create: `internal/crypto/masterkey.go`, `internal/crypto/sentinel.go`, `internal/crypto/groupkey.go`, `internal/crypto/envelope.go`
- Test: `internal/crypto/crypto_test.go`

**Interfaces:**
- Produces: `ParseMasterKey`, `NewSentinel`, `CheckSentinel`, `NewGroupKey`, `WrapGroupKey`, `UnwrapGroupKey`, `EncryptState`, `DecryptState`.

- [ ] **Step 1: Write the failing tests**

Test names: `TestParseMasterKey`, `TestSentinelRoundTrip`, `TestSentinelRejectsWrongKey`, `TestGroupKeyRoundTrip`, `TestWrapRejectsWrongMaster`, `TestEncryptDecryptRoundTrip`, `TestDecryptRejectsTamper`, `TestEncryptUsesBase64Text`, `TestEncryptRejectsWrongKeyLength`, `TestDecryptRejectsShortCiphertext`.

Assertions:
- `ParseMasterKey` accepts base64 of 32 bytes and rejects 31/33 bytes and non-base64.
- `NewSentinel` returns non-empty ciphertext; `CheckSentinel` passes with the same key and fails with a different valid key.
- `NewGroupKey` returns 32 random characters; two calls differ.
- `WrapGroupKey`/`UnwrapGroupKey` round-trip and reject a different master key.
- `EncryptState` returns standard base64 text that decodes; `DecryptState` returns the exact plaintext bytes.
- A one-byte change anywhere in the ciphertext makes `DecryptState` fail.
- `EncryptState` output never contains the plaintext.
- `EncryptState` and `DecryptState` reject keys whose decoded length is not exactly 32 bytes.
- `DecryptState` returns an error, never a panic, for empty or nonce-truncated ciphertext.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/crypto -v`
Expected: FAIL because the package does not exist.

- [ ] **Step 3: Implement the crypto package**

Use `github.com/pocketbase/pocketbase/tools/security` for `Encrypt`, `Decrypt`, and `RandomString`. This is the only PocketBase import allowed in this package (ADR 0002). `ParseMasterKey` uses `base64.StdEncoding.DecodeString`. `NewGroupKey` uses `security.RandomString(32)`. Do not define a competing cipher or associated-data scheme.

`security.Encrypt` accepts 16-, 24-, or 32-byte keys and silently selects AES-128/192/256, so both wrappers reject any key whose decoded length is not 32. `security.Decrypt` slices the first 12 bytes without a length check and panics on short input, so `DecryptState` and `UnwrapGroupKey` length-check the decoded ciphertext before calling it and return an `apperr.ClassCrypto` error instead.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/crypto -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/crypto
git commit -m "feat(crypto): add master key, sentinel, group keys, and envelope helpers"
```

### Task 1.7: Pure state domain

**Files:**
- Create: `internal/state/name.go`, `internal/state/lifecycle.go`, `internal/state/retention.go`, `internal/state/lease.go`, `internal/state/clock.go`
- Test: `internal/state/state_test.go`

**Interfaces:**
- Produces: `NormalizeName`, `Status`, `CanTransition`, `RetentionEligible`, `Lease`, `LockRequest`, `LockOutcome`, `Acquire`, `Release`.

- [ ] **Step 1: Write the failing tests**

Test names: `TestNormalizeName`, `TestCanTransition`, `TestRetentionEligible`, `TestLeaseLive`, `TestAcquire`, `TestRelease`.

Assertions:
- `NormalizeName` lowercases `MyState` to `mystate`; accepts `a`, `a.b_c-d`, 255 characters; rejects empty, 256 characters, leading `.`/`-`/`_`, `..`, `a/b`, `a\b`, whitespace, Unicode, and control characters.
- `CanTransition` allows `empty→active`, `empty→tombstoned`, `active→tombstoned`, `tombstoned→active`, `tombstoned→empty`, `tombstoned→purged`; rejects `purged→*` and `active→empty`.
- `RetentionEligible` is false at exactly `now - days`, true one second later, and false when `nonCurrentAt` is nil.
- `Lease.Live` is false at exactly `ExpiresAt`, true one nanosecond before.
- `Acquire` with no lease returns `LockAcquired`; with a live same-ID lease returns `LockIdempotent` and does not change expiry; with a live different-ID lease returns `LockContended`; with an expired lease returns `LockAcquired` with a new expiry.
- `Release` with a matching live ID returns `LockReleased`; a mismatched live ID returns `LockMismatched`; absent or expired returns `LockNotFound`.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/state -v`
Expected: FAIL because the package does not exist.

- [ ] **Step 3: Implement the pure domain**

Keep the package free of PocketBase, database, HTTP, and clock dependencies; accept `time.Time` and durations as arguments. The name regex is `^[a-z0-9][a-z0-9._-]{0,254}$`.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/state -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/state
git commit -m "feat(state): add state names, lifecycle, retention, and lease domain"
```

### Task 1.8: Migration schema

**Files:**
- Create: `internal/migrations/202611210001_schema.go`
- Test: `internal/migrations/migrations_test.go`

**Interfaces:**
- Produces: the collections, fields, indexes, and seeded singleton described below. Collection IDs come from PocketBase's deterministic `pbc_<crc32>` name checksum, so no hardcoded IDs are needed.

- [ ] **Step 1: Write the failing schema test**

Test name: `TestMigrationSchema`.

Use `tests.NewTestApp(t.TempDir())` from `github.com/pocketbase/pocketbase/tests`, then assert:

- Collections exist: `groups`, `group_memberships`, `group_invitations`, `terraform_credentials`, `logical_states`, `state_versions`, `audit_events`, `system_settings`, `retired_groups`.
- `terraform_credentials.password` is a `PasswordField` with `Hidden=true`, `Required=true`, `Min>=32`.
- `groups.wrapped_key`, `logical_states.lock_payload`, and `system_settings.key_sentinel` are hidden.
- The intent fields `groups.confirm_destructive` and `terraform_credentials.rotate_credential` are neither hidden nor system fields; `OnRecordEnrich` strips them from responses.
- `state_versions.file` is a `FileField` with `Protected=true`, `MaxSelect=1`, and `MaxSize >= 800<<20`.
- Unique indexes exist on `(group, name)` for `logical_states`, `(user, group)` for `group_memberships`, `(group, user)` for `group_invitations`, `group` for `terraform_credentials`, `username` for `terraform_credentials`, `group_id` and `credential_username` for `retired_groups`.
- `audit_events` has indexes on `(group_id, created)`, `(event_type, created)`, and `(actor_id, created)`.
- `state_versions` has an index on `(logical_state, created, id)`; `logical_states` has indexes on `non_current_at`, `lock_id`, and `lock_expires_at`.
- `system_settings` has exactly one record with `master_key_generation=1` and `invitation_ttl_days=7`.
- The built-in `users` collection has a `disabled` bool defaulting to false, and `authRule` is `verified = true && disabled != true`.
- Every domain collection has `nil` create/update/delete rules in this slice; reads are also `nil` until Task 3.1.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/migrations -v`
Expected: FAIL because the migration does not exist.

- [ ] **Step 3: Implement the migration**

Create the collections with `core.NewBaseCollection`. Use `core.TextField`, `core.SelectField`, `core.NumberField`, `core.BoolField`, `core.DateField`, `core.RelationField`, `core.FileField`, `core.PasswordField`, and `core.JSONField`. Use `app.FindCollectionByNameOrId("users")` for the built-in user relation. Create parents before children so relation IDs resolve. The `logical_states.current_version` relation is circular, so create `logical_states` without it, create `state_versions`, then add the relation field to `logical_states` and save it again. Add indexes with `Collection.AddIndex`. Seed `system_settings` with one record. Find the built-in `users` collection with `app.FindCollectionByNameOrId("users")`, add the `disabled` field, set its rules, and save it.

Field set:

- `groups`: `name` text required max 100; `status` select `active|suspended|pending_retirement`; `retention_days` number min 1 max 365; `lock_lease_duration` text optional; `wrapped_key` text required hidden; `retirement_requested_at` date optional; `confirm_destructive` bool optional (not hidden; write-only via enrichment); autodates.
- `group_memberships`: `user` relation users required cascade; `group` relation groups required cascade; `role` select `owner|member`; autodates.
- `group_invitations`: `group` relation groups required cascade; `user` relation users required cascade; `email` email required; `invited_by` text required; `status` select `pending`; `expires_at` date required; autodates.
- `terraform_credentials`: `group` relation groups required cascade; `username` text required; `password` password system hidden required min 32 max 71 cost 10 (PocketBase rejects a maximum above 71); `enabled` bool; `issued_at` date required; `rotated_at`, `disabled_at`, `enabled_at` dates optional; `rotate_credential` bool optional (not hidden; write-only via enrichment).
- `logical_states`: `group` relation groups required cascade; `name` text required min 1 max 255; `status` select `empty|active|tombstoned|purged`; `current_version` relation `state_versions` max 1; `tombstoned_at`, `purged_at` dates optional; `lock_payload` text hidden max 1 MiB; `lock_id` text optional; `lock_acquired_at`, `lock_expires_at` dates optional; autodates.
- `state_versions`: `logical_state` relation logical_states required cascade; `file` file required protected max 1 max size `800<<20`; `plaintext_bytes` number required min 0 max 536870912 only int; `sha256` text required min 64 max 64; `non_current_at` date optional; autodates.
- `audit_events`: `event_type` text required max 64; `schema_version` number required min 1 only int; `actor_type` select `user|superuser|terraform_credential|system|offline_operator`; `actor_id` text required; `subject_type` text required; `subject_id` text required; `group_id` text optional; `request_id` text optional; `details` json optional max 64 KiB; validators enforce the typed shape, and an event with no typed details stores `{}`; autodates.
- `system_settings`: `master_key_generation` number required min 1 only int; `key_sentinel` text hidden max 4096; `invitation_ttl_days` number required min 1 max 90; autodates.
- `retired_groups`: `group_id` text required; `credential_username` text required; `created` date required; `retirement_requested_at` date required; `purged_at` date required.
- `users` (PocketBase built-in, extended): add `disabled` bool default false; set `authRule` to `verified = true && disabled != true`; registration create open to guests; list, view, and update self only. Field-level update control belongs to the Task 3.9 hook.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/migrations -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/migrations
git commit -m "feat(migrations): add the full PocketBase domain schema"
```

### Task 1.9: App bootstrap, composition root, and v1 removal

**Files:**
- Create: `cmd/united/main.go`, `internal/wire/wire.go`, `.github/workflows/smoke.yml`, `.github/workflows/test.yml`
- Modify: `mise.toml`, `.goreleaser.yaml`, `Dockerfile`, `Makefile`, `.air.toml`, `README.md`, `AGENTS.md`, `.gitignore`, `.renovaterc.json`, `.golangci.yml`, `docker-compose.yml`, `tests/Makefile`
- Delete: `main.go`, `config.go`, `handlers.go`, `middlewares.go`, `util.go`, `tests/nginx.conf`, `.github/workflows/terrafrom-test.yml`
- Test: `internal/wire/wire_test.go`

**Interfaces:**
- Produces: `wire.Register(app core.App, deps Deps) error` and `wire.Deps`.

```go
type Deps struct {
	Config   *config.Config
	Logger   *slog.Logger
	Shutdown telemetry.Shutdown
}
```

- [ ] **Step 1: Write the failing boot test**

Test names: `TestBootServesAPIHealth`, `TestBootReturns404ForStateBeforeSlice2`, `TestBootRunsMigrations`, `TestBootWritesConfiguredGeneration`, `TestBootRefusesKeyMismatch`, `TestBootRefusesCorruptSentinel`.

Use `tests.NewTestApp(t.TempDir())`, call `wire.Register(app, deps)`, trigger `OnServe` into a `httptest.NewServer` through the built mux, and assert:
- `GET /api/health` returns 200.
- `GET /state/group/name` returns 404 in slice 1.
- The app migrations ran and the collections exist.
- A fresh data directory with `UNITED_MASTER_KEY_GENERATION=2` boots, writes generation 2 and the sentinel, and serves.
- A second app built with `pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: first.DataDir()})` and a different master key refuses to boot with a key-gate error.
- A corrupt `key_sentinel` refuses to boot.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/wire -v`
Expected: FAIL because `wire` does not exist.

- [ ] **Step 3: Implement the entry point and composition root**

`cmd/united/main.go`:

```go
func main() {
	base, err := config.Load(context.Background(), config.ModeCommand) // data dir and listen address only
	if err != nil { ... }
	app := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: base.DataDir})

	serve := cmd.NewServeCommand(app, false)
	serve.PreRunE = func(*cobra.Command, []string) error {
		cfg, err := config.Load(context.Background(), config.ModeServe)
		if err != nil { return err }
		shutdown, otelHandler, err := telemetry.Setup(context.Background(), telemetry.SetupConfig{...})
		if err != nil { return err }
		logger := telemetry.NewLogger(cfg.LogLevel, cfg.TelemetryEnabled, otelHandler)
		return wire.Register(app, wire.Deps{Config: cfg, Logger: logger, Shutdown: shutdown})
	}
	if f := serve.PersistentFlags().Lookup("http"); f != nil {
		f.DefValue = base.ListenAddr
		if err := f.Value.Set(base.ListenAddr); err != nil { ... }
	}
	app.RootCmd.AddCommand(serve)
	app.RootCmd.AddCommand(cmd.NewSuperuserCommand(app))
	migratecmd.MustRegister(app, app.RootCmd, migratecmd.Config{Automigrate: false})
	offline.Register(app.RootCmd, ...) // config.ModeCommand; never OTel

	if err := app.Execute(); err != nil { ... }
}
```

`app.Execute()` does not register `serve` or `superuser` (that is `app.Start()`), so United adds `cmd.NewServeCommand`, `cmd.NewSuperuserCommand`, and the `migratecmd` plugin itself. PocketBase v0.40.3 has no `backup` CLI command; backups are the superuser HTTP routes and the `backups.cron` setting. Configuration and telemetry are loaded inside the serve command, never before command dispatch, so an offline command cannot start OTel. `HideStartBanner` has no effect under `Execute()` and is not set.

`wire.Register` binds `OnServe` to the key gate, route registration, and cron registration, `OnTerminate` to shutdown with a 5-second flush timeout, and imports `internal/migrations` for its side effect. It does not bind `OnBootstrap`; application migrations have not run at that point (ADR 0002). In slice 1, `OnServe` registers no custom routes.

Delete the v1 files and run `go mod tidy`; assert `go.mod` no longer contains Gin, the AWS SDK, the S3 encryption client, or Redis. Add the `depguard` rule to `.golangci.yml`, including the `tools/security` allow entry. Update `mise.toml` build to `go build -o dist/united ./cmd/united`, `.goreleaser.yaml` to `main: ./cmd/united`, `Dockerfile` to keep `COPY united /` (goreleaser stages the binary at the build-context root) and add `CMD ["serve"]`, and `.air.toml` to build `./cmd/united` and run `serve` with a development `UNITED_MASTER_KEY`. Rewrite the `Makefile` `run` target to start air with `UNITED_MASTER_KEY` set and without `AWS_PROFILE`, `BUCKET`, `KEY_ARN`, or `AUTH_URL`. Reduce `docker-compose.yml` to the LocalStack service that the S3 storage tests need, and delete `tests/nginx.conf` with the v1 auth mock. Rewrite `README.md` to a short overview plus pointers to `site/` and `docs/`, and rewrite `AGENTS.md` with the new repository map, commands, and configuration.

Before deleting `.github/workflows/terrafrom-test.yml`, update the `main` branch required status checks to the workflows that replace it (`smoke` and `terraform-v2`); `terraform-test` is currently required and would block every pull request once its workflow is gone. Add `.github/workflows/test.yml` running `go build ./...` and `go test ./...` on pull requests, and make `smoke.yml` boot the binary, check `/api/health`, and assert the migration schema. Add `pb_data/`, `site/public/`, and `site/resources/` to `.gitignore`, and disable Renovate automerge for PocketBase, OpenTelemetry, Hugo, and Hextra so pinned versions move deliberately. Restrict the Docker tag templates in `.goreleaser.yaml` to `{{ .Tag }}` and `{{ .Major }}.{{ .Minor }}` for this release, so `latest` and `v0` keep pointing at the v0.1.7 image.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/wire -v && go build ./...`
Expected: PASS and a clean build.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat: boot PocketBase and remove the Gin/S3/Redis implementation"
```

### Task 1.10: Documentation scaffold and ADR

**Files:**
- Create: `site/hugo.yaml`, `site/go.mod`, `site/go.sum`, `site/content/_index.md`, `site/content/how-to/_index.md`, `site/content/reference/_index.md`, `site/content/explanation/_index.md`, `site/content/tutorials/_index.md`, `.markdownlint-cli2.yaml`, `.github/workflows/docs.yml`
- Modify: `docs/adr/0001-embed-pocketbase-in-process.md` (already committed; extend it)
- Modify: `README.md`
- Test: `site/` build

- [ ] **Step 1: Write the failing docs check**

Add a `site` build step to the standard gate. Run `hugo --panicOnWarning --printPathWarnings` in `site/` and expect it to fail because the site does not exist.

- [ ] **Step 2: Create the Hextra scaffold**

Pin `hugo-extended` `0.166.0` in `mise.toml` (the mise tool name is `hugo-extended`, not `hugo`). Add Hextra `v0.13.0` as a Hugo module in `site/go.mod`. Create the four Diátaxis sections and a landing page outside them. Configure FlexSearch, breadcrumbs, TOC, code copy, edit links to `main`, and tree navigation with explicit weights. Add `docs:serve`, `docs:build`, and `docs:check` tasks to `mise.toml`, pin `markdownlint-cli2` and `lychee` there, and add `.github/workflows/docs.yml` running `mise run docs:check` when `site/` or `docs/` changes. Do not add a version dropdown yet; it arrives with the first release.

- [ ] **Step 3: Write the ADR**

Extend the committed ADR to record: one embedded PocketBase process owns the listener and `/api`; United's domain packages stay PocketBase-free; v1 source is removed in slice 1 and survives only in `version-0.1x`, tags, and releases; the release is `v0.2.0`. Include the alternatives (separate service, long-lived branch) and why they were rejected.

- [ ] **Step 4: Run the build**

Run: `cd site && hugo --panicOnWarning --printPathWarnings`
Expected: PASS with zero warnings.

- [ ] **Step 5: Commit**

```bash
git add site docs/adr README.md mise.toml .markdownlint-cli2.yaml .github/workflows/docs.yml
git commit -m "docs: scaffold the Hugo site and record the embedded PocketBase ADR"
```

---

## Slice 2: Terraform Data Plane

Slice gate: the real Terraform harness passes init, plan, apply, refresh, state pull, destroy, the migration test, and the lock matrix; every contract and auth case passes.

### Task 2.1: Integration harness and credential verification

**Files:**
- Create: `tests/integration/harness.go`, `internal/store/store.go`, `internal/store/credentials.go`, `internal/store/groups.go`, `internal/store/audit.go`
- Test: `internal/store/credentials_test.go`, `tests/integration/harness_test.go`

**Interfaces:**
- Produces: `store.New(app core.App, cfg *config.Config, logger *slog.Logger, clock state.Clock) *Store`, `Store.VerifyCredential`, `Store.CreateGroup(ctx, actor Actor, name string) (Group, string, error)`, `tests/integration.NewApp(t *testing.T) (*tests.TestApp, *wire.Deps)`.

- [ ] **Step 1: Write the failing tests**

Test names: `TestVerifyCredentialSuccess`, `TestVerifyCredentialUnknownUser`, `TestVerifyCredentialWrongPassword`, `TestVerifyCredentialDisabled`, `TestVerifyCredentialSuspendedGroup`, `TestVerifyCredentialUnknownUserDoesBcryptWork`, `TestCreateGroupFixture`, `TestCreateGroupFixtureAudits`.

Assertions:
- A seeded group and credential verify with the correct password.
- Unknown user and wrong password both return the same `apperr.ClassUnauthorized` and take comparable time (measure with a generous bound, at least 10 ms, not a strict benchmark).
- Disabled returns unauthorized; suspended group returns a `Credential` with `GroupStatus="suspended"` so the handler can return 403.
- The password hash is bcrypt and the plaintext never appears in the record.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/store -run 'TestVerifyCredential|TestCreateGroupFixture' -v`
Expected: FAIL because the store does not exist.

- [ ] **Step 3: Implement the harness and verifier**

The harness wraps `tests.NewTestApp(t.TempDir())`, calls `wire.Register`, and returns the app. The verifier finds the credential by `username`, calls `Record.ValidatePassword`, checks `enabled`, and loads the group status. For an unknown username, validate against a fixed `core.PasswordFieldValue{Hash: dummy}` so the bcrypt cost matches. Never log the password.

Implement `Store.CreateGroup` in the same task because the harness needs it: one `RunInTransaction` creates the group, the creator's owner membership, the wrapped data key, the credential with a 32-byte base64url secret, and the `group_created` audit event. Slice 3 adds the API hook and the one-time secret response on top of it.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/store -run 'TestVerifyCredential|TestCreateGroupFixture' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add tests/integration internal/store
git commit -m "feat(store): add integration harness and Terraform credential verification"
```

### Task 2.2: State read path and `GET`

**Files:**
- Create: `internal/store/states.go`, `internal/httpapi/router.go`, `internal/httpapi/auth.go`, `internal/httpapi/state.go`, `internal/httpapi/errors.go`
- Test: `internal/httpapi/state_test.go`, `internal/store/states_test.go`

**Interfaces:**
- Produces: `httpapi.New(deps Deps) http.Handler`, `httpapi.Register(e *core.ServeEvent, deps Deps)`, the `CredentialVerifier`, `StateStore`, and `LockStore` ports, and the Basic Auth middleware.

```go
type Deps struct {
	Verifier CredentialVerifier
	States   StateStore
	Locks    LockStore
	Clock    state.Clock
	Logger   *slog.Logger
	Config   *config.Config
}
```

- [ ] **Step 1: Write the failing tests**

Test names: `TestGetReturnsStateBytes`, `TestGetReturns404ForMissingState`, `TestGetReturns404ForTombstonedState`, `TestGetReturns401WithoutAuth`, `TestGetReturns401ForWrongGroup`, `TestGetReturns403ForSuspendedGroup`, `TestGetDoesNotReturn404ForAuthFailure`, `TestGetReturnsContentMD5`, `TestStateMiddlewareStripsLockIDFromURL`, `TestReadStateReturnsCurrent`, `TestReadStateRejectsTombstoned`, `TestReadStateFailsClosedOnTamper`.

Assertions:
- A published state returns the exact bytes, `Content-Type: application/json`, and a correct base64 `Content-MD5`.
- Missing, empty, tombstoned, and purged states return 404.
- Missing, malformed, unknown, or wrong-password auth returns 401 with `WWW-Authenticate`.
- A valid credential for another group returns 401, never 404.
- A suspended or pending-retirement group returns 403.
- The handler never writes state bytes on an auth or integrity failure.
- A rejected request carrying `?ID=` (for example a 401) leaves no lock ID in the URL that the PocketBase activity logger records.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/httpapi -run TestGet -v`
Expected: FAIL because the handlers do not exist.

- [ ] **Step 3: Implement the read path**

`httpapi.Register` creates one state route group and binds a middleware before authentication that copies `?ID=` into the request context and clears `e.Request.URL.RawQuery`, so a request rejected by authentication, body limit, or panic never logs a lock ID. The Basic Auth middleware then parses the header, calls `VerifyCredential`, compares `credential.GroupID` to the route group, and stores the credential in the request context. Handlers read the lock ID from the context. Bind `apis.BodyLimit(limit)` on the group, where `limit` is the base64 size of `UNITED_MAX_STATE_SIZE` plus 64 KiB, so the default 32 MiB limit never caps a state. Register `GET /state/{group}/{name}`, an explicit `HEAD` route returning 405, and a method-less fallback returning 405 with `Allow`. Unbind `apis.DefaultCorsMiddlewareId` on the group so `OPTIONS` reaches the fallback with an `Allow` header instead of the CORS 204. The handler calls `ReadState`, maps `apperr` classes to statuses, and writes `Content-MD5`. `ReadState` finds the logical state by `(group, name)`, requires `status=active`, decrypts the current version, verifies length and SHA-256, and returns the bytes.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/httpapi ./internal/store -run 'TestGet|TestReadState|TestStateMiddleware' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/httpapi internal/store/states.go
git commit -m "feat(httpapi): add Basic Auth and Terraform state reads"
```

### Task 2.3: Publication and `POST`/`PUT`

**Files:**
- Create: `internal/store/versions.go`
- Modify: `internal/httpapi/state.go`
- Test: `internal/store/versions_test.go`, `internal/httpapi/state_test.go`

**Interfaces:**
- Produces: `Store.Publish`, the publication transaction, and the `logical_state_created` and `state_version_published` audit events.

- [ ] **Step 1: Write the failing tests**

Test names: `TestPublishCreatesVersion`, `TestPublishSupersedesCurrent`, `TestPublishIsLinearizable`, `TestPublishRejectsOversize`, `TestPublishRejectsContentLengthMismatch`, `TestPublishRejectsBadContentMD5`, `TestPublishRequiresLockIDWhenLive`, `TestPublishRejectsStaleLockID`, `TestPublishRejectsTombstonedState`, `TestPublishRejectsPurgedState`, `TestPublishExtendsLease`, `TestPublishNeverShortensLease`, `TestPublishAfterLockEmitsVersionEvent`, `TestPublishRollsBackOnAuditFailure`, `TestPublishAcceptsPUT`, `TestPostAndPutReturn200`.

Assertions:
- The first unlocked write creates `logical_states` with `status=active`, one version, and one `logical_state_created` audit event carrying the version ID.
- A second write creates a new version, sets `non_current_at` on the prior version, updates `current_version`, and writes `state_version_published`.
- A reader started before a write sees the old version; after commit it sees the new version.
- `Content-Length` above the configured limit returns 413 before reading; a chunked body crossing the limit returns 413; a declared length that differs from the received length returns 400.
- A bad `Content-MD5` returns 400.
- A live lease requires the exact `?ID=`; missing or mismatched returns 409; a stale ID with no live lease returns 409.
- An injected audit failure rolls back the version and pointer.
- A write to a tombstoned state returns `409`; a write to a purged state returns `410`; neither changes the state record.
- A successful write under a live lease sets `lock_expires_at` to the later of its current value and `now + effective duration`, in the same transaction, and never shortens an existing expiry (ADR 0002).
- A first write to a lineage created empty by `LOCK` emits exactly one `state_version_published` event and no second `logical_state_created` event.
- `POST` and `PUT` return `200` with an empty body.
- `PUT` behaves exactly like `POST`.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/store -run TestPublish -v`
Expected: FAIL because publication does not exist.

- [ ] **Step 3: Implement publication**

Read and validate the body before opening the transaction. Compute byte count and SHA-256. In `RunInTransaction` with `txApp`: recheck group status, logical-state status (`tombstoned` returns 409, `purged` returns 410), and lock authority; create the logical state when absent; unwrap the group key; encrypt; create the version record with the file field; save; set the prior version's `non_current_at`; set `current_version` and `status=active`; extend the live lease to `max(current expiry, now + effective duration)` when a lease is live; insert exactly one audit event (`logical_state_created` only when this write created the lineage, otherwise `state_version_published`); commit. Use `txApp` for every read and write inside the transaction.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/store -run TestPublish -v && go test ./internal/httpapi -run TestPostAndPut -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store internal/httpapi
git commit -m "feat(store): publish immutable encrypted state versions"
```

### Task 2.4: Locks and `LOCK`/`UNLOCK`

**Files:**
- Create: `internal/store/locks.go`, `internal/httpapi/lock.go`
- Test: `internal/store/locks_test.go`, `internal/httpapi/lock_test.go`

**Interfaces:**
- Produces: `Store.AcquireLock`, `Store.ReleaseLock`, and the lock HTTP handlers.

- [ ] **Step 1: Write the failing tests**

Test names: `TestLockAcquire`, `TestLockCreatesEmptyState`, `TestLockCreatesStateAudit`, `TestLockConcurrentAcquire`, `TestLockSameIDIsIdempotent`, `TestLockContentionReturns423WithExactJSON`, `TestLockAfterExpiry`, `TestLockOnTombstonedReturns410`, `TestLockRequiresContentType`, `TestLockRejectsWrongContentType`, `TestLockRejectsBadContentMD5`, `TestLockRejectsMissingFields`, `TestLockRejectsOversizeBody`, `TestLockAcceptsEmptyWhoPathInfo`, `TestLockAcceptsMigrationOperation`, `TestUnlockMatchingID`, `TestUnlockMismatchedID`, `TestUnlockConflictOmitsPayload`, `TestUnlockIDOnlyPayload`, `TestUnlockMalformedJSONReturns400`, `TestUnlockAbsentLeaseReturns200`, `TestUnlockEmptyBodyReturns400`, `TestMethodMatrixReturns405`.

Assertions:
- A valid payload acquires a lease with the group override or system duration and returns 200.
- A first `LOCK` against an unused name atomically creates the empty logical state, permanently reserves the name, and writes one `logical_state_created` audit event.
- A same-ID replay returns 200 without changing the stored payload or expiry.
- A different live ID returns 423 with the exact stored JSON, byte for byte.
- An expired lease is acquirable by the same or a different ID.
- `LOCK` on `tombstoned` or `purged` returns 410 with a non-LockInfo body.
- Missing `Content-Type`, missing `ID`/`Operation`/`Version`, or an invalid `Created` returns 400.
- Empty `Info`, `Path`, and `Who` are accepted.
- `UNLOCK` with the matching live ID returns 200; mismatch returns 409; absent returns 200; an empty body returns 400.
- `Operation` values `migration source state` and `migration destination state` are accepted.
- Two concurrent acquisitions with different IDs produce exactly one `200` and one `423`.
- A wrong `Content-Type`, a `Content-MD5` mismatch, or a body larger than 1 MiB is rejected instead of reaching the store.
- `UNLOCK` reads the ID from the JSON body, accepts an ID-only object (empty `Operation`, `Version`, `Info`, `Path`, and `Who`, zero `Created`), and rejects a body with no non-empty `ID` with `400`, even when no lease exists.
- A mismatched live ID returns `409` with no lock payload.
- `PATCH`, `HEAD`, `OPTIONS`, and any other method on `/state/{group}/{name}` returns 405 with `Allow: GET, POST, PUT, DELETE, LOCK, UNLOCK`.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/store -run TestLock -v`
Expected: FAIL because lock persistence does not exist.

- [ ] **Step 3: Implement lock persistence and handlers**

Validate `LOCK` and `UNLOCK` with separate rules: `LOCK` requires `ID`, `Operation`, `Version`, and a valid `Created`; `UNLOCK` requires only a non-empty `ID` in the JSON body. Preserve the raw `LOCK` JSON. Acquire inside a short `RunInTransaction` that checks group and state status and the current lease, and write the creation audit event when the state is new. Use the `state.Acquire` and `state.Release` domain functions. Write the `423` body directly with `e.Response.WriteHeader(423)` and the exact stored JSON rather than through `ApiError.RawData()`, so PocketBase's activity logger never persists lock JSON. Bind `apis.BodyLimit(1<<20)` on the lock routes. Register `LOCK` and `UNLOCK` with `e.Router.Route`, add the explicit `HEAD` route and method-less fallback from Task 2.2, and verify `OPTIONS` reaches the fallback after the CORS middleware is unbound.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/store -run TestLock -v && go test ./internal/httpapi -run 'TestLock|TestUnlock|TestMethod'`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/locks.go internal/httpapi/lock.go
git commit -m "feat: add Terraform lock leases and LOCK/UNLOCK handlers"
```

### Task 2.5: Tombstone and `DELETE`

**Files:**
- Modify: `internal/store/states.go`, `internal/httpapi/state.go`
- Test: `internal/store/states_test.go`, `internal/httpapi/state_test.go`

**Interfaces:**
- Produces: `Store.Tombstone` and the `DELETE` handler.

- [ ] **Step 1: Write the failing tests**

Test names: `TestDeleteTombstonesState`, `TestDeleteEmptyCreatesTombstone`, `TestDeleteRequiresLockIDWhenLive`, `TestDeleteWithoutLiveLeaseIsIdempotent`, `TestDeleteStaleIDWithoutLeaseSucceeds`, `TestDeleteNeverCreatedIsNoOp`, `TestDeleteRepeatIsNoOp`, `TestDeletePurgedReturns200`, `TestDeleteClearsMatchingLease`, `TestDeleteWritesAuditEvent`.

Assertions:
- A live lease requires `?ID=`; missing or mismatched returns 409.
- A matching delete tombstones the state, clears the lease, sets `non_current_at` on the current version, and writes the tombstone audit event atomically.
- Repeating `DELETE` returns 200.
- `DELETE` on a never-created path returns 200 and creates no record.
- `DELETE` on an empty logical state transitions it to an empty tombstone and writes the audit event.
- A repeat `DELETE` on a tombstoned or purged state returns `200` without changing `tombstoned_at`, `non_current_at`, or writing another audit event.
- A stale `?ID=` with no live lease returns `200`.
- A failed audit rolls back the tombstone.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/store ./internal/httpapi -run TestDelete -v`
Expected: FAIL because tombstoning does not exist.

- [ ] **Step 3: Implement tombstoning**

In one `RunInTransaction`: recheck group/state/lock; if the state is absent or already tombstoned or purged, return success without writing; otherwise set `status=tombstoned`, `tombstoned_at`, clear lock fields, set the current version's `non_current_at` when a current version exists, keep `current_version` as the restoration target, and insert the audit event. An `empty` state becomes an empty tombstone rather than a no-op.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/store ./internal/httpapi -run TestDelete -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store internal/httpapi
git commit -m "feat: tombstone logical states through Terraform DELETE"
```

### Task 2.6: Owner download route

**Files:**
- Create: `internal/httpapi/download.go`
- Test: `internal/httpapi/download_test.go`

**Interfaces:**
- Produces: `GET /download/{group}/{name}/{version}` bound to `apis.RequireAuth("users")`.

- [ ] **Step 1: Write the failing tests**

Test names: `TestDownloadOwnerGetsVersion`, `TestDownloadMemberDenied`, `TestDownloadNonMemberDenied`, `TestDownloadSuspendedGroupDenied`, `TestDownloadTamperedFileFailsClosed`, `TestDownloadMissingFileFailsClosed`.

Assertions:
- An owner of an active group gets the exact bytes for any retained version ID.
- A member, a non-member, and an unauthenticated user get 403/401.
- A suspended or pending-retirement group returns 403.
- A tampered or missing file returns 500 with no bytes and no fallback.
- Direct `/api/files/...` access returns 403 for every principal, including superuser.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/httpapi -run TestDownload -v`
Expected: FAIL because the route does not exist.

- [ ] **Step 3: Implement the route**

Bind `apis.RequireAuth("users")`, load the group and membership, require `role=owner` and `status=active`, then call `ReadVersion`. `ReadVersion` resolves the version by ID, ensures it belongs to the named state, decrypts, verifies length and SHA-256, and returns the bytes. Register `OnFileDownloadRequest` to reject every `/api/files/...` download.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/httpapi -run TestDownload -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/httpapi/download.go internal/httpapi/download_test.go internal/hooks/files.go
git commit -m "feat: add owner raw-state download and deny direct file access"
```

### Task 2.7: State hooks and hidden fields

**Files:**
- Create: `internal/hooks/records.go`, `internal/hooks/enrich.go`
- Test: `internal/hooks/hooks_test.go`

**Interfaces:**
- Produces: `hooks.Register(app core.App)` binding record validation and enrichment for state, version, and lock invariants.

- [ ] **Step 1: Write the failing tests**

Test names: `TestVersionFieldsImmutable`, `TestLogicalStateCombinationValid`, `TestLockFieldsAllOrNone`, `TestIntentFieldsNeverSerialize`, `TestEnrichHidesSecrets`, `TestEnrichAddsLockMetadata`, `TestEnrichHidesFileFields`.

Assertions:
- A direct `app.Save` that changes a version's file, byte count, SHA-256, or logical-state relation is rejected.
- Committed combinations follow #415: empty has no versions and no pointer; active has a current version with no `non_current_at`; tombstoned may keep the former current version with `non_current_at`; purged has no pointer or versions.
- `OnRecordEnrich` hides `wrapped_key`, `password`, `lock_payload`, and file fields; it adds `lock_id`, `lock_acquired_at`, `lock_expires_at`, and current/non-current status.
- A generic REST view of `state_versions` never exposes the file name or file URL.
- `OnRecordEnrich` strips `groups.confirm_destructive` and `terraform_credentials.rotate_credential` after the default enrichment, for every principal including superusers, and both fields are `false` after every governed save.
- A logical-state save that sets some but not all lock fields is rejected.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/hooks -v`
Expected: FAIL because the hooks do not exist.

- [ ] **Step 3: Implement the hooks**

Bind `OnRecordValidate` and `OnRecordUpdate` for `state_versions` and `logical_states`. Reject changes to immutable fields, changes to `non_current_at` outside a lifecycle command, and partial lock-field writes. Bind `OnRecordEnrich` for the domain collections and strip the two intent fields after `e.Next()` (PocketBase unhides fields for superusers during default enrichment, so the strip must follow it). These validators prevent accidents and data corruption; they are not a containment boundary against a superuser (ADR 0002). Keep the hooks idempotent and free of secrets in errors.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/hooks -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/hooks
git commit -m "feat(hooks): enforce state invariants and hide protected fields"
```

### Task 2.8: Terraform harness

**Files:**
- Create: `tests/smoke/`, rewrite `tests/main.tf`, `tests/Makefile`
- Create: `tests/backends/main.tf`, `cmd/united/devseed.go`, `.github/workflows/terraform-v2.yml`, `.github/workflows/terraform-matrix.yml`
- Modify: `mise.toml` (add `opentofu`)
- Test: `tests/` harness

- [ ] **Step 1: Write the failing harness**

Point `tests/main.tf` at `http://localhost:8090/state/{group-id}/me`, substituting the record ID that `dev-seed` prints, with `lock_address` and `unlock_address` on the same route. The harness creates the group and credential through `dev-seed`, exports the Basic Auth values, and runs `terraform init`, `plan`, `apply`, `refresh`, `state pull`, `destroy`, and a second `state pull`. Add `tests/backends/main.tf` for the migration test: a local backend fixture with a known state, then the same resources with the HTTP backend, switched with `terraform init -migrate-state`.

- [ ] **Step 2: Run the harness to verify it fails**

Run: `make -C tests`
Expected: FAIL until the setup command and route are wired.

- [ ] **Step 3: Add the setup command and workflow**

Add `united dev-seed` as a hidden cobra subcommand in `cmd/united/devseed.go`, guarded by `DEV=true`, that calls `Store.CreateGroup`, prints the group record ID and the Basic Auth values, and refuses to run without `DEV=true`.

Add `.github/workflows/terraform-v2.yml` that starts the binary and runs the harness, the migration test, and the lock matrix on Terraform 1.17. Add `.github/workflows/terraform-matrix.yml` with a matrix of Terraform 1.16 and 1.17 plus OpenTofu 1.12 and 1.13, triggered after the unit-test workflow succeeds, with `opentofu` added to `mise.toml`. The matrix covers the full harness, the migration test, the lock-payload matrix including the two `migration ... state` operations and an empty-body `UNLOCK`, the method matrix, the 15,000,000-byte limit at and one byte over, and force-unlock: OpenTofu succeeds, and Terraform is expected to fail because upstream ignores the supplied ID. Keep the smoke workflow for boot and health.

The migration test archives a pre-switch `terraform state pull`, switches with `init -migrate-state`, then requires a post-switch `plan` with no changes and an identical `state list`. It also covers a failed destination POST, which must leave the source state and `.terraform/terraform.tfstate` untouched, and a retry.

- [ ] **Step 4: Run the harness to verify it passes**

Run: `make -C tests`
Expected: PASS for init, plan, apply, state pull, destroy, and the final state pull.

- [ ] **Step 5: Commit**

```bash
git add tests .github/workflows/terraform-v2.yml
git commit -m "ci: add the Terraform v2 integration harness and client matrix"
```

---

## Slice 3: Administration and Lifecycle

Slice gate: the rule matrix passes for anonymous, verified non-member, member, owner, and superuser across every collection; lifecycle invariants, the account lifecycle, and the full audit catalog pass.

### Task 3.1: Rules migration

**Files:**
- Create: `internal/migrations/202611210002_rules.go`
- Test: `internal/migrations/rules_test.go`

**Interfaces:**
- Produces: membership-scoped list/view rules and owner-scoped update rules on every domain collection.

- [ ] **Step 1: Write the failing rule matrix**

Test names: `TestRulesMatrix`, `TestRulesLockedCollections`.

For each collection and each principal, assert allowed/denied on list, view, create, update, and delete. The principals are anonymous, verified non-member, member, owner, and superuser. Cover two groups and a user in both.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/migrations -run TestRules -v`
Expected: FAIL because all rules are still `nil`.

- [ ] **Step 3: Implement the rules migration**

Set list/view rules to membership-scoped expressions, update rules to owner-scoped expressions where #416 allows them, and leave create/delete locked except for these paths: `groups` create for any authenticated user; `group_invitations` update for the targeted user accepting; `group_invitations` delete for an owner or the target revoking; `group_memberships` delete for an owner or the member leaving; `users` guest registration create, self-only list/view/update, with the field allowlist and lifecycle transitions enforced by the Task 3.9 hook. Keep `audit_events`, `retired_groups`, and `system_settings` superuser-only. Add the expressions as constants and test them with the real query engine.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/migrations -run TestRules -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/migrations
git commit -m "feat(migrations): add membership-scoped collection rules"
```

### Task 3.2: Group creation and one-time credential secret

**Files:**
- Create: `internal/hooks/groups.go`
- Modify: `internal/store/groups.go`
- Test: `internal/store/groups_test.go`, `internal/hooks/groups_test.go`

**Interfaces:**
- Produces: the `POST /api/collections/groups/records` hook and the one-time `X-United-Credential` response header, on top of the slice 2 `Store.CreateGroup`.

- [ ] **Step 1: Write the failing tests**

Test names: `TestCreateGroupTransaction`, `TestCreateGroupReturnsSecretOnce`, `TestCreateGroupSecretNotPersisted`, `TestCreateGroupAudit`, `TestCreateGroupWrappedKey`.

Assertions:
- One transaction creates the group, owner membership, wrapped data key, credential, and `group_created` audit event.
- The response carries the credential password exactly once; a later list or view never contains it.
- The credential username is random, non-secret, unique, and independent of the group route ID.
- The password is 32 random bytes as unpadded base64url.
- A failure in any step rolls back the whole creation.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/store -run TestCreateGroup -v`
Expected: FAIL because group creation does not exist.

- [ ] **Step 3: Implement creation**

Reuse the slice 2 `Store.CreateGroup`, which already commits the group, owner membership, wrapped data key, credential, and `group_created` audit event in one transaction. In the `OnRecordCreateRequest` hook for `groups`, authorize the caller, call `Store.CreateGroup`, set `X-United-Credential` on the response to the one-time secret, and return the created group as JSON without calling `e.Next()`. The deployment's CORS configuration must expose that header. The header is the only place the secret appears; it is never persisted, never in the schema, and never in list or view responses.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/store ./internal/hooks -run TestCreateGroup -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/groups.go internal/hooks/groups.go
git commit -m "feat: create groups with wrapped keys and one-time credentials"
```

### Task 3.3: Membership and invitation lifecycle

**Files:**
- Create: `internal/store/memberships.go`, `internal/hooks/memberships.go`
- Test: `internal/store/memberships_test.go`, `internal/hooks/memberships_test.go`

**Interfaces:**
- Produces: invitation create/accept/revoke, membership role change/remove/leave, and the final-owner guard.

- [ ] **Step 1: Write the failing tests**

Test names: `TestInviteExistingUser`, `TestInviteReplacesPending`, `TestInvitationExpires`, `TestAcceptCreatesMember`, `TestAcceptSingleUse`, `TestFinalOwnerGuard`, `TestLeaveGroup`, `TestPromoteDemote`, `TestMembershipAudit`.

Assertions:
- Only owners invite; the target must be an existing user.
- Reissuing replaces the pending invitation.
- Acceptance creates a `member` membership atomically and deletes the invitation.
- The final active owner cannot leave, be removed, be demoted, be deactivated, or be deleted.
- Every transition writes the correct audit event in the same transaction.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/store ./internal/hooks -run 'TestInvite|TestInvitation|TestAccept|TestFinalOwner|TestLeave|TestPromote|TestMembership' -v`
Expected: FAIL because the lifecycle does not exist.

- [ ] **Step 3: Implement the lifecycle**

Use request hooks for the API paths and store methods for the transactions. Validate the actor, group status, role, expiry, and final-owner invariant. Use the `system_settings.invitation_ttl_days` value for expiry.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/store ./internal/hooks -run 'TestInvite|TestInvitation|TestAccept|TestFinalOwner|TestLeave|TestPromote|TestMembership' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/memberships.go internal/hooks/memberships.go
git commit -m "feat: add invitation and membership lifecycle with final-owner guard"
```

### Task 3.4: Credential rotation and disabling

**Files:**
- Create: `internal/store/credential_lifecycle.go`, `internal/hooks/credentials.go`
- Test: `internal/store/credential_lifecycle_test.go`

**Interfaces:**
- Produces: rotate, disable, and enable operations with a credential-scoped non-blocking guard.

- [ ] **Step 1: Write the failing tests**

Test names: `TestRotateInvalidatesOld`, `TestRotateReturnsSecretOnce`, `TestRotateDisabledEnables`, `TestDisablePreservesPassword`, `TestEnableRestoresPassword`, `TestCredentialGuardConflict`, `TestRotationAudit`.

Assertions:
- Rotation replaces the verifier atomically; the old password fails immediately and the new one succeeds.
- The new secret appears exactly once.
- Rotating a disabled credential enables it.
- Disable and re-enable preserve the password.
- A concurrent mutation gets 409, not a wait.
- Each transition writes an audit event with no secret.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/store -run 'TestRotate|TestRotation|TestDisable|TestEnable|TestCredentialGuard' -v`
Expected: FAIL because the lifecycle does not exist.

- [ ] **Step 3: Implement the lifecycle**

Use the write-only `rotate_credential` intent field. Validate the actor and target, consume and clear the flag in the same transaction, mint the new secret, set the password, update timestamps, and audit in one transaction. Return the new secret only in the `X-United-Credential` response header, exactly once. Implement the guard with a per-credential mutex or a database compare-and-swap; a contention returns 409.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/store -run 'TestRotate|TestRotation|TestDisable|TestEnable|TestCredentialGuard' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/credential_lifecycle.go internal/hooks/credentials.go
git commit -m "feat: add credential rotation, disabling, and enabling"
```

### Task 3.5: Group status, retention, and lease overrides

**Files:**
- Create: `internal/store/groups_lifecycle.go`, `internal/hooks/groups_lifecycle.go`
- Test: `internal/store/groups_lifecycle_test.go`

**Interfaces:**
- Produces: suspend, reactivate, request retirement, cancel retirement, retention changes, and lease overrides.

- [ ] **Step 1: Write the failing tests**

Test names: `TestSuspendReactivates`, `TestRetirementFreezesDataPlane`, `TestRetirementCancel`, `TestRetirementEligible`, `TestRetentionBounds`, `TestRetentionAudit`, `TestLeaseOverrideBounds`, `TestLeaseOverrideAudit`.

Assertions:
- Only a superuser suspends or reactivates; only an owner requests retirement for an active group; a superuser requests it for a suspended group.
- Pending retirement freezes Terraform operations but leaves administration available.
- Cancellation restores the previous status and is owner-allowed only while ineligible.
- Retention is 1–365 days, default 30; every change audits `{from,to}`.
- Lease overrides are 5m–24h and audit; existing leases keep their expiry.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/store -run 'TestSuspend|TestRetirement|TestRetention|TestLeaseOverride' -v`
Expected: FAIL because the lifecycle does not exist.

- [ ] **Step 3: Implement the lifecycle**

Validate the actor and transition, set timestamps and status, and audit atomically. Consume and clear `confirm_destructive` in the same transaction (write-only field, ADR 0002). Use the domain retention and lease validators.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/store -run 'TestSuspend|TestRetirement|TestRetention|TestLeaseOverride' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/groups_lifecycle.go internal/hooks/groups_lifecycle.go
git commit -m "feat: add group suspension, retirement, retention, and lease controls"
```

### Task 3.6: Tombstone and restore administration

**Files:**
- Modify: `internal/store/states.go`
- Create: `internal/hooks/states_admin.go`
- Test: `internal/store/states_admin_test.go`

**Interfaces:**
- Produces: owner tombstone and restore through `PATCH logical_states`.

- [ ] **Step 1: Write the failing tests**

Test names: `TestOwnerTombstoneBlockedByLiveLease`, `TestRestoreVersionedTombstone`, `TestRestoreEmptyTombstone`, `TestRestoreNeverPromotesOlderVersion`, `TestRestoreFailsOnTamper`, `TestRestoreAudit`.

Assertions:
- A live lease blocks owner tombstoning with 409.
- Restoring a versioned tombstone decrypts and verifies the retained former current version, sets it current, clears `non_current_at`, and audits.
- Restoring an empty tombstone returns it to `empty`.
- An older version is never selected.
- A tampered file fails with no metadata change.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/store -run 'TestOwnerTombstone|TestRestore' -v`
Expected: FAIL because administration does not exist.

- [ ] **Step 3: Implement tombstone and restore**

Use the request hook to validate the actor and intent. Restore in a transaction, decrypt and verify before committing, and audit.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/store -run 'TestOwnerTombstone|TestRestore' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/states.go internal/hooks/states_admin.go
git commit -m "feat: add owner tombstone and state restoration"
```

### Task 3.7: Audit catalog and PocketBase log mirror

**Files:**
- Modify: `internal/store/audit.go`
- Create: `internal/hooks/logs.go`
- Test: `internal/store/audit_test.go`, `internal/hooks/logs_test.go`

**Interfaces:**
- Produces: every audit event type from #420 with a validated `details` payload, and the safe PocketBase log mirror.

- [ ] **Step 1: Write the failing tests**

Test names: `TestAuditCatalogComplete`, `TestAuditDetailsValidated`, `TestAuditAppendOnly`, `TestAuditSuperuserOnly`, `TestLogMirrorSafeSubset`, `TestLogMirrorStripsQuery`, `TestLogMirrorDropsDetails`.

Assertions:
- Every event type has a `schema_version=1` details validator and an actor/subject.
- `user_deactivated`, `user_reactivated`, `user_deleted`, `user_password_changed`, and `owner_recovered` are part of the catalog.
- Unknown details fields or missing required fields are rejected.
- Append-only holds for every non-superuser principal: no update or delete path exists for anonymous, member, owner, or service code. A superuser can alter or delete audit rows as a trusted operator, and the trust boundary is documented rather than enforced (ADR 0002).
- The mirror forwards only `pb.type`, `pb.method`, path without query, `pb.status`, `pb.exec_time_ms`, `pb.auth`, `pb.user_ip`, `pb.remote_ip`, and `pb.error`.
- `details`, `meta` except `request_id`, `referer`, `userAgent`, and all query strings are dropped.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/store ./internal/hooks -run 'TestAudit|TestLogMirror' -v`
Expected: FAIL because the catalog and mirror do not exist.

- [ ] **Step 3: Implement the catalog and mirror**

Define one constructor per event type with typed details. Bind `OnModelCreate(core.LogsTableName)` and forward the safe subset to the operational logger. Leave `_logs` retention and storage untouched.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/store ./internal/hooks -run 'TestAudit|TestLogMirror' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/audit.go internal/hooks/logs.go
git commit -m "feat: complete the audit catalog and PocketBase log mirror"
```

### Task 3.8: Operator trust boundary and hardening documentation

**Files:**
- Create: `site/content/explanation/trust-boundary.md`, `site/content/how-to/harden-deployment.md`
- Modify: `site/content/reference/configuration.md`
- Test: `site/` build and link checks

**Interfaces:**
- Produces: the operator-facing statement of what the superuser role can do and how a deployment is hardened around it.

- [ ] **Step 1: Write the failing docs check**

Run `hugo --panicOnWarning --printPathWarnings` in `site/` and expect it to fail because the pages do not exist.

Assertions after the pages exist:
- `trust-boundary.md` states that superusers bypass every collection rule and record hook, that audit rows are not tamper-evident against them, and that validators are for correctness, not containment.
- `harden-deployment.md` states that superuser access is for setup and backup only, that the dashboard and superuser API are reachable only over TLS behind an isolated edge, that recovery from superuser misuse is restore of the paired backup set, and that registration verification requires SMTP configured through PocketBase `_params`.
- `configuration.md` names `UNITED_MASTER_KEY`, `UNITED_MASTER_KEY_GENERATION`, `UNITED_MAX_STATE_SIZE`, `UNITED_DATA_DIR`, `UNITED_LISTEN_ADDR`, `UNITED_LOCK_LEASE_DURATION`, `TELEMETRY_ENABLED`, `UNITED_ACTOR`, and `LOG_LEVEL` with their bounds and defaults.

- [ ] **Step 2: Run the docs check to verify it fails**

Run: `cd site && hugo --panicOnWarning --printPathWarnings`
Expected: FAIL because the pages do not exist.

- [ ] **Step 3: Write the pages**

Write the three pages in the Diataxis modes they belong to, link them from the explanation and how-to indexes, and keep every claim consistent with ADR 0002.

- [ ] **Step 4: Run the docs check to verify it passes**

Run: `mise run docs:check`
Expected: PASS with zero warnings.

- [ ] **Step 5: Commit**

```bash
git add site/content
git commit -m "docs: document the superuser trust boundary and deployment hardening"
```

### Task 3.9: User account lifecycle

**Files:**
- Create: `internal/hooks/users.go`, `internal/store/users.go`, `site/content/how-to/manage-accounts.md`
- Modify: `internal/migrations/202611210001_schema.go`
- Test: `internal/hooks/users_test.go`, `internal/store/users_test.go`

**Interfaces:**
- Produces: `Store.DeactivateUser(ctx context.Context, actor Actor, userID string) error`, `Store.ActivateUser(ctx context.Context, actor Actor, userID string) error`, `Store.DeleteUser(ctx context.Context, actor Actor, userID string) error`, the `users` request hooks, and the `user_deactivated`, `user_reactivated`, `user_deleted`, `user_password_changed`, and `owner_recovered` audit events (ADR 0003).

- [ ] **Step 1: Write the failing tests**

Test names: `TestDeactivateBarsAuthentication`, `TestDeactivateRevokesSessions`, `TestDeactivatePreservesMemberships`, `TestFinalOwnerCannotDeactivate`, `TestFinalOwnerCannotDelete`, `TestFinalOwnerAllowedWhileGroupSuspended`, `TestReactivationSuperuserOnly`, `TestDeleteRemovesMembershipsAndInvitations`, `TestSelfUpdateFieldAllowlist`, `TestExceptionalOwnerRecovery`, `TestUserLifecycleAudit`.

Assertions:
- Deactivation sets `disabled=true`, rotates `tokenKey`, and leaves memberships and invitations intact; a password login fails with `401`, and a token minted before deactivation fails on its next use.
- Deactivation or deletion returns `409` when the user is the only active owner of any active or pending-retirement group, and succeeds with another active owner or while that group is suspended.
- Reactivation is superuser-only, clears `disabled`, and leaves memberships intact; the user signs in again.
- Deletion removes the user, every membership, and every pending invitation, and is rejected by the final-owner guard.
- A self-update may change name, email, password, or set `disabled=true`; it may not set `disabled=false`, change another user, or change role-bearing relations.
- Exceptional recovery promotes an existing member or adds an existing user as owner only while the group is suspended, and writes `owner_recovered` with the operator as actor.
- Every transition writes its audit event in the same transaction and never stores a password, hash, or token.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/hooks ./internal/store -run 'TestDeactivate|TestFinalOwner|TestReactivation|TestDeleteRemoves|TestSelfUpdate|TestExceptional|TestUserLifecycle' -v`
Expected: FAIL because the users hook and store methods do not exist.

- [ ] **Step 3: Implement the lifecycle**

Bind `OnRecordUpdateRequest` and `OnRecordDeleteRequest` for `users`. Validate the actor (self or superuser for deactivation and deletion, superuser for reactivation), run the final-owner query inside the same transaction, call `RefreshTokenKey()` on deactivation, and write the audit event. Bind `OnRecordCreateRequest` for guest registration so it never grants a group. Emit `user_password_changed` when the request body carries a password change. Add the exceptional-recovery path through `group_memberships` creation or promotion, gated on `groups.status=suspended`.

Write `manage-accounts.md` covering registration and SMTP, deactivation and sign-out, reactivation, deletion, final-owner rules, and exceptional recovery.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/hooks ./internal/store -run 'TestDeactivate|TestFinalOwner|TestReactivation|TestDeleteRemoves|TestSelfUpdate|TestExceptional|TestUserLifecycle' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/hooks/users.go internal/hooks/users_test.go internal/store/users.go internal/store/users_test.go internal/migrations site/content/how-to/manage-accounts.md
git commit -m "feat: enforce the user account lifecycle"
```

---

## Slice 4: Scheduled Operations

Slice gate: fake-clock eligibility, batch and budget limits, single-flight, shutdown, brake, orphan age guard, local and LocalStack, and failure injection all pass.

### Task 4.1: Single-flight guard and cron registration

**Files:**
- Create: `internal/jobs/guard.go`, `internal/jobs/jobs.go`
- Test: `internal/jobs/guard_test.go`

**Interfaces:**
- Produces: `Guard`, `Guard.Try() (release func(), ok bool)`, and `jobs.Register(app core.App, deps Deps)`.

```go
type Deps struct {
	App    core.App
	Logger *slog.Logger
	Clock  state.Clock
	Config *config.Config
}
```

- [ ] **Step 1: Write the failing tests**

Test names: `TestGuardSingleFlight`, `TestGuardShutdown`, `TestCronRegistration`, `TestOverlapSkips`.

Assertions:
- A second concurrent `Try` returns false and emits `united.cleanup.skipped{outcome="overlap"}`.
- Shutdown stops new items and lets the in-flight item finish.
- `retention-cleanup` is registered as `*/15 * * * *`, `orphan-reconciliation` as `17 3 * * *`, and `integrity-scan` as `37 4 * * *`.
- No catch-up run starts at boot for cleanup or reconciliation; the integrity scan has its own run shortly after serve.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/jobs -run 'TestGuard|TestCron|TestOverlap' -v`
Expected: FAIL because the guard does not exist.

- [ ] **Step 3: Implement the guard and registration**

Use `sync.Mutex`/`atomic.Bool` and a `context.Context` cancelled on `OnTerminate`. Register jobs in `OnBootstrap` or `OnServe` before priority 999.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/jobs -run 'TestGuard|TestCron|TestOverlap' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/jobs
git commit -m "feat(jobs): add single-flight guard and cron registration"
```

### Task 4.2: Retention cleanup and group purge

**Files:**
- Create: `internal/jobs/cleanup.go`, `internal/jobs/purge.go`
- Test: `internal/jobs/cleanup_test.go`

**Interfaces:**
- Produces: the two-phase `retention-cleanup` run.

- [ ] **Step 1: Write the failing tests**

Test names: `TestPurgeBeforeRetention`, `TestBatchLimits`, `TestBudgetStops`, `TestOldestFirst`, `TestRemainingCount`, `TestRecheckPreventsRemoval`, `TestTombstonedPointerClear`, `TestPurgedTransition`, `TestGroupPurgeCascade`, `TestGroupPurgeAudit`.

Assertions:
- Group purge runs before version retention.
- At most 10 groups and 1,000 versions per run; the 60-second budget stops the phase.
- Oldest `non_current_at` first; the summary reports remaining.
- A restoration or policy extension committed before the recheck prevents removal.
- Removing a tombstoned state's current version clears the pointer, writes the permanent audit event, and sets `purged` when the last version goes.
- Group purge cascades and writes the retired marker and audit atomically.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/jobs -run 'TestPurge|TestBatch|TestBudget|TestOldest|TestRemaining|TestRecheck|TestTombstoned|TestPurged|TestGroupPurge' -v`
Expected: FAIL because cleanup does not exist.

- [ ] **Step 3: Implement the two phases**

Use `RunInTransaction` per item, recheck eligibility, and emit one run summary plus per-item failures. Use a fake clock in tests.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/jobs -run 'TestPurge|TestBatch|TestBudget|TestOldest|TestRemaining|TestRecheck|TestTombstoned|TestPurged|TestGroupPurge' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/jobs/cleanup.go internal/jobs/purge.go
git commit -m "feat(jobs): add retention cleanup and group purge"
```

### Task 4.3: Orphan reconciliation and brake

**Files:**
- Create: `internal/jobs/orphans.go`
- Test: `internal/jobs/orphans_test.go`

**Interfaces:**
- Produces: `orphan-reconciliation`, the 24-hour guard, the committed-key set, the brake, and the forced run.

- [ ] **Step 1: Write the failing tests**

Test names: `TestOrphanAgeGuard`, `TestOrphanPrefixOnly`, `TestOrphanCommittedSet`, `TestOrphanIdempotentDelete`, `TestBrakeThreshold`, `TestBrakeStopsAll`, `TestForcedCleanup`, `TestOrphanLocalAndS3`.

Assertions:
- Only objects older than 24 hours under the `state_versions` prefix are candidates.
- The committed set is `collectionId/recordId/storedFilename`.
- Deleting an absent key succeeds.
- Candidates above `max(100, 5% of listed)` abort with an error event and no deletes.
- Only `cleanup --orphans --force` clears the brake; the next cron tick does not.
- Local disk and LocalStack produce the same outcome.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/jobs -run 'TestOrphan|TestBrake|TestForced' -v`
Expected: FAIL because reconciliation does not exist.

- [ ] **Step 3: Implement reconciliation**

Use `app.NewFilesystem()`, `List(collection.Id + "/")`, and `Delete(key)`. Build the committed set from all `state_versions`. Emit the run summary and brake event.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/jobs -run 'TestOrphan|TestBrake|TestForced' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/jobs/orphans.go
git commit -m "feat(jobs): add orphan reconciliation with a mass-deletion brake"
```

### Task 4.4: Integrity scan

**Files:**
- Create: `internal/jobs/integrity.go`
- Modify: `internal/jobs/jobs.go`
- Test: `internal/jobs/integrity_test.go`

**Interfaces:**
- Produces: the daily current-version integrity scan and its metrics/events.

- [ ] **Step 1: Write the failing tests**

Test names: `TestIntegrityChecksCurrentOnly`, `TestIntegrityReportsMissing`, `TestIntegrityReportsDecrypt`, `TestIntegrityReportsLength`, `TestIntegrityReportsDigest`, `TestIntegrityNeverMutates`, `TestIntegrityScheduled`, `TestIntegrityRunsAfterServe`.

Assertions:
- Every active current version is checked; non-current versions are not.
- Missing, undecryptable, wrong-length, and wrong-digest cases emit `united.integrity_scan.failure` with the right class.
- No metadata or file is changed on failure.
- The run emits one summary and updates `united.integrity_scan.last_success_age`.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/jobs -run TestIntegrity -v`
Expected: FAIL because the scan does not exist.

- [ ] **Step 3: Implement the scan**

Register `integrity-scan` in `jobs.go`. Iterate active logical states, decrypt and verify each current version, emit events and metrics, and never write. Run once shortly after `OnServe` and then on the cron schedule; a superuser can trigger it through `POST /api/crons/integrity-scan`, and the single-flight guard prevents overlap.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/jobs -run TestIntegrity -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/jobs/integrity.go
git commit -m "feat(jobs): add the daily state integrity scan"
```

---

## Slice 5: Offline and Recovery

Slice gate: rekey, repair, verify, forced cleanup, `--actor`, stdout-only, the startup gate, backup pairing, and the restore drill pass.

### Task 5.1: Offline CLI skeleton

**Files:**
- Create: `internal/offline/root.go`
- Modify: `cmd/united/main.go`
- Test: `internal/offline/root_test.go`

**Interfaces:**
- Produces: `offline.Register(root *cobra.Command, deps Deps)` with `rekey`, `verify`, and `cleanup` subcommands.

```go
type Deps struct {
	Config *config.Config
	Logger *slog.Logger
	App    core.App
}
```

- [ ] **Step 1: Write the failing tests**

Test names: `TestOfflineDoesNotInitOTel`, `TestOfflineWritesStdoutOnly`, `TestOfflineRequiresActor`, `TestOfflineBootstrapsMigrations`.

Assertions:
- `united rekey --old-key ... --new-key ... --actor ops` runs without constructing OTel providers.
- Every offline command requires `--actor` or `UNITED_ACTOR` when it can write audit.
- Output is line-oriented stdout with no structured logger.
- The app bootstraps and runs migrations before the command body.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/offline -v`
Expected: FAIL because the CLI does not exist.

- [ ] **Step 3: Implement the skeleton**

Add subcommands to `app.RootCmd` before `Execute`. `app.Execute()` bootstraps the app, so do not call `app.Bootstrap()` again; a second call re-fires every `OnBootstrap` hook. In each `RunE`, call `app.RunAllMigrations()`, then `wire.keyGate(app, cfg)` with `config.ModeCommand`, then the command body. Never call `apis.Serve` and never initialize telemetry.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/offline -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/offline/root.go cmd/united/main.go
git commit -m "feat(offline): add the offline command skeleton"
```

### Task 5.2: Rekey and group-key repair

**Files:**
- Create: `internal/offline/rekey.go`
- Test: `internal/offline/rekey_test.go`

**Interfaces:**
- Produces: global rekey and scoped repair with `--old-key`, `--new-key`, `--group`, and `--actor`.

- [ ] **Step 1: Write the failing tests**

Test names: `TestRekeyAllOrNothing`, `TestRekeyAdvancesGeneration`, `TestRekeyReencryptsSentinel`, `TestRekeyRejectsBadKeys`, `TestRekeyAudit`, `TestRepairOneGroup`, `TestRepairDoesNotAdvanceGeneration`, `TestRepairRefusesRetired`.

Assertions:
- Rekey re-wraps every non-retired group key, re-encrypts `system_settings.key_sentinel` under the new master key, advances `master_key_generation`, and audits the old and new generation in one transaction (ADR 0002).
- Any failure rolls back all changes.
- Malformed, wrong-length, or identical keys are rejected.
- Repair targets one group, does not advance the generation, and audits.
- Retired groups are ineligible.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/offline -run 'TestRekey|TestRepair' -v`
Expected: FAIL because rekey does not exist.

- [ ] **Step 3: Implement rekey and repair**

Use `RunInTransaction`, `crypto.UnwrapGroupKey`/`WrapGroupKey`, and the audit catalog. Re-encrypt the sentinel with `crypto.NewSentinel` under the new key before committing. Record `actor_type=offline_operator`, `actor_id`, `details.host`, and `details.master_key_generation`; never key material.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/offline -run 'TestRekey|TestRepair' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/offline/rekey.go
git commit -m "feat(offline): add global rekey and scoped group-key repair"
```

### Task 5.3: Full-history verify and forced cleanup

**Files:**
- Create: `internal/offline/verify.go`, `internal/offline/cleanup.go`
- Test: `internal/offline/verify_test.go`, `internal/offline/cleanup_test.go`

**Interfaces:**
- Produces: `verify` over every retained version and `cleanup --orphans --force`.

- [ ] **Step 1: Write the failing tests**

Test names: `TestVerifyAllVersions`, `TestVerifyReportsOnly`, `TestForcedCleanupIgnoresBrake`, `TestForcedCleanupNoAudit`, `TestVerifyNoOTel`.

Assertions:
- `verify` decrypts and checks length and SHA-256 for every version, including non-current and tombstoned history.
- It never mutates metadata or files.
- Forced cleanup deletes per key with the brake disabled and writes no security audit event.
- Both commands use stdout only.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/offline -run 'TestVerify|TestForced' -v`
Expected: FAIL because the commands do not exist.

- [ ] **Step 3: Implement both commands**

Share the integrity check with the background scan. Force cleanup reuses the orphan scanner with the brake disabled.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/offline -run 'TestVerify|TestForced' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/offline/verify.go internal/offline/cleanup.go
git commit -m "feat(offline): add full-history verify and forced orphan cleanup"
```

### Task 5.4: Startup gate, backup pairing, and restore drill

**Files:**
- Modify: `internal/wire/wire.go`
- Create: `tests/restore/drill.sh`, `docs/releases/v0.2.0.md` (draft)
- Test: `internal/wire/keygate_test.go`, `tests/restore/drill.sh`

**Interfaces:**
- Produces: `wire.keyGate(app core.App, cfg *config.Config) error`, the first-boot sentinel rule, the mismatch refusal, the backup pairing checklist, and the restore drill.

- [ ] **Step 1: Write the failing tests**

Test names: `TestFirstBootWritesSentinel`, `TestFirstBootWritesConfiguredGeneration`, `TestMissingSentinelWithGroupsRefuses`, `TestSecondBootVerifies`, `TestGenerationMismatchRefuses`, `TestWrongKeyRefuses`, `TestCorruptSentinelRefuses`, `TestBootAfterRekey`.

Assertions:
- A fresh database with no sentinel writes the configured generation and sentinel and starts, for any configured generation of 1 or more.
- A database that has wrapped group keys but no sentinel refuses to start.
- A corrupt sentinel refuses to start.
- A second boot with the same key and generation starts.
- A different generation or key refuses to start before listening.
- A boot after a global rekey with the new key and generation starts, and with the old key refuses.
- The restore drill restores the newest database backup with its paired file snapshot and key, runs `verify`, and runs the Terraform harness against the restored instance.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/wire -run TestFirstBoot -v`
Expected: FAIL because the corrupt-sentinel, missing-sentinel-with-groups, configured-generation, and post-rekey cases are not implemented (the base gate lands in Task 1.9).

- [ ] **Step 3: Implement the gate and drill**

Implement the gate as `wire.keyGate(app, cfg) error`, called from `OnServe` before `e.Next()` (application migrations have run by then; `OnBootstrap` runs before they do) and from each offline command after `RunAllMigrations()`. On boot: load `system_settings`; if `key_sentinel` is empty and no group has a wrapped key, write the configured generation and `crypto.NewSentinel`, then start; if the sentinel is empty but wrapped keys exist, refuse; otherwise compare `master_key_generation` and `CheckSentinel` and refuse on either mismatch. The drill script documents the file snapshot before the database backup and the full-history verify.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/wire -run 'TestFirstBoot|TestMissingSentinel|TestSecondBoot|TestGeneration|TestWrongKey|TestCorruptSentinel|TestBootAfterRekey' -v`
Expected: PASS. Run `tests/restore/drill.sh` separately and record the result.

- [ ] **Step 5: Commit**

```bash
git add internal/wire tests/restore docs/releases
git commit -m "feat: add the startup key gate and restore drill"
```

---

## Release Milestone: `v0.2.0`

This is not a slice. After slice 5 passes, run the release-only gates and publish.

- [ ] Run the full four-client matrix: Terraform 1.16 and 1.17, OpenTofu 1.12 and 1.13, covering the full harness, migration, lock payloads, methods, size limits, and force-unlock.
- [ ] Run the seven release-blocking security checks from #417.
- [ ] Run the manual failure-injection suite and record every named case.
- [ ] Run the restore drill against the newest backup and record the result.
- [ ] Confirm the Hugo build, markdownlint, internal links, and external links.
- [ ] Confirm no plaintext state, credential secret, or master key appears in the database, files, logs, or error bodies.
- [ ] Confirm `/api` exposes only PocketBase built-ins and no custom route.
- [ ] Write `docs/releases/v0.2.0.md` with the acceptance record.
- [ ] Tag `v0.2.0`, publish the image and GitHub Release, and deploy the docs site.
- [ ] Close the map with the plan and release record linked.

## Traceability Matrix

| Decision | Slice | Named tests | Gate |
| --- | --- | --- | --- |
| #406 wire contract | 2 | `TestGet*`, `TestPublish*`, `TestPostAndPut*`, `TestLock*`, `TestUnlock*`, `TestMethodMatrix*` | Slice 2 gate |
| #407 transactions and files | 2, 4 | `TestPublishRollsBack*`, `TestOrphan*`, `TestIntegrity*` | Slice 2, 4 gates |
| #408 auth extension points | 2 | `TestVerifyCredential*` | Slice 2 gate |
| #409 roles and membership | 3 | `TestInvite*`, `TestAccept*`, `TestFinalOwner*`, `TestDeactivate*`, `TestExceptional*` | Slice 3 gate |
| #410 credential lifecycle | 2, 3 | `TestVerifyCredential*`, `TestRotate*`, `TestDisable*` | Slice 2, 3 gates |
| #411 group retirement | 3, 4 | `TestRetirement*`, `TestGroupPurge*` | Slice 3, 4 gates |
| #412 deletion and retention | 2, 3, 4 | `TestDelete*`, `TestRestore*`, `TestRetention*` | Slice 2, 3, 4 gates |
| #413 key lifecycle | 1, 5 | `TestParseMasterKey`, `TestSentinel*`, `TestRekey*`, `TestBootAfterRekey` | Slice 1, 5 gates |
| #414 lock leases | 2 | `TestLock*`, `TestUnlock*`, `TestPublish*`, `TestDelete*` | Slice 2 gate |
| #415 persistence model | 1, 2 | `TestMigrationSchema`, `TestPublish*`, `TestVersionFieldsImmutable` | Slice 1, 2 gates |
| #416 administration boundary | 1, 3 | `TestRules*`, `TestCreateGroup*`, `TestSelfUpdate*` | Slice 1, 3 gates |
| #417 rollout and acceptance | 2, 5, release | migration harness, `tests/restore/drill.sh`, release checklist | Slice 2, 5 gates, release |
| #418 Terraform/OpenTofu | 2 | `TestMethodMatrix*`, `terraform-matrix.yml` | Slice 2 gate |
| #419 credential storage | 2, 3 | `TestVerifyCredential*`, `TestRotate*` | Slice 2, 3 gates |
| #420 audit and abuse | 2, 3 | `TestAudit*`, `TestUserLifecycle*`, `TestVerifyCredentialUnknownUser*` | Slice 2, 3 gates |
| #421 cleanup operations | 4 | `TestPurge*`, `TestBatch*`, `TestBrake*`, `TestOrphan*` | Slice 4 gate |
| #560 documentation | 1, 3, release | Hugo build, `docs:check`, link checks | Slice 1, 3 gates, release |
| #561 telemetry | 1, 2 | `TestRegistry*`, `TestSetup*`, `TestRedaction*` | Slice 1, 2 gates |
| #562 boundaries and slicing | all | build, depguard, slice gates | Every PR |

## Deferrals

No soak gate, no dual-run, no feature flag, no server-side import, no S3 import, no penetration test, no offline docs bundle, no v0.1 docs, no in-app metrics listener, no status route, no in-app rate limiting, no administration route beyond PocketBase collections, no custom storage interface, and no group or state attributes in metrics. The failure-injection suite stays manual until wired into the release. The restore drill runs separately. Documentation versioning and Pages deployment happen only at release.
