# Agent Guide

## Project

United is a multitenant Terraform HTTP backend designed to run alongside Atlantis. It stores Terraform state in encrypted S3 objects, coordinates state locks with Redis, and can validate HTTP Basic Auth credentials through an external service.

Treat state paths, authentication, encryption, and locking as security- and data-sensitive behavior. Preserve Terraform's HTTP backend contract when changing the state endpoints.

## Repository map

- `main.go` — loads environment configuration, builds the Gin server, registers middleware and routes.
- `config.go` — environment-backed runtime configuration and Terraform lock payload types.
- `middlewares.go` — S3/Redis setup, Basic Auth validation, and S3 key/path construction.
- `handlers.go` — Terraform state `GET`, `POST`, `DELETE`, `LOCK`, and `UNLOCK` handlers.
- `util.go` — shared AWS and Redis client helpers.
- `tests/` — Terraform-based end-to-end test harness.
- `docker-compose.yml` — local Redis, LocalStack, and nginx dependencies.
- `Makefile` — development, build, dependency, and test entry points.
- `.github/workflows/` — pull-request automation for linting, pre-commit checks, and Terraform tests.
- `dist/` and `tmp/` — generated/local workspace content; do not treat them as source.

## Development workflow

Run commands from the repository root.

```bash
# Install the project tooling and local prerequisites
mise install && mise use

# Build the binary
misew run build

# Start Docker dependencies, configure LocalStack, and run with Air
make run

# Run the Terraform integration test harness
make test
```

`make run` expects Docker, AWS CLI, `jq`, and a configured `localstack` AWS profile. The local defaults use Redis at `localhost:6379`, LocalStack at `localhost:4566`, bucket `united-test`, and KMS alias `alias/united-test`. The test harness uses the Terraform version in mise.toml and its defaults are defined in `tests/Makefile`.

To stop the local dependencies:

```bash
make down
```

For a target list, run `make help`.

## Validation

Use the narrowest relevant check first, then the broader check when practical:

- Go source changes: run `gofmt` on changed Go files and run `go test ./...` when Go tests or package behavior are affected.
- Lint-sensitive changes: run the configured pre-commit hooks; CI also runs `golangci-lint`.
- HTTP backend, S3, Redis, locking, or authentication changes: start the local services with `make run`, then run `make test`.
- Documentation-only changes: check Markdown formatting and links, and avoid running infrastructure-dependent tests unnecessarily.

The Terraform test creates and updates state, pulls state, destroys the test configuration, and checks the resulting S3 contents. It needs Docker, Terraform, AWS CLI, LocalStack, Redis, and the local AWS profile; do not run it against production credentials or resources.

## Configuration and security

Configuration is environment-based. Required runtime values are `BUCKET` and `KEY_ARN`; other settings and defaults are defined in `config.go` and documented in `README.md`.

When changing configuration or request handling:

- Keep secrets and credentials out of source, logs, issues, and committed test fixtures.
- Preserve the `BUCKET_PREFIX` and user/group/name key semantics unless the change explicitly includes a state migration plan.
- Treat `VALIDATE_AUTH=false` as a development/testing fallback, not a production authentication mechanism; its derived key path is not a substitute for external credential validation.
- Keep S3, KMS, Redis, authentication, and fronting-proxy traffic appropriately isolated and encrypted in deployed environments.
- Update `README.md`, the Terraform test harness, or both when externally visible behavior changes.

## Working agreements

- Read `CONTEXT.md` and relevant ADRs before making domain or architectural changes; these files may be created lazily and are allowed to be absent.
- Prefer small, focused changes that follow the existing Go and Makefile patterns.
- Preserve the MPL-2.0 license headers in source files.
- Do not commit generated binaries, local Terraform state, credentials, or dependency caches.
- Use GitHub Issues as the source of work items and interact with them through `gh`; see [`docs/agents/issue-tracker.md`](docs/agents/issue-tracker.md).
- When validation depends on Docker, AWS, Terraform, or external services, report what was and was not run rather than treating a partial check as complete.

## Agent skills

### Issue tracker

Issues and specs live in GitHub Issues; use the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Domain docs

This is a single-context repo with root `CONTEXT.md` and `docs/adr/`. See `docs/agents/domain.md`.
