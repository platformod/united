# Task 3 Report: PocketBase Group Management Documentation

## Delivered

- Documented that human users manage only their own groups through authenticated, owner-scoped PocketBase collection APIs.
- Documented service-assigned immutable group ownership, route slugs, and Terraform credential usernames.
- Defined group API deletion as a permanent tombstone that cannot be updated or revived through normal APIs.
- Documented that group tombstones retain logical states, state versions, and encrypted state files; retention behavior is unchanged.
- Documented the Terraform authentication distinction: valid credentials for a tombstoned group receive `410 Gone`; unknown, missing, or invalid credentials receive the generic `401 Unauthorized` Basic Auth challenge.
- Confirmed `README.md` and `docs/pocketbase-follow-ups.md` contain no `test-provision` or `test-inspect` references.
- Kept native `terraform force-unlock` as separate accepted technical debt and did not alter the separate developer-bootstrap cleanup follow-up.

## Validation

| Command | Result |
| --- | --- |
| `grep -nE 'test-provision|test-inspect' README.md Makefile tests .github || true` | Completed with no stale-reference matches; BSD `grep` reported that `tests` and `.github` are directories because the task-prescribed command does not use `-r`. |
| `grep -nE 'test-provision|test-inspect' README.md docs/pocketbase-follow-ups.md || true` | Passed with no matches. |
| `go build -o dist/united .` | Passed. |
| `pre-commit run --all-files` | Passed. |
| `make test` | Reached PocketBase startup but could not run in this sandbox: binding `127.0.0.1` failed with `operation not permitted`, so the harness could not become ready. |
| `git --no-pager diff --check` | Passed before final report creation. |

## Commit

`docs: describe PocketBase group management`
