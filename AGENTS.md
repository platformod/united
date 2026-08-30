# Agent Guide

## Project

United is a multitenant PocketBase-based Terraform HTTP backend designed to run alongside Atlantis. It stores encrypted Terraform state versions and state-lock metadata in PocketBase, and authenticates Terraform through one shared Basic Auth group credential per group.

Treat state paths, authentication, encryption, and locking as security- and data-sensitive behavior. Preserve Terraform's HTTP backend contract when changing the state endpoints.

## Repository map

- `main.go` — creates and starts the PocketBase application.
- `config.go` — loads `UNITED_STATE_MASTER_KEY` and defines Terraform lock payload types.
- `auth.go` — validates group Basic Auth credentials for Terraform state routes.
- `crypto.go` — encrypts state documents and wraps per-group state keys.
- `hooks.go` — enforces group/state/state-version invariants.
- `routes.go` and `state.go` — register and implement Terraform state `GET`, `POST`, `DELETE`, `LOCK`, and `UNLOCK` handlers.
- `migrations/` — PocketBase collection schema and API-rule migrations.
- `tests/` — standalone Terraform-based end-to-end harness.
- `pb_data/` — local PocketBase data; never treat it as source or commit its contents.
- `Makefile` — development, build, dependency, and test entry points. Brewfile cleanup remains deferred.
- `.github/workflows/` — pull-request automation for linting, pre-commit checks, and Terraform tests.
- `dist/` and `tmp/` — generated/local workspace content; do not treat them as source.

## Development workflow

Run commands from the repository root.

```bash
# Install the project tooling and local prerequisites
make devprep

# Build the binary
make build

# Run with Air and local PocketBase data.
# UNITED_STATE_MASTER_KEY must already be a persistent, base64-encoded 32-byte key.
make run

# Run the standalone Terraform integration harness
make test
```

`make run` requires Air and `UNITED_STATE_MASTER_KEY`; it uses `./pb_data` for local PocketBase data. Do not generate a replacement key for existing data. `make test` generates isolated runtime credentials and an ephemeral master key; it requires Terraform, curl, jq, and OpenSSL, but no Docker, AWS CLI, LocalStack, Redis, or cloud profile. The harness uses the Terraform version in `.terraform-version` and its defaults are defined in `tests/Makefile`.

Stop `make run` with `Ctrl-C`; there are no Docker dependencies to stop.

For a target list, run `make help`.

## Validation

Use the narrowest relevant check first, then the broader check when practical:

- Go source changes: run `gofmt` on changed Go files and run `go test ./...` when Go tests or package behavior are affected.
- Lint-sensitive changes: run the configured pre-commit hooks; CI also runs `golangci-lint`.
- HTTP backend, PocketBase persistence, encryption, locking, or authentication changes: run `make test` after focused Go coverage.
- Documentation-only changes: check Markdown formatting and links, and avoid running infrastructure-dependent tests unnecessarily.

The Terraform test creates and updates state, pulls state, destroys the test configuration, verifies retained encrypted state versions through PocketBase APIs, and exercises locking. It uses isolated temporary PocketBase data and generated credentials. Do not point it at production data, credentials, or a persistent master key.

## Configuration and security

Configuration is environment-based. The required runtime value is `UNITED_STATE_MASTER_KEY`, a persistent base64-encoded 32-byte key used to wrap per-group state keys; its validation is defined in `config.go` and documented in `README.md`.

When changing configuration or request handling:

- Keep master keys, group credentials, human-user credentials, tokens, and state plaintext out of source, logs, issues, and committed test fixtures.
- Preserve the immutable group route-slug and state `group/name` semantics unless the change explicitly includes a backend migration plan. Group slugs must remain one path-safe segment.
- Treat `pb_data`, including its SQLite database and encrypted files, as sensitive durable data. Back it up with access control, and retain the matching persistent master key in a secret manager.
- Keep Terraform/PocketBase administration traffic behind trusted TLS termination and restricted to appropriate networks.
- Update `README.md`, the Terraform test harness, or both when externally visible behavior changes.

## Working agreements

- Read `CONTEXT.md` and relevant ADRs before making domain or architectural changes; these files may be created lazily and are allowed to be absent.
- Prefer small, focused changes that follow the existing Go and Makefile patterns.
- Preserve the MPL-2.0 license headers in source files.
- Do not commit generated binaries, local Terraform state, credentials, or dependency caches.
- Use GitHub Issues as the source of work items and interact with them through `gh`; see [`docs/agents/issue-tracker.md`](docs/agents/issue-tracker.md).
- When validation depends on Terraform or local tooling, report what was and was not run rather than treating a partial check as complete.

## Agent skills

### Issue tracker

Issues and specs live in GitHub Issues; use the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Domain docs

This is a single-context repo with root `CONTEXT.md` and `docs/adr/`. See `docs/agents/domain.md`.
