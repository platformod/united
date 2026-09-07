# Terraform HTTP backend: DELETE and lock lifecycle

**Conclusion:** Ordinary `terraform destroy` does not invoke HTTP DELETE. Terraform documents destroy as an alias for `terraform apply -destroy`; the remote state manager persists the resulting empty state through `Client.Put` (HTTP POST by default), then the normal lock cleanup invokes UNLOCK.

With locking enabled, the ordinary lifecycle is `LOCK -> state writes (POST/ configured update method, with ?ID=<lock-id>) -> UNLOCK`. The HTTP client `Delete` method is separate: it sends DELETE, accepts only 200, does not add the lock ID, and does not call UNLOCK.

DELETE is for state/workspace deletion, distinct from resource destruction. Terraform workspace deletion explicitly unlocks before calling `Backend.DeleteWorkspace`; the HTTP backend rejects workspace operations because it does not support workspaces. Backend migration likewise copies state and persists the destination; it does not delete the source.

**Atomic-delete answer:** A server may atomically delete state and clear its matching lock only as an explicitly defined deletion policy, with ownership checking and a 200 response. It must not assume this substitutes for UNLOCK. A client sequence `LOCK -> DELETE -> UNLOCK` will report an unlock error if DELETE removed the lock and UNLOCK returns non-200; a server wanting compatibility can make matching post-delete UNLOCK idempotently return 200 without releasing a newer lock. For ordinary destroy, use `LOCK -> POST empty state -> UNLOCK`, not DELETE.

## Exact primary-source citations

- Terraform HTTP backend docs: https://developer.hashicorp.com/terraform/language/backend/http — states GET fetches, POST updates, DELETE purges; locking uses LOCK/UNLOCK; lock ID is added to state update requests.
- Terraform HTTP client: https://raw.githubusercontent.com/hashicorp/terraform/main/internal/backend/remote-state/http/client.go — `Lock` lines 58-103; `Unlock` 105-123; `Put` 181-218 (adds `ID`, default POST); `Delete` 220-237 (DELETE, only 200).
- Terraform destroy command: https://raw.githubusercontent.com/hashicorp/terraform/main/internal/command/apply.go — `helpDestroy` says destroy is a convenience alias for `terraform apply -destroy` (approximately lines 257-267).
- Terraform remote state manager: https://raw.githubusercontent.com/hashicorp/terraform/main/internal/states/remote/state.go — `PersistState` calls `s.Client.Put` (approximately lines 130-193); `Lock`/`Unlock` delegate separately (approximately lines 196-225).
- Terraform remote client interface: https://raw.githubusercontent.com/hashicorp/terraform/main/internal/states/remote/remote.go — `Client` includes separate Get/Put/Delete methods (approximately lines 8-18).
- Terraform backend interface: https://raw.githubusercontent.com/hashicorp/terraform/main/internal/backend/backend.go — separate `StateMgr`, `DeleteWorkspace`, and locking responsibility (approximately lines 44-83).
- Terraform workspace deletion: https://raw.githubusercontent.com/hashicorp/terraform/main/internal/command/workspace_delete.go — unlock occurs before `b.DeleteWorkspace` (approximately lines 91-161; explanatory comment immediately before unlock).
- Terraform HTTP backend: https://raw.githubusercontent.com/hashicorp/terraform/main/internal/backend/remote-state/http/backend.go — only default state is supported; named workspaces and `DeleteWorkspace` return `ErrWorkspacesNotSupported` (approximately lines 210-235).
- Terraform backend migration: https://raw.githubusercontent.com/hashicorp/terraform/main/internal/command/meta_backend_migrate.go — migration copies state, says source remains untouched, locks both states, and persists destination (approximately lines 29-38 and 190-315).
- OpenTofu cross-check: https://raw.githubusercontent.com/opentofu/opentofu/main/internal/backend/remote-state/http/client.go — same separate Lock/Unlock/Put/Delete behavior.

## Upstream tests

- Terraform HTTP client tests: https://raw.githubusercontent.com/hashicorp/terraform/main/internal/backend/remote-state/http/client_test.go — `TestHTTPClient` exercises ordinary remote client operations and `remote.TestRemoteLocks` (approximately lines 20-75).
- Terraform remote state tests: https://raw.githubusercontent.com/hashicorp/terraform/main/internal/states/remote/state_test.go — normal persistence assertions expect Get/Put, including resource removal, not Delete; migration assertions expect Put (tests `TestStatePersist` and `TestWriteStateForMigration`).
- Terraform remote client test fixture: https://raw.githubusercontent.com/hashicorp/terraform/main/internal/states/remote/remote_test.go — mock client has separate Put and Delete methods; Delete is not part of `PersistState` test flow.
- OpenTofu HTTP client tests: https://raw.githubusercontent.com/opentofu/opentofu/main/internal/backend/remote-state/http/client_test.go — test handler has independent LOCK, UNLOCK, DELETE cases; normal `remote.TestClient` and `remote.TestRemoteLocks` exercise them separately.
