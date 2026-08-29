# United Terraform State Backend

This context defines the domain language for the multitenant Terraform HTTP backend and its state and lock lifecycle.

## Groups, users, and state

**State document**:
The serialized Terraform state associated with one logical state path. It is the complete document Terraform reads and writes through the HTTP backend.
_Avoid_: statefile, state blob, state object

**Group**:
An access boundary that owns a collection of state paths and the shared Basic Auth credential used by Terraform for those paths. Each group has one human owner, while a human user may own multiple groups.
_Avoid_: tenant bucket, project namespace

**Group route slug**:
The immutable, path-safe identifier used as the `group` segment of a Terraform backend address. It is distinct from the group’s mutable display name.
_Avoid_: group name, URL label

**User**:
A human identity authenticated by PocketBase and related to one or more groups. A user is distinct from the shared Terraform credential used to access a group.
_Avoid_: Terraform identity, state owner

**State path**:
The `group/name` identity Terraform uses to address a state document within a group.
_Avoid_: object key, storage key

**Logical state**:
The group-scoped identity for one state path and the coordination information for its lifecycle, including its current state version and any active state lock.
_Avoid_: state record, state row

**State version**:
An immutable historical instance of a state document associated with a logical state. New writes create new versions while the logical state identifies which version is current.
_Avoid_: statefile record, state object

**Deleted logical state**:
A permanently inactive logical state whose historical versions remain retained but which cannot be read, locked, or revived through the Terraform API. Reusing its path requires a separate backend migration or administrative action.
_Avoid_: archived state, resurrectable state

**Retention cleanup**:
A future policy that permanently removes deleted logical states and their retained versions after a user-selected age. It is intentionally out of scope for the first PocketBase implementation.
_Avoid_: automatic pruning, state resurrection

**Group credential**:
The immutable Basic Auth identity and replaceable password supplied by Terraform on each backend request to authenticate access to a group and all of its state paths. It is an API credential, not a human user login.
_Avoid_: API key, user credential, token

## Locking

**State lock**:
A time-limited lease that prevents competing Terraform operations from mutating the same state document concurrently. It has an owner identity, Terraform lock information, and an expiry; an active lease cannot be reacquired, even by the same owner ID.
_Avoid_: Redis lock, mutex, permanent lock

**Lock owner**:
The Terraform operation identified by the lock ID that currently holds a state lock and may release it.
_Avoid_: authenticated user, tenant

**Expired lock**:
A state lock whose lease has passed its expiry and may be replaced by a subsequent lock acquisition.
_Avoid_: stale mutex

**Force-unlock**:
Terraform's normal `UNLOCK` request used to release a lock when an operation must be recovered manually. It is not a separate administrative API in this domain.
_Avoid_: admin unlock command
