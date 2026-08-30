# Final Review Fix Report: PocketBase Group Management

## Delivered

- Replaced stale Gin, AWS, Redis, and LocalStack guidance in `AGENTS.md` with the PocketBase runtime, development, validation, and data-security workflow. The guide now requires the persistent base64-encoded 32-byte `UNITED_STATE_MASTER_KEY`, describes `pb_data` as durable sensitive data, and records that Brewfile cleanup remains deferred.
- Replaced committed Go-test credential literals with a test-only cryptographically generated password helper. The Terraform harness continues to generate isolated credentials at runtime.
- Added migration `1788134400_group_slug_constraint.go`, which restricts immutable group route slugs to one lowercase alphanumeric path segment with optional single hyphens between components. Slash, whitespace, dot segments, percent encoding, and query delimiters are rejected.
- Updated `README.md` with the externally visible slug format.
- Strengthened state-lock record validation: a populated `lockInfo` must decode as a JSON object with a nonempty `ID` equal to `lockID`.
- Added regression coverage for accepted and rejected route slugs and malformed, non-object, missing-ID, empty-ID, and mismatched-ID lock metadata.

## Test-first evidence

The new slug and lock-integrity tests were added before the migration and hook implementation. Their red run failed because unsafe slugs and invalid lock records were accepted. After implementation, the focused tests passed.

## Validation

| Command | Result |
| --- | --- |
| `gofmt -w` on changed Go files | Passed. |
| `go test ./...` | Passed. |
| `go vet ./...` | Passed. |
| Focused slug/lock regression tests | Passed after implementation. |
| `make build` | Passed. |
| `make test` | Passed in an unsandboxed local run; exercised the Terraform lifecycle, encrypted-version inspection, locking, deletion retention, and restart checks. |
| `pre-commit run --all-files` | Passed. |
| `git diff --check` | Passed before this report was added. |
| Direct `golangci-lint run` | Ran but returned the pre-existing `nlreturn` issue at unchanged `state.go:340`. Its configured pre-commit hook passed. |

## Preserved contracts and accepted debt

- Terraform state endpoint paths and Basic Auth semantics are unchanged.
- Group route slugs and state `group/name` identities remain immutable.
- The accepted native `terraform force-unlock` follow-up remains unchanged.
- Brewfile cleanup remains deferred.
