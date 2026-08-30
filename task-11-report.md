# Task 11 report: deployment documentation and CI

## Documentation assertion checklist

- [x] README requires `UNITED_STATE_MASTER_KEY` to be base64-encoded and decode to 32 bytes.
- [x] README requires durable, access-controlled `pb_data` storage and protected backups; it identifies the directory as containing PocketBase SQLite data and protected encrypted state files.
- [x] README identifies the shared group credential as Terraform-only and states that it cannot log in to PocketBase.
- [x] README documents immutable group route slugs and credential usernames, and immediate invalidation of the previous password on rotation.
- [x] README preserves the `/state/:group/:name` Terraform HTTP endpoint for state, lock, and unlock operations.
- [x] README documents immutable encrypted history and permanent tombstones.
- [x] README documents server-enforced 35-minute lock leases.
- [x] README states that United performs no automatic migration from an existing backend, including an S3 backend; operators use Terraform's backend migration flow.
- [x] README documents that hard-delete retention cleanup is a future administrative capability, not an automatic current behavior.
- [x] README preserves TLS termination and network-isolation guidance.

## Changes

- Updated `README.md` operator guidance for PocketBase persistence, key handling, Terraform-only group credentials, state lifecycle, locks, migration, and secure operation.
- Updated `.github/workflows/terrafrom-test.yml` to generate an ephemeral master key for the Terraform integration harness. The workflow installs only Go and Terraform.
- Updated `Dockerfile` to package the static `united` binary and run `serve --dir=/pb_data` with a writable `/pb_data` volume.
- Left `.goreleaser.yaml` unchanged: its existing `CGO_ENABLED=0` builds already provide the static binary copied by the Dockerfile; no packaging configuration change was required.
- Left `docs/adr/0001-pocketbase-state-storage-and-locking.md` unchanged because the implementation remains consistent with its accepted decision.
- Left `docs/terraform-force-unlock-investigation.md` unchanged; force-unlock remains documented technical debt.

## Validation

| Check | Result |
| --- | --- |
| `go build -o dist/united .` | Passed using project-local Go caches. |
| Legacy operational-term check across README, Makefile, and workflow | Passed with no matches. |
| `go test ./...` | Passed. |
| `go vet ./...` | Passed. |
| `pre-commit run --all-files` | Passed. |
| `UNITED_STATE_MASTER_KEY=<ephemeral 32-byte base64 test key> make test` | Passed against the local PocketBase/Terraform integration harness; no cloud storage, distributed-lock, or local-cloud dependencies were used. |
| `git diff --check` and final status review | Recorded after this report, immediately before commit. |

No secrets, runtime data, generated binary, or cache files are included in the commit.
