# tenant-sso-policy Specification

## Purpose

A tenant can offer or require sign-in through its own OIDC identity provider. This specification calls that sign-in **company sign-in**. The SSO service stores the settings of the identity provider as a **connection**. This service stores what the tenant does with its connections, which is the **SSO policy** of the tenant. This service stores the SSO policy because a sign-in already asks this service about tenants and memberships.

A policy has 4 parts:

- The **enforcement**. It is `off`, `optional` or `required`. With `optional`, the tenant offers company sign-in. With `required`, company sign-in is the only way in.
- **Auto-join**. With auto-join on, an address in the domains of the tenant becomes a member when its user signs in.
- The **bindings**. A binding is a connection id, and it is active or not active.
- The **domains**. They are the email domains that the policy applies to.

Key decisions:

- Enforcement and auto-join are columns of `tenants`. Bindings and domains are tables. We turned down a policy row of its own for each tenant. It is possible that such a row does not exist yet. Thus the service cannot lock it, and every reader needs a "missing means off" case.
- A write changes one part of the policy. The service locks the row of the tenant and applies the part to the stored policy. Then the service checks the policy as a whole and commits. We turned down a write of the whole policy with a version. Every caller changes one part. A version means a read, a conflict error and a retry loop in another service.
- This service owns every rule that needs no connection data. The SSO service is the only caller, and it checks what only it knows. It checks that a binding names a connection that the tenant owns. It checks that an active binding names a connection that passed a test sign-in.
- The 4 RPCs are a gRPC service of their own, `TenantSSOPolicyService`, with no HTTP route. The gateway authorizes HTTP routes, and a route to these RPCs is a way around the checks of the SSO service.

**Non-goals:**

- A connection that more than one tenant binds.
- Proof that a tenant owns a domain, or any check of a domain other than its syntax.
- A version or a stored history of a policy.
- An SSO policy for a personal tenant.
- The application of the policy at a sign-in. The login UI and the SSO service apply the policy with what `GetSignInContext` tells them.

## Requirements
### Requirement: Stored and effective enforcement
The system SHALL store the enforcement of a tenant as `off`, `optional` or `required`. The stored value SHALL be `off` for a tenant whose policy was never written. The system SHALL treat a tenant with no active binding as off, whatever enforcement the tenant stores. `GetTenantSSOPolicy` returns the stored value. Every decision that depends on enforcement (`GetSignInContext`, `InviteMember`, auto-join) uses the effective enforcement.

#### Scenario: A tenant that never wrote a policy
- **WHEN** a caller reads the policy of a tenant that never wrote one
- **THEN** the answer is enforcement `OFF`, auto-join false, no bindings and no domains

#### Scenario: Optional with nothing active is off
- **WHEN** a tenant stores `optional` and has no bindings, or has only inactive bindings
- **THEN** `GetTenantSSOPolicy` answers `OPTIONAL`
- **AND** `GetSignInContext` answers `OFF` with no connection ids

### Requirement: Reading a policy
The system SHALL answer `GetTenantSSOPolicy` with the stored enforcement, auto-join, domains and bindings of the tenant. The SSO service calls this RPC. The answer SHALL list the domains sorted, and the bindings ordered by connection id. The system SHALL refuse these requests:

- a `tenant_id` that is not a UUID, with `INVALID_ARGUMENT`
- an unknown tenant, with `NOT_FOUND`
- a personal tenant, with `FAILED_PRECONDITION` and the reason `PERSONAL_TENANT`

The system reads the policy in one statement. Thus the answer shows one state of the policy without a transaction.

#### Scenario: Personal tenant
- **WHEN** the tenant is the personal tenant of an account
- **THEN** the call fails with `FAILED_PRECONDITION` and the reason `PERSONAL_TENANT`

### Requirement: Writing enforcement, auto-join and bindings
The system SHALL let the SSO service replace a tenant's enforcement, auto-join and whole set of bindings with `PutTenantSSOPolicy`. The system SHALL leave the stored domains unchanged and SHALL return the policy that results. The system SHALL refuse the write at the first check that fails, and SHALL then write nothing. The system SHALL check these conditions in this order:

1. The request is valid: `tenant_id` and every `connection_id` are UUIDs, and `enforcement` is `OPTIONAL` or `REQUIRED`. Otherwise the system answers `INVALID_ARGUMENT`. The SSO service cannot write `OFF`. A write with no active binding turns a tenant off.
2. The tenant exists. Otherwise the system answers `NOT_FOUND`. The tenant is not a personal tenant. Otherwise the system answers `FAILED_PRECONDITION` with the reason `PERSONAL_TENANT`.
3. The write has at most 5 bindings, and no connection twice. Otherwise the system answers `INVALID_ARGUMENT`.
4. Auto-join needs `REQUIRED` and at least one stored domain. Otherwise the system answers `FAILED_PRECONDITION` with the reason `AUTO_JOIN_NEEDS_REQUIRED_AND_DOMAINS`.
5. `REQUIRED` needs at least one active binding in the new set. Otherwise the system answers `FAILED_PRECONDITION` with the reason `REQUIRED_NEEDS_ACTIVE_BINDING`.
6. No other tenant binds a connection of the new set. Otherwise the system answers `FAILED_PRECONDITION` with the message "a connection is bound by another tenant" and no reason.

