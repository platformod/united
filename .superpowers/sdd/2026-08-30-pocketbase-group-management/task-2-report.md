# Task 2 Report: PocketBase API Test Provisioning

## Delivered

- Removed the `test-provision` and `test-inspect` Cobra commands and their implementation/tests.
- Added a command-surface regression test confirming both commands are unavailable.
- Updated the integration harness to generate superuser, human-user, Terraform group-credential, and state master-key material at runtime.
- Bootstrapped a PocketBase superuser only when the temporary data directory has no `data.db`.
- Provisioned the human user and its group with authenticated PocketBase collection APIs.
- Replaced CLI inspection with bearer-authenticated collection list queries limited to state/version metadata. The harness does not request state-file plaintext.
- Retained temporary-directory cleanup, dynamic-port/PID readiness handling, restart verification, and the accepted Terraform force-unlock debt.

## Test-first evidence

`TestNoTestProvisionCommand` was added before removing the commands. Its initial run failed because both commands were still registered and returned required-flag errors; after removing command registration and the CLI implementation, the focused test passed.

## Validation

Passed:

- `bash -n tests/run.sh tests/provision.sh`
- `go test ./...`
- `go vet ./...`
- `pre-commit run --all-files`
- `make test` (twice, using disposable localhost PocketBase servers and runtime-generated credentials)
- `git --no-pager diff --check`

Known unrelated validation debt:

- A standalone `golangci-lint run` checks the whole repository and currently fails at the pre-existing `state.go:340` `nlreturn` violation. Task 2 does not modify `state.go`; the changed-file `golangci-lint` hook within `pre-commit run --all-files` passed.

## Fix round 1: shell regression coverage

Added `tests/harness_regression_test.sh`, which is required by the `tests` Makefile before the integration lifecycle. It verifies the built binary rejects both removed CLI commands, runtime password generation has no static fallback/default in the harness, and the user/group provisioning and metadata inspection boundaries use PocketBase collection endpoints with bearer authentication. No production code or harness behavior changed. The target was first wired before the test script existed and failed with exit 127, then passed after the script was added.

## Fix round 2: invocation-scoped bearer and master-key coverage

Tightened the shell regression assertions so each protected collection curl invocation (`users`, `groups`, `states`, and `statefiles`) must contain its bearer header within the same curl command block. Password-auth endpoints are intentionally excluded. Added validation that `UNITED_STATE_MASTER_KEY` is generated exactly once with `openssl rand -base64 32`, while other uses only pass that generated value through rather than providing a static or default fallback. The new helper calls first failed because the helpers were absent, then passed after adding the test-only helpers. Production code and harness behavior remain unchanged.

Round 2 validation passed `bash -n`, the shell regression target, `go test ./...`, `go vet ./...`, `pre-commit run --all-files`, `make test`, and `git --no-pager diff --check`. The standalone `golangci-lint run` remains blocked only by the pre-existing `state.go:340` `nlreturn` finding documented above.
