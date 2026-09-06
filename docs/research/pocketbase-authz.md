# PocketBase authentication and authorization extension points

**Ticket:** [#408 Establish PocketBase authentication and authorization extension points](https://github.com/platformod/united/issues/408)
**Parent:** [#405 Plan United's PocketBase-only replatform](https://github.com/platformod/united/issues/405)
**Branch:** `research/pocketbase-authz`
**Research date:** 2026-09-05
**PocketBase sources:** current documentation v0.40.2 and the current `master` source tree.

## Scope and constraints

This is a fresh design for United, not a reuse of `nrh/plan-two`. The target is:

- one PocketBase server and one writer;
- group-scoped Terraform states;
- human PocketBase users who belong to groups and have roles;
- one shared Terraform Basic Auth credential per group;
- Terraform credentials must be validated without granting the credential a normal PocketBase user session.

The existing United HTTP contract remains the boundary: Terraform sends Basic Auth to state and lock endpoints, while human users use PocketBase authentication for administration and other application behavior.

## Executive recommendation

Use PocketBase as the source of truth for human users, groups, memberships/roles, and group credentials, but do **not** model a shared Terraform credential as a normal interactive user login.

Recommended shape:

```text
users (Auth)
  └── memberships (Base or relation collection) ──> groups (Base)
                                                   └── terraform_credentials (Base)
                                                        - identity/name
                                                        - password (design choice; see below)
                                                        - enabled/revoked/rotatedAt
                                                        - group relation

state records or state metadata
  └── group relation / group identifier
```

Add a private custom route such as `POST /api/united/terraform-auth` or an in-process verifier used directly by the United handlers. It should:

1. parse the Basic Auth username and password;
2. locate the credential by a unique, non-secret identity;
3. call PocketBase's native `Record.ValidatePassword` against the credential record;
4. check `enabled`, rotation/revocation state, and group relation;
5. return only a minimal internal authorization result (`groupId`, credential id, and perhaps credential version/role), never a PocketBase auth response or JWT.

The route may be a custom PocketBase route, but calling it over HTTP from the same process is unnecessary overhead and creates an extra internal trust boundary. Prefer a Go service/helper called by the Terraform handlers. If a route is required for separation or future deployment, keep it private to the fronting proxy/network and return a generic failure for all invalid credentials.

This satisfies “validated without granting it a normal PocketBase user session”: the shared credential is a Base record or otherwise a non-interactive credential record, is password-checked natively, and never passes through `RecordAuthResponse` or receives an auth token.

## Findings from PocketBase

### 1. Auth collections

PocketBase has Base, View, and Auth collections. An Auth collection adds authentication fields including `email`, `emailVisibility`, `verified`, `password`, and `tokenKey`; these are system fields and can be configured but not renamed/deleted. Multiple Auth collections are supported, each with separate login and record-management endpoints.

Auth collections also have a `manageRule` in addition to list/view/create/update/delete rules. This is intended for one authenticated user to manage another auth record, including sensitive fields such as email and password.

**Implication:** use an Auth collection for human users. Do not add every Terraform group credential as a human user merely to reuse the login endpoint: that would conflate a shared machine credential with a person, expose it to normal auth flows, and make group authorization depend on a user-session representation.

Source: [Collections documentation](https://pocketbase.io/docs/collections/).

### 2. Group membership and roles

PocketBase does not impose a separate RBAC product model. The documented patterns are ordinary fields and relations:

- a role can be a single or multiple `select` field on an Auth record;
- ownership and membership can be modeled with relation fields;
- API rules can traverse nested and back-relations;
- multiple expressions can be combined using `&&`, `||`, and parentheses.

For United, a dedicated `memberships` relation collection is preferable to putting one `group` relation and one `role` field directly on `users` if users may belong to multiple groups or have different roles in different groups:

```text
users (Auth)
groups (Base)
group_memberships (Base)
  user  -> users
  group -> groups
  role  -> select: member, operator, admin (example vocabulary)
  active -> bool
```

A direct `users.groups` multi-relation plus a global `users.role` is simpler but cannot represent per-group roles without either a role array convention or a second mapping. The membership collection makes the authorization fact explicit and queryable.

**Role semantics are a United decision, not a PocketBase default.** At minimum define whether `member` can read/write state, whether `operator` can lock/unlock or delete state, and whether `admin` manages membership and credential rotation.

Sources: [Collections documentation](https://pocketbase.io/docs/collections/) and [API rules and filters](https://pocketbase.io/docs/api-rules-and-filters/).

### 3. API rules

Each collection has list, view, create, update, and delete rules. Auth collections additionally have `manageRule`. Rules are both authorization checks and record filters.

Rule states:

- `null` / locked: superuser only;
- empty: guests and authenticated users;
- non-empty expression: only requests satisfying the expression.

Useful documented expressions include `@request.auth.id != ""`, role comparisons such as `@request.auth.role = "staff"`, relation checks such as `allowed_users.id ?= @request.auth.id`, and nested relation lookups. Rules can also inspect request method, headers, query, body, context, and other collections through `@collection.*` joins.

Recommended rule posture:

- leave credential secret fields and credential mutation locked except for a controlled management path;
- do not expose `terraform_credentials` list/view to ordinary users;
- use membership-aware rules for group and state metadata;
- do not rely only on collection rules for Terraform state endpoints, because Terraform's Basic Auth request is intentionally not a PocketBase user-auth request.

A human user’s state visibility can be expressed using a relation-backed rule, for example conceptually:

```text
@request.auth.id != "" &&
@collection.group_memberships.user ?= @request.auth.id &&
@collection.group_memberships.group ?= group
```

The exact expression depends on whether the state record has a `group` relation and on the multi-relation match semantics; test it with representative memberships, including users in multiple groups.

Source: [API rules and filters](https://pocketbase.io/docs/api-rules-and-filters/).

### 4. Hooks

PocketBase's standard extension mechanism is event hooks. Relevant hooks include:

- `OnRecordAuthWithPasswordRequest`: intercepts password authentication, exposes identity/password and the matched record, and permits changing the record before the default response;
- `OnRecordAuthRequest`: runs after successful auth requests;
- `OnRecordCreateRequest`, `OnRecordUpdateRequest`, and `OnRecordDeleteRequest`: validate or alter CRUD requests;
- `OnRecordEnrich`: hide or add computed fields during serialization;
- `OnServe`: register custom routes and middleware.

Hooks are useful for enforcing invariants, for example preventing a non-admin human from changing a credential’s group, or ensuring a credential is disabled when its group is archived. Request hooks are specifically required when request body, headers, or auth context are needed; lower-level record hooks do not have request context.

Do not use `OnRecordAuthWithPasswordRequest` as the primary shared Terraform verifier unless the shared credential is deliberately represented in an Auth collection. The hook belongs to PocketBase's normal auth API and the default successful path returns an auth response/token. A direct `ValidatePassword` call on a non-session credential record is clearer and avoids accidental token issuance.

Source: [Event hooks](https://pocketbase.io/docs/go-event-hooks/).

### 5. Custom routes and middleware

`OnServe` can register custom GET/POST/PUT/PATCH/DELETE/Any routes. Routes can be grouped and bound to middleware. The current request auth state is available as `e.Auth`; builtin middleware includes `apis.RequireAuth`, `apis.RequireSuperuserAuth`, and related helpers. Custom routes can inspect headers and return generic `ApiError` responses.

PocketBase's custom route API is therefore an extension point for a future management API or a dedicated verifier endpoint. A route must not be treated as a security boundary by itself: route registration and network/proxy policy still need to prevent arbitrary public access to credential-validation internals.

For human management routes, use `apis.RequireAuth()` and then perform group-role checks against the authenticated user and membership record. For Terraform state routes, keep United's Basic Auth middleware and call the verifier directly; do not bind `apis.RequireAuth()` because the Terraform credential is not a PocketBase session.

Source: [Go routing](https://pocketbase.io/docs/go-routing/).

### 6. Native password verification

PocketBase exposes `record.ValidatePassword(pass)`. The current password-auth implementation:

1. finds the record using a configured identity field and requires a unique index;
2. invokes `ValidatePassword`;
3. performs a dummy password check when no record is found to reduce enumeration timing differences;
4. returns a generic authentication failure;
5. only on success creates the normal auth response through `RecordAuthResponse`.

The important reusable primitive is `ValidatePassword`, not the normal auth endpoint. United can locate a credential record by a unique public identifier and call `ValidatePassword` directly. The stored password remains a PocketBase-managed hash; United never needs the plaintext or to duplicate PocketBase's hashing parameters.

Source: current PocketBase implementation [`apis/record_auth_with_password.go`](https://github.com/pocketbase/pocketbase/blob/master/apis/record_auth_with_password.go), especially `recordAuthWithPassword`, `dummyPasswordCheck`, and `Record.ValidatePassword` usage. The same behavior is reflected in the [record operations documentation](https://pocketbase.io/docs/go-records/).

Security requirements for United's verifier:

- perform a dummy `ValidatePassword` against a representative credential record when the identity is absent, or otherwise preserve comparable work;
- use a generic 401/403 response and avoid revealing whether the group, identity, or password was wrong;
- rate-limit or otherwise protect repeated failures;
- never log Basic Auth values or raw password errors;
- bind the authorization result to the requested group/state path after credential validation.

### 7. Credential rotation and revocation

PocketBase auth tokens are stateless and not stored as server sessions. The documentation states that changing an individual superuser password invalidates tokens for that account, while changing the shared auth-token secret invalidates all tokens for that collection. Ordinary auth records also have a `tokenKey`, and the record API exposes `RefreshTokenKey`/`SetTokenKey`.

That token behavior is not the right rotation mechanism for a shared Terraform credential that never receives a token. Rotation should instead be explicit in the credential model:

```text
terraform_credentials
  id
  identity              unique, non-secret lookup name
  group                 relation -> groups
  password              PocketBase-managed password field, or a dedicated Auth record if required
  enabled               bool
  credentialVersion     number or text
  rotatedAt             date
  revokedAt             date/empty
```

Rotation transaction:

1. authenticate an authorized human administrator;
2. generate a new high-entropy shared secret outside logs/UI history;
3. update the password through PocketBase record APIs so it is hashed by PocketBase;
4. increment `credentialVersion` and set `rotatedAt`;
5. optionally disable the old credential first or use a short, explicitly documented overlap window;
6. distribute the new Basic Auth secret through the deployment/secret-management channel;
7. verify old credentials fail and the new credential resolves only to the same group.

If the product requires atomic old/new overlap, represent two credential records for the same group with explicit `notBefore`/`expiresAt`, rather than relying on an undocumented password-history behavior. The default recommendation is no overlap: one active credential per group, replaced atomically in one PocketBase write transaction.

### 8. Stateless auth and the no-session requirement

PocketBase's authentication is stateless: a client is authenticated when it sends a valid `Authorization: TOKEN` header; tokens are not stored in the database. The normal password endpoint returns a token and record data. PocketBase has no dedicated token verification endpoint; `authRefresh` verifies a token and returns a new one.

These facts reinforce the boundary:

- human browser/API clients may use normal PocketBase auth tokens;
- Terraform's Basic Auth password is a group capability, not a human identity;
- the Terraform verifier should return an internal group authorization result, not mint a PocketBase token, call `authRefresh`, or create a session-like cache;
- request authorization must be evaluated on every state/lock operation, so disabling a group or rotating its credential takes effect immediately on the next request.

Source: [Authentication](https://pocketbase.io/docs/authentication/) and [record operations](https://pocketbase.io/docs/go-records/).

## Proposed United authorization model

### Human path

1. Human authenticates with the `users` Auth collection using password/OAuth/other enabled methods.
2. PocketBase loads the user record into the request auth context.
3. United management handlers resolve active `group_memberships` for that user.
4. The handler checks the membership role and requested group/state.
5. PocketBase API rules provide baseline filtering; handler-level checks remain the final authorization decision for operations that span multiple records or external state storage.

### Terraform path

1. Terraform sends Basic Auth to the existing HTTP backend endpoint.
2. United parses the credential and derives the requested group/state scope from the route contract.
3. United locates the active credential by a unique identity and calls native `ValidatePassword`.
4. United checks credential enabled/expiry/version and obtains its group relation.
5. United rejects if the credential group does not match the requested state group.
6. United performs the Terraform operation against the single writer/state store.

No `@request.auth` is expected on this path, and no PocketBase user token is created.

### State scope

Every state record or state metadata row should have an explicit group relation/identifier. Do not infer group scope solely from a credential username or from a user’s current memberships. The credential lookup establishes a group capability; the state row establishes the target; authorization compares the two.

If the state remains in S3, PocketBase should hold the authoritative group/credential metadata and United should preserve the existing encrypted S3 object behavior. If state metadata moves into PocketBase, use a collection with a group relation and locked mutation rules, while keeping the single-writer constraint explicit.

## Options considered

### A. Shared credential as a normal Auth user

**Rejected as the default.** It allows reuse of `authWithPassword`, but it creates a pseudo-user for a group credential, invites token issuance, complicates human/user semantics, and risks treating a shared secret as a session identity. It can be made safe only with a dedicated Auth collection plus a custom endpoint that stops before token issuance, but then the Auth collection is mostly being used as a password-hash container.

### B. Shared credential as a Base record with a password-like field

**Recommended if PocketBase's schema/API permits the required password hashing for that record type.** The direct `Record.ValidatePassword` primitive is available on records, but implementation should verify at the selected PocketBase version that a Base record receives the same password field validation/hash behavior as an Auth record. If it does not, use option C rather than duplicating hashing.

### C. Dedicated non-interactive Auth collection

**Safe fallback and likely implementation choice if Base records cannot use native password hashing.** Create an Auth collection such as `terraform_credentials`, disable all interactive methods and public CRUD rules, and store one record per group. Validate with `FindFirstRecordByData`/unique identity plus `ValidatePassword`, but call the method directly and never call the normal auth response path. This preserves native PocketBase hashing while keeping the records out of the human `users` collection.

The remaining implementation question is whether PocketBase allows the desired password field and `ValidatePassword` behavior on Base records; current docs clearly document the primitive on `Record`, while Auth collections are the documented home for password-auth system fields. Verify this with a focused prototype/test before committing to B.

## Decision questions newly surfaced

1. **Credential record type:** Can the selected PocketBase version safely hash and validate passwords on a Base record, or should United use a dedicated non-interactive Auth collection? This is the highest-priority prototype.
2. **Credential identity:** What non-secret username/identity should Terraform send, and is it stable through group renames? Prefer an immutable credential id/name rather than a human email or group display name.
3. **One credential invariant:** Does “one shared credential per group” mean exactly one active record, or may old/new records overlap during rotation? The recommendation is exactly one active record and atomic replacement.
4. **Role matrix:** Which human roles may read state, write state, lock/unlock, delete state, manage memberships, and rotate the group credential?
5. **Membership cardinality:** May a human belong to multiple groups, and can a membership have only one role or multiple roles? This determines whether `group_memberships` is mandatory versus a simpler user multi-relation.
6. **Credential administration:** Can a group admin rotate only their own group credential, or only a global/superuser operator? Define the API rule and handler transaction boundary.
7. **State metadata authority:** Will PocketBase own state metadata only while S3 remains the blob store, or will state blobs move into PocketBase? The authorization model works for either, but consistency and backup requirements differ.
8. **Failure and rate limits:** What rate limit, audit event, and alerting policy applies to failed Terraform Basic Auth attempts and credential rotation?
9. **Secret distribution:** Where are group credentials generated and delivered, and how is the first credential issued without exposing it to PocketBase record responses or logs?
10. **Single-writer deployment:** Is the one-server/one-writer constraint permanent, or must the design leave room for a later multi-instance deployment? If later, all credential/state writes need a migration path to serialized transactions or an external coordinator.

## Implementation checklist

- [ ] Prototype the selected PocketBase version for Base versus dedicated Auth credential records.
- [ ] Create collections and migrations from `main`; do not copy the `nrh/plan-two` implementation.
- [ ] Add unique indexes for credential identity and any password-auth identity field.
- [ ] Lock credential list/view and secret-bearing fields from ordinary users.
- [ ] Implement native password verification with dummy-check behavior for missing identities.
- [ ] Compare credential group to requested state group on every Terraform operation.
- [ ] Add explicit role-aware human management authorization and tests for cross-group access.
- [ ] Implement atomic rotation/revocation and tests proving old credentials fail immediately.
- [ ] Add generic errors, rate limiting, audit logging without secrets, and no-token assertions.
- [ ] Test users with multiple groups, multiple roles, disabled memberships, disabled groups, missing credentials, wrong-group credentials, and concurrent lock/state requests.

## Source index

- [PocketBase Authentication](https://pocketbase.io/docs/authentication/)
- [PocketBase Collections](https://pocketbase.io/docs/collections/)
- [PocketBase API rules and filters](https://pocketbase.io/docs/api-rules-and-filters/)
- [PocketBase Go record operations](https://pocketbase.io/docs/go-records/)
- [PocketBase Go routing](https://pocketbase.io/docs/go-routing/)
- [PocketBase Go event hooks](https://pocketbase.io/docs/go-event-hooks/)
- [PocketBase current password-auth implementation](https://github.com/pocketbase/pocketbase/blob/master/apis/record_auth_with_password.go)
- [PocketBase source repository](https://github.com/pocketbase/pocketbase)
