# Enforce the user account lifecycle

## Status

Accepted. Completes the account requirements of issues #409, #416, and #420, which the implementation plan referenced but did not implement.

## Context

The plan named the outcome — "the final active owner cannot … be deactivated or deleted" — but had no mechanism behind it:

- PocketBase's built-in `users` collection has no `disabled` field, so the planned `authRule` (`verified = true && disabled != true`) and `PATCH users {"disabled": true}` referenced a field that did not exist.
- No users hook, store method, test, or audit event existed for deactivation, reactivation, deletion, or exceptional owner recovery.
- PocketBase evaluates `authRule` only when it issues a token. It does not re-evaluate it on every request, and the default `users` token lives five days, so a deactivated account would have kept working sessions, including the owner download route.
- Open registration with `verified = true` depends on SMTP, which the plan never mentioned.

## Decision

### Schema and rules

The migration extends the built-in `users` collection:

- a `disabled` bool field, default `false`, readable by the user and superusers, never writable except through the lifecycle hook;
- `authRule = verified = true && disabled != true`;
- registration open (`createRule` allows guests, no group granted);
- list and view limited to the authenticated user's own record;
- update limited to the authenticated user's own record, with the field allowlist enforced by the hook, not the rule.

### Deactivation and reactivation

Deactivation sets `disabled=true`, calls `RefreshTokenKey()` so every existing session fails on its next request, and preserves memberships and pending invitations. It is allowed for the user themselves and for a superuser. It writes `user_deactivated` with the actor in the same transaction.

Reactivation clears `disabled` and is superuser-only. It writes `user_reactivated`. It does not restore revoked sessions; the user signs in again.

### Deletion

Permanent deletion removes the user, their memberships (cascade relation), and their pending invitations (cascade relation), and writes `user_deleted` in the same transaction. It is allowed for the user themselves and for a superuser.

### Final-owner guard

Deactivation or deletion is rejected with `409` when the user is the only active owner of any group that is active or pending retirement. The check passes when another active owner exists, or when the affected group is suspended, matching #409. Every group the user owns is checked in the same transaction, not only the group in the request's context.

### Exceptional owner recovery

When a group has lost its final usable owner, a superuser may promote an existing member or add an existing user as an owner, but only while the group is suspended. The operation writes `owner_recovered` with the operator as actor and the group as subject. A superuser never becomes a member through this path.

### Password changes and registration

A self-service password change writes `user_password_changed`. Registration verification requires SMTP, configured through PocketBase `_params` by a superuser, and documented as a deployment requirement.

## Consequences

- Deactivation is immediate and global for the account: every device is signed out. This is intentional; the alternative is a per-request authorization check on every route.
- The slice 3 rule matrix gains the `users` collection, and the audit catalog gains `user_deactivated`, `user_reactivated`, `user_deleted`, `user_password_changed`, and `owner_recovered`.
- The plan gains `internal/hooks/users.go`, `internal/store/users.go`, a users migration step, account task tests, and an operator how-to covering SMTP, registration, deactivation, and recovery.
- `CONTEXT.md` records that deactivation revokes sessions.

## Considered Options

- **Re-check `authRule` on every request via a middleware.** Rejected: it adds a database read to every request and duplicates PocketBase's auth model for the same outcome.
- **Set `disabled=true` without rotating the token key.** Rejected: a deactivated or compromised account would keep access for up to the token lifetime.
- **Hard-delete the account on deactivation.** Rejected: #409 requires deactivation to preserve memberships and invitations.
