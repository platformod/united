# United

United stores and coordinates Terraform state for independently administered groups.

## Language

**Group**:
A tenant boundary with an immutable identity that owns one namespace of logical Terraform states and one shared Terraform credential.
_Avoid_: Tenant, organization, account

**Suspended group**:
A group whose state data plane is frozen while limited owner controls remain available for remediation.
_Avoid_: Deleted group, read-only group

**Pending retirement**:
A reversible group status entered when an owner requests deletion, freezing the state data plane until cancellation or eventual operator-authorized purge.
_Avoid_: Suspended group, deleted group

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
The single machine credential shared by Terraform clients acting within one group.
_Avoid_: User credential, membership

**System operator**:
A global administrator responsible for service-level suspension and exceptional recovery without belonging to groups.
_Avoid_: Owner, member
