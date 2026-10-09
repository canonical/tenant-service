## Purpose

Inviting someone used to make them a member at once and return a Kratos recovery link, whoever they were. That does not fit a tenant that requires company sign-in: its members come in through the company's identity provider, and a new user needs no password to do so. It was also unsafe for an existing account: a recovery link takes over the account, and the link is returned to the admin who sent the invitation.

An invitation is now a **pending invitation**, unless it is for a new address at a tenant that does not require company sign-in. A pending invitation is a row that says "this address may join this tenant until this time". It becomes a membership when the user signs in to the tenant.

Key decisions:

- An invitation is accepted in one way: `JoinTenant`, which the login UI calls when it accepts a sign-in to the tenant. Turned down: the membership at once for an existing account (a member who never signed in to the tenant); a recovery link for an existing account (see above); accepting at registration (a second way in, with conditions of its own).
- `JoinTenant` also serves auto-join. Both are the same question, "does this tenant admit this address", and the same write.
- An invitation is for an address, not for an account: the address may have no account until its first company sign-in creates one.
- Expired invitations are ignored wherever they are read and are not deleted. A clean-up job was not built: there is at most one row per tenant and address.

**Non-goals:** deleting expired invitations; listing or revoking pending invitations; sending the invitation email; checking that the user who joins has proved the address (the caller accepted the sign-in); remembering that a user was removed, so that auto-join does not admit them again.

## ADDED Requirements

### Requirement: The outcome of InviteMember depends on the invitee and the tenant
The system SHALL handle `InviteMember` (`POST /api/v0/tenants/{tenant_id}/invites`, called by tenant admins through the gateway) by validating the request (`tenant_id` a UUID, `email` an address; otherwise `INVALID_ARGUMENT`), asking Kratos whether the address has an account, and then checking, in this order: the tenant exists (`NOT_FOUND`); it is not a personal tenant (`FAILED_PRECONDITION`, reason `PERSONAL_TENANT`); and, if the tenant's effective enforcement is `required` and it lists domains, the address's domain is one of them (`INVALID_ARGUMENT`, reason `DOMAIN_NOT_ALLOWED`). The outcome is then:

| Invitee | Tenant | Outcome | Response |
|---|---|---|---|
| An account that is a member | any | nothing changes | `status: "invited"`, no link or code |
| An account that is not a member | any | a pending invitation | `status: "pending"` |
| An address with no account | effective enforcement `required` | a pending invitation and no account | `status: "pending"` |
| An address with no account | any other | an account, a recovery link and code valid for `INVITATION_LIFETIME`, the membership, and `can_view` published | `status: "invited"`, link, code |

A pending invitation creates no account, no membership and no permission event. The account of the last row is created in Kratos with the address as its `email` trait and Kratos's default identity schema. The system SHALL log the admin action `invite_member` for the last row and `invite_member_pending` for a pending invitation, with the resource `<tenant id>:<address>`, and none when nothing changed.

#### Scenario: An existing account
- **WHEN** a tenant admin invites an address whose account is not a member, at a tenant with any enforcement
- **THEN** a pending invitation is stored, the response is `pending` with no link or code, no membership exists and nothing is published

#### Scenario: A new address at a tenant that requires company sign-in
- **WHEN** the address has no account and the tenant's effective enforcement is `required`
- **THEN** a pending invitation is stored and no account is created in Kratos

#### Scenario: A new address at any other tenant
- **WHEN** the address has no account and the tenant's effective enforcement is `off` or `optional`
- **THEN** the account is created, the response is `invited` with a recovery link and code, the account is a member and `can_view` is published

#### Scenario: A member is invited again
- **WHEN** the address belongs to a member of the tenant
- **THEN** the response is `invited` with no link or code, and nothing is written, published or logged as an admin action

#### Scenario: An address outside the domains of a required tenant
- **WHEN** the tenant's effective enforcement is `required`, it lists domains, and the address's domain is not one of them
- **THEN** the call fails with `INVALID_ARGUMENT`, reason `DOMAIN_NOT_ALLOWED`, whether or not the address has an account

### Requirement: Pending invitations expire and can be renewed
The system SHALL keep at most one invitation per tenant and address, with the address in lower case (the database refuses any other), and SHALL treat it as pending until `INVITATION_LIFETIME` after it was created or last renewed, by the database's clock. Inviting the same address again SHALL renew the expiry. An expired invitation is ignored by `ListSignInTenants`, by `GetSignInContext`, by `JoinTenant` and by the personal-tenant check, and is not deleted when it expires. An invitation is deleted when its address becomes a member of the tenant by any RPC, and with its tenant. The system SHALL refuse to start when `INVITATION_LIFETIME` (default `24h`) is not a positive duration.

