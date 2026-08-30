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
