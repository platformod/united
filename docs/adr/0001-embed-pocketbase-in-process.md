# Embed PocketBase in process as the single United server

## Status

Accepted.

## Context

United currently runs a Gin server that writes encrypted state to S3, uses KMS for encryption, and uses Redis for locks. The replatform replaces all three with one PocketBase process that owns group authentication, encrypted versioned state, lock leases, audit, cleanup, and telemetry while preserving the Terraform HTTP backend contract.

## Decision

United embeds PocketBase v0.40.3 in process. PocketBase owns the only HTTP listener, `/api`, migrations, cron, backups, and the dashboard. United owns `/state/{group}/{name}`, `/download/{group}/{name}/{version}`, hooks, crons, crypto, and offline commands. The domain packages `config`, `apperr`, `telemetry`, `crypto`, and `state` never import PocketBase; only the adapters do. v1 source is removed in the first implementation slice; `main` becomes the v0.2 line, and v1 survives only in the `version-0.1x` branch, its tags, and its releases. The replatform ships as one `v0.2.0` release.

## Consequences

- One process and one SQLite writer are the deployment model; there is no clustering, dual-run, or compatibility shim.
- PocketBase's file storage and SQLite are separate failure domains, so publication is metadata-authoritative and orphan reconciliation is required.
- The Terraform contract is implemented as custom routes on PocketBase's router, not as PocketBase collection endpoints.
- The superuser API is the remote operator surface; only the dashboard is network-isolated.
- Offline recovery commands remain in the same binary but never start the server or OTel.

## Considered Options

- **Keep Gin and add PocketBase beside it.** Rejected: two listeners, two auth models, and a permanent compatibility boundary.
- **Run PocketBase as a separate service.** Rejected: an internal HTTP hop for credential verification, more deployment surface, and no benefit for a single-writer system.
- **Develop on a long-lived `replatform` branch.** Rejected: drift, a big-bang review, and a late integration risk. Incremental green slices on `main` are cheaper, and v1 remains recoverable from its branch and tags.