#### Scenario: Company sign-in made optional
- **WHEN** the SSO service writes `OPTIONAL`, auto-join false and one active binding for a tenant with stored domains
- **THEN** the service stores the enforcement and the binding
- **AND** the domains are as before
- **AND** the response carries the whole policy

#### Scenario: Required without an active binding
- **WHEN** the write asks for `REQUIRED` and none of its bindings is active
- **THEN** the write fails with `FAILED_PRECONDITION` and the reason `REQUIRED_NEEDS_ACTIVE_BINDING`
- **AND** the stored policy is unchanged

#### Scenario: Auto-join without domains or without required
- **WHEN** the write asks for auto-join
- **AND** the tenant stores no domain, or the enforcement of the write is `OPTIONAL`
- **THEN** the write fails with `FAILED_PRECONDITION` and the reason `AUTO_JOIN_NEEDS_REQUIRED_AND_DOMAINS`

#### Scenario: Too many bindings, or one connection twice
- **WHEN** the write carries 6 bindings, or 2 bindings with the same connection id
- **THEN** the write fails with `INVALID_ARGUMENT`

#### Scenario: A connection another tenant binds
- **WHEN** another tenant already binds one of the connections
- **THEN** the write fails with `FAILED_PRECONDITION`
- **AND** the service stores no part of the write, not even the enforcement

### Requirement: Writing the domains
The system SHALL let the SSO service replace a tenant's whole set of domains with `SetTenantSSODomains`. The system SHALL leave the stored enforcement, auto-join and bindings unchanged and SHALL return the policy that results. An empty set clears the domains. The system trims each domain, changes it to lower case and removes a trailing dot. Duplicates count once. The system sorts the stored set.

The system SHALL check these conditions in this order:

1. `tenant_id` is a UUID. Otherwise the system answers `INVALID_ARGUMENT`.
2. The tenant exists. Otherwise the system answers `NOT_FOUND`. The tenant is not a personal tenant. Otherwise the system answers `FAILED_PRECONDITION` with the reason `PERSONAL_TENANT`.
3. Every domain is a DNS name of at most 253 characters. The name is made of at least 2 labels of letters, digits and inner hyphens. Its last label is at least 2 letters, or is an internationalised top-level label in its ASCII form (`xn--…`). There are at most fifty domains. Otherwise the system answers `INVALID_ARGUMENT`.
4. A tenant with auto-join on keeps at least one domain. Otherwise the system answers `FAILED_PRECONDITION` with the reason `AUTO_JOIN_NEEDS_REQUIRED_AND_DOMAINS`.

Several tenants can list the same domain. The system checks nothing else about a domain.

#### Scenario: Domains normalised
- **WHEN** the SSO service writes the domains ` Hooli.EXAMPLE`, `acme.example.` and `hooli.example`
- **THEN** the tenant stores `acme.example` and `hooli.example`

#### Scenario: Not a domain
- **WHEN** one entry is an address, a URL, a single label, a name with a non-ASCII letter or a name whose last label is numeric
- **THEN** the write fails with `INVALID_ARGUMENT`
- **AND** the stored domains are unchanged

#### Scenario: Clearing the domains of an auto-joining tenant
- **WHEN** the tenant has auto-join on and the write carries no domain
- **THEN** the write fails with `FAILED_PRECONDITION` and the reason `AUTO_JOIN_NEEDS_REQUIRED_AND_DOMAINS`

#### Scenario: Domains written before any enforcement
- **WHEN** the SSO service writes domains for a tenant whose enforcement was never written
- **THEN** the service stores the domains
- **AND** the tenant stays off

### Requirement: Removing one binding
The system SHALL let the SSO service remove one binding with `RemoveTenantSSOBinding`. The SSO service calls this RPC before it deletes the connection. The system SHALL refuse these requests:

- a `tenant_id` or a `connection_id` that is not a UUID, with `INVALID_ARGUMENT`
- an unknown tenant, with `NOT_FOUND`
- a personal tenant, with `FAILED_PRECONDITION` and the reason `PERSONAL_TENANT`
- a removal that leaves a `required` tenant with no active binding, with `FAILED_PRECONDITION` and the reason `REQUIRED_NEEDS_ACTIVE_BINDING`

