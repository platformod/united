# United

United stores and coordinates Terraform state for independently administered groups.

## Language

**Group**:
A tenant boundary with an immutable identity that owns one namespace of logical Terraform states and one shared Terraform credential.
_Avoid_: Tenant, organization, account

**Logical state**:
A permanently reserved Terraform state lineage whose identity is established for one group and state name by its first successful lock or state write, even when no state document has yet been published.
_Avoid_: State version, state path

**State name**:
A group-scoped lowercase ASCII route identifier that permanently names one logical state and permits only letters, digits, dots, underscores, and hyphens.
_Avoid_: Display name, storage path, filename

**State document**:
The opaque content supplied by Terraform and returned byte-for-byte without interpretation or modification by United.
_Avoid_: State metadata, normalized state

**State integrity metadata**:
The exact plaintext byte count and SHA-256 digest recorded when United receives a state document and required to match after every successful decryption; it verifies content but never identifies or deduplicates a state.
_Avoid_: HTTP Content-Length, state identity, ciphertext metadata

**Encrypted state object**:
The immutable stored ciphertext produced from one state document for a prospective state version; it becomes authoritative only when referenced by committed state-version metadata.
_Avoid_: State document, state version, database record

**Orphaned state object**:
An encrypted state object not referenced by authoritative state-version metadata, giving it no state semantics and making it eligible for physical removal.
_Avoid_: Non-current version, purged version, missing state object

**State version**:
A published immutable historical snapshot belonging to one logical state; exactly one version is current while that state is active, and every other version is non-current.
_Avoid_: Logical state, current state

**Current version pointer**:
A logical state's reference to the last state version it selected; it is served while the state is active, remains the sole restoration target while tombstoned, and is cleared rather than reassigned when that version is purged.
_Avoid_: Latest retained version, version sequence, fallback version

**Non-current version**:
A state version that has been superseded or belonged to a logical state when it was tombstoned, making it subject to the group retention policy from that time.
_Avoid_: Tombstoned state, purged state

**Cleanup-eligible version**:
A non-current version older than the group retention policy that remains visible and accessible until cleanup removes it; a policy change may make it ineligible again before removal.
_Avoid_: Expired version, claimed version, purged version

**Purged version**:
A state version whose authoritative metadata and content association have been permanently removed, making its state document inaccessible.
_Avoid_: Cleanup-eligible version, tombstoned state

**Tombstoned state**:
A logical state that is absent through the Terraform API while its identity remains reserved; its retained former current version is its sole restoration target until cleanup removes it, and older versions never substitute.
_Avoid_: Purged state, deleted state

**Purged state**:
A logical state with no retained versions remaining under the group retention policy while its identity remains permanently reserved.
_Avoid_: Tombstoned state, reusable state name

**Empty logical state**:
A permanently reserved logical state established by an initial lock but having no published state version; it remains absent through the Terraform read API until its first successful write.
_Avoid_: Tombstoned state, purged state, unused path

**Group retention policy**:
The owner-controlled, service-bounded age threshold at which cleanup may purge a non-current version, measured from when that version ceased to be current.
_Avoid_: Stored deadline, group retirement, manual purge

**State restoration**:
An owner action that returns a tombstoned logical state to use by making its retained former current version current or, for a never-published lineage, returning it to empty.
_Avoid_: Path reuse, rollback

**Lock lease**:
A time-bounded exclusive claim by one Terraform operation over one logical state, identified by the operation's opaque lock ID and recoverable through expiry.
_Avoid_: Database lock, indefinite lock, user lock

**Terraform lock payload**:
The complete client-supplied description of a lock lease, whose opaque ID establishes operation ownership while its remaining fields provide unverified diagnostics returned to competing Terraform clients.
_Avoid_: Authenticated user identity, security audit event, server-generated lock

**Expired lock lease**:
A lock lease whose expiry has passed and therefore cannot admit a new protected operation or exclude a new acquisition, though an authoritative mutation admitted while it was live may finish.
_Avoid_: Active lock, retained lock, cleanup-eligible lock

**Suspended group**:
A group whose state data plane is frozen while limited owner controls remain available for remediation.
_Avoid_: Deleted group, read-only group

**Pending retirement**:
A reversible group status entered when an active-group owner, or a system operator for a suspended group, requests retirement, freezing the state data plane until cancellation or automatic purge; owners may cancel whenever the group is not retirement-eligible.
_Avoid_: Suspended group, retired group

**Retirement-eligible group**:
A group that has remained in pending retirement longer than its current group retention policy and may be automatically purged unless a policy extension or cancellation commits first; only a system operator may cancel while the group remains eligible.
_Avoid_: Pending retirement, cleanup-eligible version, retired group

**Retired group**:
A permanently purged group represented only by its reserved immutable machine identity and original creation, latest retirement request, and completed purge timestamps after its namespace, access relationships, credential, mutable display name, key material, and state history have been removed.
_Avoid_: Pending retirement, suspended group, purged state

**User**:
A human identity that may belong to multiple groups through separate memberships.
_Avoid_: Member, operator

**Membership**:
The association between a user and a group, carrying exactly one group role after an invitation is accepted.
_Avoid_: Group user

**Invitation**:
A revocable, expiring request from an owner for an existing user to join a group as a member.
_Avoid_: Membership, invite link

**Owner**:
The privileged group role responsible for the group's continued stewardship.
_Avoid_: Admin

**Member**:
The non-owning group role assigned to an accepted group participant.
_Avoid_: User

**Terraform credential**:
The group-bound machine identity shared by Terraform clients, consisting of a public username that remains permanently reserved and a secret known only at issuance.
_Avoid_: User credential, membership, reusable username

**Disabled credential**:
A Terraform credential temporarily barred from authenticating while retaining the same username and secret for possible reactivation.
_Avoid_: Rotated credential, suspended group

**System operator**:
A global administrator responsible for service-level suspension and exceptional recovery without belonging to groups.
_Avoid_: Owner, member

**Runtime master key**:
The deployment-held secret that protects every operational group's data key, is never persisted with application data, and whose loss makes all retained state documents permanently inaccessible.
_Avoid_: Group data key, Terraform credential

**Master-key generation**:
The informational ordinal stored with application data that begins at one and advances only when a completed rekey replaces the runtime master key; it helps operators associate backups with externally retained keys but does not validate key compatibility.
_Avoid_: Key identifier, backup generation, group data-key version

**Group data key**:
A secret created with a group and retained for its operational lifetime that protects the group's state documents while itself remaining protected by the runtime master key.
_Avoid_: Runtime master key, Terraform credential, per-version key

**Global rekey**:
An offline, all-or-nothing replacement of the runtime master key that re-protects every non-retired group's existing data key and advances the master-key generation without rewriting state documents.
_Avoid_: Group-key repair, group data-key rotation, state re-encryption

**Group-key repair**:
An exceptional offline re-protection of one non-retired group's existing data key using operator-supplied old and current runtime master keys, without advancing the master-key generation or restoring retired key material.
_Avoid_: Global rekey, group data-key rotation, retired-group recovery

**Cryptographic erasure**:
Permanent removal of a group's protected data key, making any residual encrypted state documents inaccessible without requiring their physical removal to succeed first.
_Avoid_: Physical object deletion, tombstoning, key rotation

**Security audit event**:
An immutable, permanently retained record of a security-significant administrative action or state-authority transition, available only to system operators and preserved after group retirement.
_Avoid_: Request log, metric
