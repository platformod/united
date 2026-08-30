# PocketBase Backend Follow-ups

Group ownership and deletion are handled through owner-scoped PocketBase APIs. A group deletion is a permanent tombstone that retains group state/version data; valid deleted-group Terraform credentials receive `410`, while invalid credentials remain `401`.

## Deferred: Terraform client locking coverage

The integration harness currently validates lock behavior with explicit HTTP requests and runs Terraform lifecycle mutations with `-lock=false`. Add a separate test loop that exercises Terraform's normal `LOCK` and `UNLOCK` behavior during `apply` and `destroy`.

This is separate from the accepted native `terraform force-unlock` investigation documented in [`terraform-force-unlock-investigation.md`](terraform-force-unlock-investigation.md).

## Deferred: Developer bootstrap cleanup

Remove obsolete AWS CLI and LocalStack installation requirements from `Brewfile` and `make devprep` in a separate developer-tooling cleanup loop.
