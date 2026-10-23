## Purpose

Before this change, an invitation made the invitee a member at once and returned a Kratos recovery link, whoever the invitee was. That does not fit a tenant that requires company sign-in. The members of such a tenant come in through the identity provider of the company, and a new user needs no password to do so. It was also unsafe for an existing account. A recovery link takes over the account, and the service returned the link to the admin who sent the invitation.

An invitation is now a **pending invitation**. The one exception is an invitation for a new address at a tenant that does not require company sign-in. A pending invitation is a row that says "this address can join this tenant until this time". The row becomes a membership when the user signs in to the tenant.

Key decisions:

- One way accepts an invitation: `JoinTenant`. The login UI calls it when it accepts a sign-in to the tenant. We turned down these other ways:
  - The membership at once for an existing account. The account is then a member who never signed in to the tenant.
  - A recovery link for an existing account. The reason is above.
  - Acceptance at registration. This is a second way in, and it has conditions of its own.
- `JoinTenant` also serves auto-join. Both ask the same question, "does this tenant admit this address", and make the same write.
- An invitation is for an address, not for an account. The address can have no account until its first company sign-in creates one.
- The service ignores expired invitations wherever it reads them, and it does not delete them. We did not build a clean-up job, because there is at most one row for each tenant and address.

**Non-goals:**

- Deleting expired invitations.
- Listing or revoking pending invitations.
- Sending the invitation email.
- A check that the user who joins has proved the address. The caller accepted the sign-in.
- A record that a user was removed, so that auto-join does not admit the user again.

## ADDED Requirements

### Requirement: The outcome of InviteMember depends on the invitee and the tenant
The system SHALL handle `InviteMember` (`POST /api/v0/tenants/{tenant_id}/invites`, called by tenant admins through the gateway) with these steps, in this order:

1. Validate the request. `tenant_id` must be a UUID and `email` must be an address. Otherwise the system SHALL answer `INVALID_ARGUMENT`.
2. Ask Kratos if the address has an account.
3. Check that the tenant exists. If not, the system SHALL answer `NOT_FOUND`.
4. Check that the tenant is not a personal tenant. If it is, the system SHALL answer `FAILED_PRECONDITION` with reason `PERSONAL_TENANT`.
5. Check the domain of the address when the effective enforcement of the tenant is `required` and the tenant lists domains. The domain must be one of them. If not, the system SHALL answer `INVALID_ARGUMENT` with reason `DOMAIN_NOT_ALLOWED`.

The outcome then depends on the invitee and the tenant:

| Invitee | Tenant | Outcome | Response |
|---|---|---|---|
| An account that is a member | any | nothing changes | `status: "invited"`, no link or code |
| An account that is not a member | any | a pending invitation | `status: "pending"` |
| An address with no account | effective enforcement `required` | a pending invitation and no account | `status: "pending"` |
| An address with no account | any other | an account, a recovery link and code valid for `INVITATION_LIFETIME`, the membership, and `can_view` published | `status: "invited"`, link, code |

A pending invitation creates no account, no membership and no permission event. In the last row, the system creates the account in Kratos. The account has the address as its `email` trait and uses the default identity schema of Kratos. The system SHALL log the admin action `invite_member` for the last row. The system SHALL log the admin action `invite_member_pending` for a pending invitation. Both admin actions SHALL have the resource `<tenant id>:<address>`. The system SHALL NOT log an admin action when nothing changed.

#### Scenario: An existing account
- **WHEN** a tenant admin invites an address whose account is not a member, at a tenant with any enforcement
- **THEN** the system stores a pending invitation
- **AND** the response is `pending` with no link or code
- **AND** no membership exists and the system publishes nothing

#### Scenario: A new address at a tenant that requires company sign-in
- **WHEN** the address has no account and the effective enforcement of the tenant is `required`
- **THEN** the system stores a pending invitation and creates no account in Kratos

#### Scenario: A new address at any other tenant
- **WHEN** the address has no account and the effective enforcement of the tenant is `off` or `optional`
- **THEN** the system creates the account
- **AND** the response is `invited` with a recovery link and code
- **AND** the account is a member and the system publishes `can_view`

#### Scenario: A member is invited again
- **WHEN** the address belongs to a member of the tenant
- **THEN** the response is `invited` with no link or code
- **AND** the system writes nothing, publishes nothing and logs no admin action

#### Scenario: An address outside the domains of a required tenant
- **WHEN** the tenant's effective enforcement is `required`, the tenant lists domains, and the address's domain is not one of them
- **THEN** the call fails with `INVALID_ARGUMENT` and reason `DOMAIN_NOT_ALLOWED`
- **AND** this is true if the address has an account and if it has no account

### Requirement: Pending invitations expire and can be renewed
The system SHALL keep at most one invitation for each tenant and address. The address SHALL be in lower case, and the database refuses any other. The system SHALL treat an invitation as pending until `INVITATION_LIFETIME` after the system created it or last renewed it. The system SHALL measure this time by the database's clock. When the same address is invited again, the system SHALL renew the expiry.