When the tenant does not bind the connection, the call succeeds and changes nothing.

#### Scenario: The last active binding of a required tenant
- **WHEN** the tenant stores `required` and the binding is its only active binding
- **THEN** the call fails with `FAILED_PRECONDITION` and the reason `REQUIRED_NEEDS_ACTIVE_BINDING`
- **AND** the binding stays

#### Scenario: The last active binding of an optional tenant
- **WHEN** the tenant stores `optional` and the binding is its only active binding
- **THEN** the service removes the binding
- **AND** the tenant is off

#### Scenario: A connection the tenant does not bind
- **WHEN** the policy has no binding to the connection
- **THEN** the call succeeds
- **AND** the service writes nothing and logs no admin action

### Requirement: Policy writes of one tenant are serialized
The system SHALL run each of the 3 writes in one transaction. The transaction SHALL first lock the row of the tenant. Then the transaction SHALL read the stored policy in a statement of its own. Then the transaction SHALL apply the part of the write, check the result and write.

The lock is the lock that an update of the row takes. Another write of the same tenant's row waits for the lock. A new membership or a new invitation of the tenant does not wait. A write that waits for the lock longer than the lock timeout SHALL fail with `ABORTED`. A write whose transaction cannot start SHALL fail with `INTERNAL` after the statement that locks the row, and SHALL write nothing. The reason is that, outside a transaction, the lock ends with that statement.

#### Scenario: Two parts written at the same time
- **WHEN** one request writes the domains of a tenant
- **AND** at the same time another request writes `REQUIRED`, auto-join and an active binding for the same tenant
- **THEN** the second request waits for the first request
- **AND** the second request checks auto-join against the domains that the first request committed
- **AND** the service stores both changes

#### Scenario: The lock is not released in time
- **WHEN** a write waits for the row of the tenant longer than the lock timeout
- **THEN** the write fails with `ABORTED` and writes nothing
- **AND** the caller can repeat the write

#### Scenario: No transaction
- **WHEN** the transaction of the request cannot start
- **AND** the statement that locks the row of the tenant runs on its own
- **THEN** the write fails with `INTERNAL` and writes nothing

#### Scenario: A sign-in during a policy write
- **WHEN** a user joins the tenant while a policy write holds the row of the tenant
- **THEN** the service writes the membership and does not wait for the policy write

### Requirement: The database keeps the policy's shape
The database SHALL refuse each of these:

- an enforcement other than `off`, `optional` and `required`
- auto-join on a tenant whose enforcement is not `required`
- a second binding of one connection, by the same tenant or by another tenant
- a domain that is not lower case
- the same domain twice for one tenant

The database SHALL delete bindings and domains with their tenant. The connection id of a binding refers to a row of the SSO service and has no foreign key. Only the service checks the rules that span tables. Those rules are: `required` needs an active binding, auto-join needs a domain, and the limits are 5 bindings and fifty domains.

#### Scenario: A connection bound twice
- **WHEN** the service inserts a binding for a connection that another tenant binds
- **THEN** the database refuses the binding with a unique violation
- **AND** the service reports the violation as `FAILED_PRECONDITION`

### Requirement: SSO policy RPCs are for the SSO service only
The system SHALL serve `GetTenantSSOPolicy`, `PutTenantSSOPolicy`, `SetTenantSSODomains` and `RemoveTenantSSOBinding` as the gRPC service `TenantSSOPolicyService`. The system SHALL serve them over gRPC only, with no HTTP route. The system SHALL authenticate the caller's token as it does for every other RPC that takes a token. The system SHALL log each write that changed the policy as an admin action (`put_tenant_sso_policy`, `set_tenant_sso_domains`, `remove_tenant_sso_binding`). Each admin action SHALL contain these items:

- the subject of the caller's token as the actor
- the tenant id as the resource
- the part that the write wrote, as it was `before` and as it is `after`. The part is the enforcement, auto-join and bindings, or it is the domains.

Thus the actor is the SSO service. It is the task of the SSO service to record which admin asked for the change. The system writes an admin-action record at log level `info`. Thus at the default `LOG_LEVEL`, which is `error`, the system writes none.

#### Scenario: No HTTP route
- **WHEN** the HTTP routes of the service are listed
- **THEN** no route leads to an SSO policy RPC

#### Scenario: A write that changed the policy
- **WHEN** the SSO service writes `OPTIONAL` and one active binding for a tenant that was off with no bindings
- **THEN** the service logs one admin action `put_tenant_sso_policy`
- **AND** the admin action has the SSO service as the actor and the tenant id as the resource
- **AND** the admin action has the enforcement, auto-join and bindings before and after

#### Scenario: A refused write
- **WHEN** one of the rules refuses a write
- **THEN** the service logs no admin action

