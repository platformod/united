# PocketBase Backend Follow-ups

Group ownership and deletion are handled through owner-scoped PocketBase APIs. A group deletion is a permanent tombstone that retains group state/version data; valid deleted-group Terraform credentials receive `410`, while invalid credentials remain `401`.

Native `terraform force-unlock` support remains a separate accepted-technical-debt investigation; see [`terraform-force-unlock-investigation.md`](terraform-force-unlock-investigation.md).

## Deferred: Developer bootstrap cleanup

Remove obsolete AWS CLI and LocalStack installation requirements from `Brewfile` and `make devprep` in a separate developer-tooling cleanup loop.
