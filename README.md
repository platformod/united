# United: a multitenant Terraform HTTP backend

United is a PocketBase-based Terraform HTTP backend designed to run alongside [Atlantis](https://www.runatlantis.io/). It stores encrypted Terraform state versions and lock metadata in PocketBase, with one shared Terraform credential per group.

## Requirements

- The United binary (or the published container image)
- A **base64-encoded, 32-byte** `UNITED_STATE_MASTER_KEY`
- Durable, access-controlled storage for PocketBase's `pb_data` directory and backups for that directory
- A network location reachable only by Terraform clients such as Atlantis

United has no external object-store, key-management, distributed-lock, cloud CLI, or local cloud-emulation runtime requirement.

## Run United

Provision a key **once** in your secret-management system, store it outside the repository, and inject that same persistent value on every start. For example, generate the initial value only during secret provisioning, not in a startup command:

```bash
openssl rand -base64 32
```

Configure the resulting value as `UNITED_STATE_MASTER_KEY` in your deployment's secret mechanism, then start United without generating or replacing it:

```bash
./dist/united serve --dir=./pb_data
```

`pb_data` contains PocketBase's SQLite data and the protected encrypted state files. Mount it on durable storage, restrict filesystem access to the service account, and back it up together as one unit. A backup without the corresponding master key cannot restore encrypted state, so protect and retain the key in an appropriate secret manager.

The container image starts United with `serve --dir=/pb_data` and declares `/pb_data` as a volume. Mount durable, protected storage there and inject the **previously provisioned, persistent** `UNITED_STATE_MASTER_KEY` through your deployment's secret mechanism; do not generate a key in the container startup command:

```bash
# UNITED_STATE_MASTER_KEY is supplied by the deployment's secret mechanism.
docker run --rm \
  --mount type=volume,source=united-pb-data,target=/pb_data \
  --env UNITED_STATE_MASTER_KEY \
  ghcr.io/platformod/united:latest
```

## Terraform configuration

United preserves Terraform's HTTP backend endpoint paths. A state path is `/state/:group/:name`; use the same path for the state, lock, and unlock endpoints.

```terraform
terraform {
  backend "http" {
    address        = "https://united.example.com/state/platform/network"
    lock_address   = "https://united.example.com/state/platform/network"
    unlock_address = "https://united.example.com/state/platform/network"
  }
}
```

Configure Terraform with the group credential through its normal HTTP backend environment variables, supplied from a secret manager or your automation environment:

```bash
export TF_HTTP_USERNAME
export TF_HTTP_PASSWORD
terraform init
terraform plan
```

A group credential is an API credential for Terraform only. It cannot be used to log in to PocketBase and is not a human user account. The group route slug and credential username are immutable because they identify existing Terraform backend addresses. Rotating the group password immediately invalidates the old password; distribute the replacement to Terraform clients without changing the backend path.

## Group management

Human users manage groups through PocketBase's authenticated collection APIs. Those APIs are owner-scoped: a signed-in user can create, list, view, update, and delete only groups they own. The service assigns group ownership at creation, so callers cannot create a group for another user; route slugs, Terraform credential usernames, and ownership remain immutable. A route slug is one lowercase alphanumeric path segment with optional single hyphens between alphanumeric components (for example, `platform` or `platform-prod`); spaces, slashes, dot segments, percent encoding, and query delimiters are rejected.

Deleting a group permanently creates an API tombstone rather than removing its records. The tombstone cannot be updated or revived through the normal group APIs and does not remove the group's logical states, state versions, or encrypted state files; their retention behavior is unchanged. State requests with that group's valid Basic Auth username and password receive `410 Gone`; this lets authorized Terraform clients distinguish a deleted group from a temporary failure. Requests for an unknown group, or with a missing or invalid username or password, always receive the generic `401 Unauthorized` Basic Auth challenge and do not reveal whether a group exists or was deleted.

## State lifecycle and locks

Each successful state write creates an immutable encrypted state version. The logical state selects the current version while encrypted historical versions remain in protected PocketBase storage.

Deleting a logical state creates a permanent tombstone: it cannot be read, locked, or recreated through the Terraform API, while retained encrypted versions remain available for the future retention policy. Group tombstones likewise retain the group's logical states, versions, and encrypted files. Automatic hard-delete retention cleanup is intentionally out of scope; a future administrative cleanup process must remove deleted logical states and their retained files according to a chosen retention period.

State locks are server-enforced 35-minute leases. An expired lease may be replaced by a subsequent lock acquisition. Native `terraform force-unlock` support is accepted technical debt; see [`docs/terraform-force-unlock-investigation.md`](docs/terraform-force-unlock-investigation.md) before relying on it for recovery procedures.

## Migrating an existing backend

United does not automatically import or migrate state from an existing Terraform backend. Plan the migration per state path, keep a recoverable backup of the old backend, and use Terraform's backend migration flow (for example, `terraform init -migrate-state`) when switching the configuration to United. Verify the resulting state and lock behavior before retiring the previous backend.

## Operational security

- Terminate TLS at a trusted fronting proxy and encrypt traffic between Terraform clients and that proxy.
- Restrict network access to trusted Terraform automation; do not expose the service or PocketBase administration interface to the public internet.
- Protect `pb_data`, backups, group credentials, and `UNITED_STATE_MASTER_KEY` as sensitive data. Do not commit them or log their values.
- Use distinct group credentials and rotate their passwords through a managed secret-distribution process.

## Development

```bash
make devprep
# Inject a previously provisioned persistent key through your shell or secret tool.
make run
make test
```

`make run` starts United with local PocketBase data in `./pb_data` through Air and requires the same previously provisioned `UNITED_STATE_MASTER_KEY` used for that data. `make test` runs the standalone Terraform integration harness with its own ephemeral test key and without cloud-service, distributed-lock, or reverse-proxy dependencies.

## Copyright

Copyright (C) 2026 Platform OnDemand, Inc - All Rights Reserved

This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.