`ListSignInTenants`, `GetSignInContext`, `JoinTenant` and the personal-tenant check ignore an expired invitation. The system does not delete an invitation when it expires. The system deletes an invitation when its address becomes a member of the tenant by any RPC. The system also deletes the invitations of a tenant with the tenant. The system SHALL refuse to start when `INVITATION_LIFETIME` (default `24h`) is not a positive duration.

#### Scenario: An invitation expires
- **WHEN** `INVITATION_LIFETIME` has passed since an address was invited
- **THEN** `ListSignInTenants` no longer lists the tenant for the address
- **AND** `GetSignInContext` answers `invitation_admits` false
- **AND** `JoinTenant` answers `NOT_ADMITTED`
- **AND** the system still stores the row

#### Scenario: Invited again
- **WHEN** a tenant admin invites an address that has a pending or an expired invitation
- **THEN** there is still one invitation
- **AND** it expires `INVITATION_LIFETIME` from now

### Requirement: What admits an address to a tenant
The system SHALL consider an address that is not a member admitted to a tenant when all of these are true:

- The tenant is enabled.
- The tenant is not a personal tenant.
- At least one of these holds:
  - **Invitation**: the address has a pending invitation to the tenant, and `InviteMember` still invites the address. That means: when the tenant's effective enforcement is `required` and the tenant lists domains, the address's domain is one of them.
  - **Auto-join**: the tenant's effective enforcement is `required`, auto-join is on, and the tenant lists the address's domain.

A tenant can require company sign-in after the system issued an invitation. A tenant can also get its domains after that. An address that the tenant's company sign-ins do not apply to has no sign-in to the tenant. The system keeps the invitation. The invitation admits the address again if the policy changes back.

`GetSignInContext` reports the two conditions separately. `JoinTenant` acts on them. Neither of them checks how the user signed in or if the address is verified. That is the work of the caller.

#### Scenario: Auto-join
- **WHEN** a tenant is `required` with an active binding, has auto-join on and lists `hooli.example`
- **THEN** the tenant admits `hank@hooli.example`
- **AND** the tenant does not admit `hank@elsewhere.example`

#### Scenario: A disabled tenant
- **WHEN** the tenant is disabled
- **THEN** it admits nobody, also if an address has a pending invitation

#### Scenario: The tenant got its domains after the invitation
- **WHEN** `iris@elsewhere.example` has a pending invitation to a `required` tenant, and the domains of the tenant are then set to `initech.example`
- **THEN** the invitation no longer admits her
- **AND** `GetSignInContext` reports `invitation_admits` false
- **AND** `JoinTenant` answers `NOT_ADMITTED`
- **AND** `ListSignInTenants` does not list the tenant for her address
- **AND** when `elsewhere.example` is added to the domains, the same invitation admits her again

### Requirement: JoinTenant is the one way an invitation is accepted
The system SHALL make an account a member of a tenant with `JoinTenant`. `JoinTenant` is an RPC of `TenantSignInService`. The login UI calls it with the id of the tenant and the id of the account when it accepts a sign-in to the tenant. The system serves it over gRPC only, with no HTTP route, so that only the internal network reaches it. This is because the RPC creates a membership on the caller's word that the user signed in.

The system SHALL do these steps in this order:

1. Refuse a `tenant_id` or an `identity_id` that is not a UUID with `INVALID_ARGUMENT`.
2. Read the account from Kratos, and answer `NOT_FOUND` if Kratos does not know it. The system checks the admission for the address of the account. `JoinTenant` takes no address and creates no account.
3. Answer `NOT_FOUND` for an unknown tenant.
4. Succeed without a write when the account is a member already.
5. Answer `FAILED_PRECONDITION` with reason `NOT_ADMITTED` when neither a pending invitation nor auto-join admits the address.
6. Otherwise create the membership and delete the invitation of the address to the tenant. Then publish `can_view` for the account, and log the admin action `join_tenant` with the resource `<tenant id>:<address>`.

The membership and the deletion of the invitation are one transaction. The call takes no lock on the tenant, so it decides on the policy as it read it. No other RPC and no webhook turns an invitation into a membership. `ProvisionUser` makes an address a member directly and deletes its invitation.

#### Scenario: A pending invitation is accepted
- **WHEN** the login UI calls `JoinTenant` for an account whose address has a pending invitation to the tenant
- **THEN** the account is a member
- **AND** the invitation is gone
- **AND** the system publishes `can_view`

#### Scenario: A member already
- **WHEN** the account is a member of the tenant, or a concurrent `JoinTenant` made it a member first
- **THEN** the call succeeds and the system publishes nothing

#### Scenario: Not admitted
- **WHEN** the account is not a member, the address has no pending invitation, and auto-join does not admit the address
- **THEN** the call fails with `FAILED_PRECONDITION` and reason `NOT_ADMITTED`

#### Scenario: Unknown account
- **WHEN** Kratos has no identity with the `identity_id`
- **THEN** the call fails with `NOT_FOUND` and the system writes nothing

#### Scenario: A user registers while invited
- **WHEN** a user whose address has a pending invitation registers an account without a sign-in to the tenant
- **THEN** the account is not a member and has no personal tenant
- **AND** the account becomes a member when `JoinTenant` is called for it while the invitation is pending