#### Scenario: An invitation expires
- **WHEN** `INVITATION_LIFETIME` has passed since an address was invited
- **THEN** `ListSignInTenants` no longer lists the tenant for the address, `GetSignInContext` answers `invitation_admits` false, `JoinTenant` answers `NOT_ADMITTED`, and the row is still stored

#### Scenario: Invited again
- **WHEN** an address with a pending or an expired invitation is invited again
- **THEN** there is still one invitation, and it expires `INVITATION_LIFETIME` from now

### Requirement: What admits an address to a tenant
The system SHALL consider an address that is not a member admitted to a tenant when the tenant is enabled, is not a personal tenant, and at least one of these holds:

- **Invitation**: the address has a pending invitation to the tenant, and `InviteMember` would still invite it: when the tenant's effective enforcement is `required` and it lists domains, the address's domain is one of them. A tenant may have required company sign-in, or got its domains, after the invitation was issued; an address its company sign-ins do not apply to has no sign-in to it. The invitation is kept, and admits again if the policy changes back.
- **Auto-join**: the tenant's effective enforcement is `required`, auto-join is on, and the tenant lists the address's domain.

`GetSignInContext` reports the two separately; `JoinTenant` acts on them. Neither looks at how the user signed in or whether the address is verified: that is the caller's part.

#### Scenario: Auto-join
- **WHEN** a tenant is `required` with an active binding, has auto-join on and lists `hooli.example`
- **THEN** `hank@hooli.example` is admitted and `hank@elsewhere.example` is not

#### Scenario: A disabled tenant
- **WHEN** the tenant is disabled
- **THEN** it admits nobody, a pending invitation notwithstanding

#### Scenario: The tenant got its domains after the invitation
- **WHEN** `iris@elsewhere.example` has a pending invitation to a `required` tenant, and the tenant's domains are then set to `initech.example`
- **THEN** the invitation no longer admits her: `GetSignInContext` reports `invitation_admits` false, `JoinTenant` answers `NOT_ADMITTED`, and `ListSignInTenants` does not list the tenant for her address
- **AND** when `elsewhere.example` is added to the domains, the same invitation admits her again

### Requirement: JoinTenant is the one way an invitation is accepted
The system SHALL make an account a member of a tenant with `JoinTenant`, an RPC of `TenantSignInService`, which the login UI calls with the tenant's id and the account's id when it accepts a sign-in to the tenant. It is served over gRPC only, with no HTTP route, because it creates a membership on the caller's word that the user signed in. The system SHALL, in this order:

1. Refuse a `tenant_id` or an `identity_id` that is not a UUID with `INVALID_ARGUMENT`.
2. Read the account from Kratos, and answer `NOT_FOUND` if Kratos does not know it. The admission is checked for the account's address: `JoinTenant` takes no address and creates no account.
3. Answer `NOT_FOUND` for an unknown tenant.
4. Succeed without writing anything when the account is a member already.
5. Answer `FAILED_PRECONDITION`, reason `NOT_ADMITTED`, when neither a pending invitation nor auto-join admits the address.
6. Otherwise create the membership, delete the address's invitation to the tenant, publish `can_view` for the account, and log the admin action `join_tenant` with the resource `<tenant id>:<address>`.

The membership and the deletion of the invitation are one transaction. The call takes no lock on the tenant, so it decides on the policy as it read it. No other RPC and no webhook turns an invitation into a membership. `ProvisionUser` makes an address a member directly and deletes its invitation.

#### Scenario: A pending invitation is accepted
- **WHEN** the login UI calls `JoinTenant` for an account whose address has a pending invitation to the tenant
- **THEN** the account is a member, the invitation is gone, and `can_view` is published

#### Scenario: A member already
- **WHEN** the account is a member of the tenant, or a concurrent `JoinTenant` made it one first
- **THEN** the call succeeds and nothing is published

#### Scenario: Not admitted
- **WHEN** the account is not a member and the address has no pending invitation and is not admitted by auto-join
- **THEN** the call fails with `FAILED_PRECONDITION`, reason `NOT_ADMITTED`

#### Scenario: Unknown account
- **WHEN** Kratos has no identity with the `identity_id`
- **THEN** the call fails with `NOT_FOUND` and nothing is written

#### Scenario: A user registers while invited
- **WHEN** a user whose address has a pending invitation registers an account without signing in to the tenant
- **THEN** the account is not a member and has no personal tenant; it becomes a member when `JoinTenant` is called for it while the invitation is pending
