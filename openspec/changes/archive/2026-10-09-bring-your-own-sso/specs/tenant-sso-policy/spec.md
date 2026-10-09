## Purpose

A tenant can offer or require sign-in through its own OIDC identity provider, called **company sign-in** here. The SSO service stores the provider's settings as a **connection**. This service stores what the tenant does with its connections, the tenant's **SSO policy**, because a sign-in already asks this service about tenants and memberships.

A policy has four parts: the **enforcement** (`off`; `optional`: company sign-in is offered; `required`: it is the only way in), **auto-join** (an address in the tenant's domains becomes a member by signing in), the **bindings** (a connection id, active or not) and the **domains** (the email domains the policy applies to).

Key decisions:

- Enforcement and auto-join are columns of `tenants`; bindings and domains are tables. A policy row of its own per tenant was turned down: it may not exist yet, so it cannot be locked, and every reader needs a "missing means off" case.
- A write changes one part of the policy. The service locks the tenant's row, applies the part to what is stored, checks the policy as a whole and commits. Writing the whole policy with a version was turned down: every caller changes one part, and a version means a read, a conflict error and a retry loop in another service.
- This service owns every rule that needs no connection data. The SSO service, the only caller, checks what only it knows: a binding names a connection the tenant owns, and an active one a connection that passed a test sign-in.
- The four RPCs are a gRPC service of their own, `TenantSSOPolicyService`, with no HTTP route. The gateway authorizes HTTP routes, and a route to these RPCs would lead around the SSO service's checks.

**Non-goals:** a connection bound by more than one tenant; proof that a tenant owns a domain, or any check of a domain beyond its syntax; a version or a stored history of a policy; an SSO policy for a personal tenant; applying the policy at a sign-in, which the login UI and the SSO service do with what `GetSignInContext` tells them.

## ADDED Requirements

### Requirement: Stored and effective enforcement
The system SHALL store a tenant's enforcement as `off`, `optional` or `required`, with `off` for a tenant whose policy was never written, and SHALL treat a tenant with no active binding as off whatever it stores. `GetTenantSSOPolicy` returns the stored value. Every decision that depends on enforcement (`GetSignInContext`, `InviteMember`, auto-join) uses the effective one.

#### Scenario: A tenant that never wrote a policy
- **WHEN** the policy of a tenant that never wrote one is read
- **THEN** the answer is enforcement `OFF`, auto-join false, no bindings and no domains

#### Scenario: Optional with nothing active is off
- **WHEN** a tenant stores `optional` and has no bindings, or only inactive ones
- **THEN** `GetTenantSSOPolicy` answers `OPTIONAL` and `GetSignInContext` answers `OFF` with no connection ids

### Requirement: Reading a policy
The system SHALL answer `GetTenantSSOPolicy`, called by the SSO service, with the tenant's stored enforcement, auto-join, domains (sorted) and bindings (ordered by connection id). It SHALL refuse a `tenant_id` that is not a UUID with `INVALID_ARGUMENT`, an unknown tenant with `NOT_FOUND` and a personal tenant with `FAILED_PRECONDITION`, reason `PERSONAL_TENANT`. The policy is read in one statement, so the answer shows one state of it without a transaction.

#### Scenario: Personal tenant
- **WHEN** the tenant is an account's personal tenant
- **THEN** the call fails with `FAILED_PRECONDITION` and reason `PERSONAL_TENANT`

### Requirement: Writing enforcement, auto-join and bindings
The system SHALL let the SSO service replace a tenant's enforcement, auto-join and whole set of bindings with `PutTenantSSOPolicy`, leave the stored domains unchanged, and return the resulting policy. It SHALL check, in this order, and refuse at the first check that fails without writing anything:

1. The request: `tenant_id` and every `connection_id` are UUIDs and `enforcement` is `OPTIONAL` or `REQUIRED`; otherwise `INVALID_ARGUMENT`. `OFF` cannot be written: a tenant is turned off by a write with no active binding.
2. The tenant exists (`NOT_FOUND`) and is not a personal tenant (`FAILED_PRECONDITION`, reason `PERSONAL_TENANT`).
3. At most five bindings, and no connection twice; otherwise `INVALID_ARGUMENT`.
4. Auto-join needs `REQUIRED` and at least one stored domain; otherwise `FAILED_PRECONDITION`, reason `AUTO_JOIN_NEEDS_REQUIRED_AND_DOMAINS`.
5. `REQUIRED` needs at least one active binding in the new set; otherwise `FAILED_PRECONDITION`, reason `REQUIRED_NEEDS_ACTIVE_BINDING`.
6. No connection of the new set is bound by another tenant; otherwise `FAILED_PRECONDITION` with the message "a connection is bound by another tenant" and no reason.

#### Scenario: Company sign-in made optional
- **WHEN** the SSO service writes `OPTIONAL`, auto-join false and one active binding for a tenant with stored domains
- **THEN** enforcement and the binding are stored, the domains are as before, and the response carries the whole policy

#### Scenario: Required without an active binding
- **WHEN** the write asks for `REQUIRED` and none of its bindings is active
- **THEN** it fails with `FAILED_PRECONDITION`, reason `REQUIRED_NEEDS_ACTIVE_BINDING`, and the stored policy is unchanged

#### Scenario: Auto-join without domains or without required
- **WHEN** the write asks for auto-join and the tenant stores no domain, or the write's enforcement is `OPTIONAL`
- **THEN** it fails with `FAILED_PRECONDITION`, reason `AUTO_JOIN_NEEDS_REQUIRED_AND_DOMAINS`

#### Scenario: Too many bindings, or one connection twice
- **WHEN** the write carries six bindings, or two bindings with the same connection id
- **THEN** it fails with `INVALID_ARGUMENT`

#### Scenario: A connection another tenant binds
- **WHEN** one of the connections is already bound by another tenant
- **THEN** the write fails with `FAILED_PRECONDITION` and nothing of it is stored, the enforcement included

### Requirement: Writing the domains
The system SHALL let the SSO service replace a tenant's whole set of domains with `SetTenantSSODomains`, leave the stored enforcement, auto-join and bindings unchanged, and return the resulting policy. An empty set clears the domains. Each domain is trimmed, lower-cased and stripped of a trailing dot; duplicates count once; the stored set is sorted. The system SHALL check, in this order:

1. `tenant_id` is a UUID; otherwise `INVALID_ARGUMENT`.
2. The tenant exists (`NOT_FOUND`) and is not a personal tenant (`FAILED_PRECONDITION`, reason `PERSONAL_TENANT`).
3. Every domain is a DNS name of at most 253 characters, made of at least two labels of letters, digits and inner hyphens, whose last label is at least two letters, or an internationalised top-level label in its ASCII form (`xn--…`); and there are at most fifty; otherwise `INVALID_ARGUMENT`.
4. A tenant with auto-join on keeps at least one domain; otherwise `FAILED_PRECONDITION`, reason `AUTO_JOIN_NEEDS_REQUIRED_AND_DOMAINS`.

The same domain MAY be listed by several tenants. The system checks nothing else about a domain.

#### Scenario: Domains normalised
- **WHEN** the domains written are ` Hooli.EXAMPLE`, `acme.example.` and `hooli.example`
- **THEN** the tenant stores `acme.example` and `hooli.example`

#### Scenario: Not a domain
- **WHEN** one entry is an address, a URL, a single label, a name with a non-ASCII letter or a name whose last label is numeric
- **THEN** the write fails with `INVALID_ARGUMENT` and the stored domains are unchanged

#### Scenario: Clearing the domains of an auto-joining tenant
- **WHEN** the tenant has auto-join on and the write carries no domain
- **THEN** it fails with `FAILED_PRECONDITION`, reason `AUTO_JOIN_NEEDS_REQUIRED_AND_DOMAINS`

#### Scenario: Domains written before any enforcement
- **WHEN** domains are written for a tenant whose enforcement was never written
- **THEN** the domains are stored and the tenant stays off

### Requirement: Removing one binding
The system SHALL let the SSO service remove one binding with `RemoveTenantSSOBinding`, which it calls before it deletes the connection. It SHALL refuse a `tenant_id` or `connection_id` that is not a UUID with `INVALID_ARGUMENT`, an unknown tenant with `NOT_FOUND`, a personal tenant with `FAILED_PRECONDITION` (reason `PERSONAL_TENANT`), and a removal that would leave a `required` tenant with no active binding with `FAILED_PRECONDITION` (reason `REQUIRED_NEEDS_ACTIVE_BINDING`). A connection the tenant does not bind is a success that changes nothing.

#### Scenario: The last active binding of a required tenant
- **WHEN** the tenant stores `required` and the binding is its only active one
- **THEN** the call fails with `FAILED_PRECONDITION`, reason `REQUIRED_NEEDS_ACTIVE_BINDING`, and the binding stays

#### Scenario: The last active binding of an optional tenant
- **WHEN** the tenant stores `optional` and the binding is its only active one
- **THEN** the binding is removed and the tenant is off

#### Scenario: A connection the tenant does not bind
- **WHEN** the policy has no binding to the connection
- **THEN** the call succeeds, nothing is written and no admin action is logged

### Requirement: Policy writes of one tenant are serialized
The system SHALL run each of the three writes in one transaction that first locks the tenant's row, then reads the stored policy in a statement of its own, applies its part, checks the result and writes. The lock is the one an update of the row takes: another write of the same tenant's row waits, while a new membership or invitation of the tenant does not. A write that waits for the lock longer than the lock timeout SHALL fail with `ABORTED`. A write whose transaction could not be started SHALL fail with `INTERNAL` after the statement that locks the row and write nothing: outside a transaction the lock ends with that statement.

#### Scenario: Two parts written at the same time
- **WHEN** one request writes a tenant's domains while another writes `REQUIRED`, auto-join and an active binding for the same tenant
- **THEN** the second waits for the first, checks auto-join against the domains the first one committed, and both changes are stored

#### Scenario: The lock is not released in time
- **WHEN** a write waits for the tenant's row longer than the lock timeout
- **THEN** it fails with `ABORTED`, writes nothing, and the caller may repeat it

#### Scenario: No transaction
- **WHEN** the request's transaction cannot be started and the statement that locks the tenant's row runs on its own
- **THEN** the write fails with `INTERNAL` and nothing is written

#### Scenario: A sign-in during a policy write
- **WHEN** a user joins the tenant while a policy write holds the tenant's row
- **THEN** the membership is written without waiting for the policy write

### Requirement: The database keeps the policy's shape
The database SHALL refuse an enforcement other than `off`, `optional` and `required`; auto-join on a tenant whose enforcement is not `required`; a second binding of one connection, by the same or by another tenant; a domain that is not lower case; and the same domain twice for one tenant. Bindings and domains SHALL be deleted with their tenant. A binding's connection id refers to a row of the SSO service and has no foreign key. The rules that span tables (`required` needs an active binding, auto-join needs a domain, the limits of five and fifty) are checked by the service only.

#### Scenario: A connection bound twice
- **WHEN** a binding is inserted for a connection that another tenant binds
- **THEN** the database refuses it with a unique violation, which the service reports as `FAILED_PRECONDITION`

### Requirement: SSO policy RPCs are for the SSO service only
The system SHALL serve `GetTenantSSOPolicy`, `PutTenantSSOPolicy`, `SetTenantSSODomains` and `RemoveTenantSSOBinding` as the gRPC service `TenantSSOPolicyService`, over gRPC only, with no HTTP route, and authenticate the caller's token as for every other RPC. It SHALL log each write that changed the policy as an admin action (`put_tenant_sso_policy`, `set_tenant_sso_domains`, `remove_tenant_sso_binding`) with the subject of the caller's token as the actor, the tenant id as the resource, and the part it wrote as it was `before` and as it is `after`: the enforcement, auto-join and bindings, or the domains. The actor is therefore the SSO service; which admin asked for the change is for the SSO service to record. An admin-action record is written at log level `info`: at the default `LOG_LEVEL`, `error`, none is written.

#### Scenario: No HTTP route
- **WHEN** the service's HTTP routes are listed
- **THEN** none leads to an SSO policy RPC

#### Scenario: A write that changed the policy
- **WHEN** the SSO service writes `OPTIONAL` and one active binding for a tenant that was off with no bindings
- **THEN** one admin action `put_tenant_sso_policy` is logged with the SSO service as the actor, the tenant id as the resource, and the enforcement, auto-join and bindings before and after

#### Scenario: A refused write
- **WHEN** a write is refused by one of the rules
- **THEN** no admin action is logged
