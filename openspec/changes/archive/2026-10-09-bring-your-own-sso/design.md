## Context

See `proposal.md` for the motivation. Three services take part in a company sign-in:

- the **login UI** decides what a user is shown and whether a sign-in is accepted;
- the **SSO service** stores each tenant's identity-provider connections and talks to the providers;
- **this service** stores tenants and memberships, and with this change each tenant's SSO policy, its MFA policy, pending invitations and personal tenants.

This service calls neither of the other two. They call it, over gRPC, with their own tokens. It calls Kratos's admin API and PostgreSQL, and publishes permission events.

Three things already true of the service shape the design:

- It authenticates tokens and authorizes nothing. The gateway authorizes HTTP routes; a gRPC call is accepted with any token the service's authentication accepts.
- A request's transaction begins at its first statement, not when the request arrives, and runs at READ COMMITTED.
- Permission events are published from inside the request, asynchronously, before the transaction commits.

Terms: an **account** is a Kratos identity; an **address** is an email address, which may have no account; **company sign-in** is a sign-in through one of a tenant's connections; a tenant's **effective enforcement** is its stored enforcement when it has an active binding and off otherwise.

## Goals / Non-Goals

**Goals:**
- Keep every rule about a tenant's sign-in in one place, so that the login UI and the SSO service ask instead of deciding.
- Make concurrent admin writes of one policy safe without a version that travels between services.
- Have one way to accept an invitation and one way to create a personal tenant.
- Bound every wait on PostgreSQL and on Kratos.

**Non-Goals:**
- A feature flag in this service. What that means for a deployment is under Rollout.
- Everything listed as a non-goal in `proposal.md` and in the specs.

## Decisions

### Decision 1: Enforcement and auto-join are columns of `tenants`; bindings and domains are tables
- **Rationale**: The tenant's row already carries the MFA policy and is the row a policy write locks. With the two values on it, every tenant has a policy from the moment it exists (`off`, no auto-join): there is no "no row means off" case and no row to create on the first write. Bindings and domains are sets, so they are rows, each with a constraint the database keeps (a connection is bound once; a domain is lower case and listed once per tenant). A read is one statement that joins the three tables; the service puts the rows together.
- **Alternative considered**: a `tenant_sso_policies` table, one row per tenant. Turned down: a row that may not exist cannot be locked, so the first two writers of a tenant would race, and every reader would need the "missing means off" case. A one-to-one table adds a join and a second lock target and stores nothing the tenant's row cannot.

### Decision 2: A write changes one part of a policy, under the tenant's row lock; there is no version
- **Rationale**: The parts have different writers. Tenant admins decide enforcement, auto-join and bindings; platform admins decide the domains; the SSO service removes a binding when a connection is deleted. Each write (`PutTenantSSOPolicy`, `SetTenantSSODomains`, `RemoveTenantSSOBinding`) locks the tenant's row, reads the stored policy, applies its part, checks the whole policy and commits. Two writes of different parts cannot undo each other, and a rule that spans parts (auto-join needs domains) is checked against what is stored. The policy is read in a statement of its own after the lock: under READ COMMITTED a statement that waited for the lock would still see the bindings and domains as they were when it started.
- **Alternative considered**: one `Put` of the whole policy with an `expected_version`. The caller reads the policy, changes its part, writes everything back, and retries on a conflict. Turned down: it needs a version column, a conflict error and a retry loop in another service, all because the API would take the whole policy while every caller changes one part.
- **The lock needs its transaction**: a request's transaction begins at its first statement, and when it cannot begin the statement runs on its own. The lock would then end with the statement that took it, so the read that locks fails when no transaction is open, and the write with it.
- **Lock mode**: `FOR NO KEY UPDATE`, the lock an `UPDATE` of the row takes anyway. `FOR UPDATE` was turned down: inserting a membership or an invitation takes a key-share lock on the tenant's row for its foreign key and would wait behind an admin's write.
- **Trade-off**: two writes of the same part are last write wins.

### Decision 3: This service owns every rule that needs no connection data
- **Rationale**: `required` needs an active binding; auto-join needs `required` and a domain; the syntax of a domain; at most five bindings and fifty domains; a connection is bound once; a personal tenant has no policy. Most of them relate parts that different RPCs write, so they have to be checked on the stored state inside the locked transaction; the rest are kept with them so that one service decides. The SSO service checks only what it alone knows: a binding names a connection the tenant owns, and an active one names a tested connection.
- **Alternative considered**: all checks in the SSO service, the only caller. Turned down: it would have to read the policy first, which brings back Decision 2's alternative.

### Decision 4: What only services call is in two gRPC services of its own, with no HTTP route
- **Rationale**: `TenantSignInService` (`ListSignInTenants`, `GetSignInContext`, `JoinTenant`, `CreatePersonalTenant`) is called by the login UI and the SSO service; `TenantSSOPolicyService` (the four SSO policy RPCs) by the SSO service. Each RPC would do harm on a route a browser can reach: the list says which tenants invited an address or auto-join its domain; the sign-in context says whether an address has an account and is a member; `JoinTenant` creates a membership on the caller's word that the user signed in; `CreatePersonalTenant` creates a tenant for whatever account it is given; a policy write would skip the SSO service's connection checks. A service with no HTTP mapping is not registered with the gateway, so no route to it can be opened by a gateway rule. `TenantService` keeps what people call: the MFA policy RPCs and `RemoveTenantUser` have routes, because tenant admins call them.
- **Alternative considered**: the same RPCs in `TenantService`, each without an HTTP mapping. Turned down: one service would serve two audiences, and what a person can reach would depend on each RPC being left without a mapping.
- **Alternative considered**: HTTP routes, closed by gateway rules. Turned down: nothing needs them over HTTP, and a route that exists can be opened by mistake.
- **Trade-off**: for these RPCs the service trusts every caller whose token it accepts and that can reach its gRPC port.

### Decision 5: An invitation is accepted in one way, by signing in to its tenant
- **Rationale**: `JoinTenant` is called by the login UI when it accepts a sign-in to the tenant, after that sign-in met the tenant's requirements. Nobody is therefore a member of a tenant that requires company sign-in without having passed its company sign-in, and there is one place where an invitation turns into a membership.
- **Alternatives considered**:
  - *The membership at once for an existing account* (the previous behaviour). Turned down: the account would be a member of a tenant it never signed in to, without its user doing anything.
  - *A recovery link for an existing account* (the previous behaviour). Turned down: the link is returned to the inviting admin and takes over the account.
  - *Accepting at registration*: the registration webhook turns the pending invitations of a registered address into memberships. Turned down: a second way in with conditions of its own (is the address verified?), and it made the personal-tenant call do two jobs.
  - *A join page with a token*. Turned down: the normal sign-in already covers the invitee, and the token would be returned to the inviting admin, so it proves nothing.
- **Trade-off**: an invitation can expire unused, and an invited account is not in the member list until its first sign-in to the tenant.

### Decision 6: The tenants of a sign-in are listed by an RPC of their own
- **Rationale**: `LookupTenants` is unauthenticated, has an HTTP route, and has callers that cannot complete a join: a login UI without the feature never calls `JoinTenant`, and would show tenants its users cannot enter. `ListSignInTenants` adds the invitations and the auto-join candidates of an address to its memberships, over gRPC only and for an authenticated caller. `LookupTenants` lists memberships only.
- **Alternative considered**: a field on `LookupTenants` that asks for the candidates. Turned down: anyone can set a field of an unauthenticated lookup, and learn which tenants invited an address or auto-join its domain.
- **What an entry reveals**: a tenant's id and name, and that it invited the address or auto-joins its domain; not whether the address has an account.

### Decision 7: The personal-tenant call refuses an account that has a tenant, and registration is that same call
- **Rationale**: A personal tenant exists so that no account is without a tenant to sign in to. Two callers ask for it: Kratos's registration webhook, and the login UI, through `CreatePersonalTenant`, when an account signs in with no tenant. Neither knows about the other, about the account's memberships or about its pending invitations. With the rule in the one function both use, the two can run in any order or at once and end in the same state: the existing personal tenant, a new one, or `HAS_TENANT`. On `HAS_TENANT` the login UI lists the account's tenants again, and the webhook has nothing more to do. A pending invitation counts as having a tenant because accepting it (Decision 5) is how that account gets one.
- **Alternatives considered**: *a personal tenant for every account*, which removes the refusal; not chosen now and still possible later. *The callers decide*: turned down, because the webhook has nothing to decide with. *A field on `CreateTenant`*: turned down, because `CreateTenant` has an HTTP route for platform admins, takes a name and publishes no permission event, while a personal tenant is asked for by a service, names itself and publishes its owner's.
- **Uniqueness**: `tenants.personal_identity_id` is unique; the insert does nothing on a conflict and the loser reads the winner's row.

### Decision 8: A personal tenant is created enabled
- **Rationale**: The lookup lists enabled tenants only, and the login and token webhooks refuse a disabled one. A user whose only tenant is disabled can sign in nowhere.
- **Alternative considered**: disabled until an activation step, as self-registered tenants were. Turned down: there is no activation step, so those tenants stayed unusable until a platform admin enabled them.

### Decision 9: Kratos is asked before the request's first statement
- **Rationale**: The transaction begins at the first statement. A Kratos call after it keeps a transaction open, and after the first write a row locked, for as long as Kratos takes. So the account lookup comes first in `InviteMember`, `ProvisionUser`, `JoinTenant` and `CreatePersonalTenant`. The registration webhook makes no Kratos call: Kratos sends the account's id and address with it. Creating an account cannot come first, because whether one is created depends on the tenant and its policy; it comes after the reads and before the first write, when nothing is locked.
- **Alternative considered**: call Kratos where the flow reaches it (the previous order: create the account, write the membership, then create the recovery link). Turned down: the transaction stayed open, with the new membership row in it, while Kratos answered; and when the recovery link failed, the membership was rolled back after its `can_view` event had been published.

### Decision 10: Database timeouts are constants
- **Rationale**: `lock_timeout` 2 s, `statement_timeout` 5 s and `idle_in_transaction_session_timeout` 15 s are sent with every connection. They depend on each other and on the 5 s Kratos timeout (two Kratos calls inside an open transaction stay under 15 s). No deployment has a reason to differ, and a setting could be set to zero, which brings the unbounded wait back.
- **Alternative considered**: environment settings. Turned down for the reason above.
- **Status codes**: a lock timeout is `ABORTED` and a statement timeout is `UNAVAILABLE`, so that a caller can retry. A Kratos failure stays `INTERNAL`.

### Decision 11: An invitation is a row keyed by tenant and address, and expires by the database's clock
- **Rationale**: The address may have no account, so the row cannot refer to one. `expires_at` is computed and compared in SQL (`NOW()`), so replicas whose clocks differ agree. Expired rows are filtered by every read; nothing deletes them on expiry, and a new invitation of the same address renews the row.
- **Alternative considered**: a clean-up job. Not built: the table holds at most one row per tenant and address.

## Data and control flow

### A sign-in

What this service is asked, in order, when a user signs in to an application:

1. `ListSignInTenants{email}` from the login UI. Kratos: the account of the address. Database: the account's enabled tenants, the tenants that invited the address, the tenants that auto-join its domain.
2. `GetSignInContext{tenant_id, email}` for the tenant the user picked. Database: the tenant. Kratos: the account. Database: the membership, the SSO policy, the invitation. The login UI shows what the answer allows.
3. The user signs in. For a company sign-in the SSO service asks `GetSignInContext` too before it lets the address through. An account for a new address is created by Kratos during that sign-in, not by this service.
4. `JoinTenant{tenant_id, identity_id}` when the user is not a member and the tenant admits the address. Kratos: the account, for its address. Database, in one transaction: tenant, membership, policy, invitation; insert the membership; delete the invitation. Event: `can_view`.
5. `CreatePersonalTenant{identity_id}` when the account has no tenant at all. On `HAS_TENANT` the login UI repeats step 1.
6. The token webhook checks the active membership, as before.

### An invitation

1. A tenant admin calls `InviteMember`. Kratos: the account of the address. Database: the tenant and its policy. Then one of four outcomes (spec `tenant-invitations`): nothing, a pending invitation (an upsert of `tenant_invitations`), or an account with a recovery link and a membership.
2. While the invitation is pending, step 1 of a sign-in lists the tenant with `invited`, and step 2 answers `invitation_admits`.
3. The user signs in to the tenant; step 4 above makes the membership and deletes the invitation.
4. Otherwise the invitation expires and is ignored from then on.

### A policy write

1. A tenant admin changes bindings or enforcement at the SSO service, which checks what it knows about the connections and calls `PutTenantSSOPolicy`.
2. `SELECT … FROM tenants WHERE id = $1 FOR NO KEY UPDATE`: the transaction begins and waits at most 2 s for the row. If no transaction is open after it, the write fails.
3. One `SELECT` joins the tenant to its bindings and domains: the stored policy.
4. The checks of the spec, in order.
5. `UPDATE tenants` (enforcement, auto-join), `DELETE` the tenant's bindings, `INSERT` the new set. A unique violation here means another tenant binds one of the connections.
6. The admin action is logged with the part as it was and as it is now, the handler returns, the interceptor commits and the lock is released.

`SetTenantSSODomains` (a platform admin's change) and `RemoveTenantSSOBinding` (before a connection is deleted) follow the same steps with their own part in step 5.

## Failure handling

| What fails | The caller sees | What is left |
|---|---|---|
| Kratos is down or slower than 5 s | `INTERNAL` | Nothing, when the call was the account lookup |
| The recovery link cannot be created for a new invitee | `INTERNAL` | An account with no membership. Inviting the address again finds the account and stores a pending invitation |
| A database write fails after an account was created (invitation or provisioning of a new address) | `INTERNAL`, or the timeout codes below | The same account with no membership |
| A row lock is not granted within 2 s | `ABORTED` | Nothing: the transaction is rolled back |
| The transaction of a policy write cannot begin | `INTERNAL` | Nothing: the write stops after the read that should have locked the row |
| A statement runs longer than 5 s | `UNAVAILABLE` | Nothing |
| The commit fails | `INTERNAL` over gRPC; over HTTP the response was already sent, as for every HTTP write of this service | Nothing in the database. A permission event and an admin-action log line of the request are already out |
| The registration webhook fails | 500 | No personal tenant. The next `CreatePersonalTenant` for the account creates it |
| Two personal-tenant calls, or two `JoinTenant` calls, at once | Both succeed | One tenant, one membership, one event |
| A policy changes while an invitation or a join is being decided | The decision made on the policy that was read | `InviteMember` and `JoinTenant` take no lock on the tenant |

## Risks / Trade-offs

- **[Risk] No feature flag** → Mitigation: deploy with the login UI's feature on; the differences are listed under Rollout.
- **[Risk] `ListSignInTenants` tells its caller which tenants invited an address or auto-join its domain** → Mitigation: gRPC only and authenticated (Decision 4); an entry carries the tenant's id and name and two flags.
- **[Risk] `JoinTenant` trusts its caller that the user signed in** → Mitigation: gRPC only (Decision 4); the service still checks that the tenant admits the address.
- **[Risk] A removed member of an auto-join tenant is admitted again at the next sign-in** → Mitigation: none here; the user has to be removed at the identity provider.
- **[Risk] Inviting again no longer gives an existing account a new recovery link** → Mitigation: the user recovers the account through the normal recovery flow.
- **[Risk] An admin action is logged, and an event published, before the commit** → Mitigation: none in this change; publishing inside the request is how the service works today.
- **[Risk] Self-registered tenants are usable at once; the earlier decision to keep them disabled until activation (`docs/adr/0003-tenant-activation.md`) no longer holds for them** → Mitigation: accepted (Decision 8).
- **[Trade-off] Expired invitations stay in the table** → at most one row per tenant and address.
- **[Trade-off] The timeouts are set as connection start-up parameters** → a connection pooler between the service and PostgreSQL, such as PgBouncer, rejects a connection that sends them unless it is configured to pass them on or to ignore them. Where it ignores them, the same values have to be set on the database role.
- **[Trade-off] The Down steps of migrations 004–007 drop the policies, the invitations and the mark that makes a tenant personal.**

## Rollout

1. **API.** The tree builds against a local checkout of `identity-platform-api` through a `replace` directive in `go.mod`. Before this merges, the `v0/tenant` changes have to be released there, the directive removed, and the required version raised to that release in `go.mod` and in `tests/e2e/go.mod`.
2. **Migrations 004–007** only add columns and tables, with no backfill. Three of them lock `tenants` exclusively for a moment; `migrate` waits at most 3 s for a table lock and fails otherwise, and can be run again. Every CHECK constraint they add is named (`<table>_<what>_check`), so that a later migration can replace it by name. Every existing tenant is off, has the MFA requirement `none` and is not personal. A disabled tenant that an earlier registration created is not converted. An account whose only tenant is such a tenant gets a personal tenant the first time `CreatePersonalTenant` is called for it, and the disabled tenant stays.
3. **Deploy with the login UI's feature on.** With it off, this is what a deployment notices:
   - An invitation of an existing account is a pending invitation, and only a login UI that calls `JoinTenant` can accept it. Inviting a member again returns no link.
   - A newly registered account has an enabled personal tenant, which the lookup lists and the login and token webhooks accept, unless its address has a pending invitation or it is a member of an enabled tenant.
   - A personal tenant refuses `InviteMember`, `ProvisionUser` and `RemoveTenantUser`.
   - `ListMyTenants`, `ListUserTenants` and the lookup put the personal tenant first.
   - For the RPCs named in spec `database-resilience`: a lock wait over 2 s answers `ABORTED`, a statement over 5 s `UNAVAILABLE`, a Kratos call over 5 s `INTERNAL`, and an unexpected error no longer carries its text. A failed commit answers `INTERNAL` with the fixed message `transaction failed`, for every RPC that runs in a transaction.
   - `InviteMember`, `ProvisionUser` and `UpdateTenant` answer `NOT_FOUND` for an unknown tenant, and `ProvisionUser` answers `ALREADY_EXISTS` for a member; these were `INTERNAL`.
4. **Gateway.** The new routes (`…/mfa-policy`, `DELETE …/users/{user_id}`) are under `/api/v0/tenants/*`, which the gateway sends to the Authorization Service. Its rules for them are kept there and are not part of this change.
5. **Configuration.** `INVITATION_LIFETIME` has to be a positive Go duration (`24h`); the service does not start otherwise. Admin-action records, which are the only history of a policy change, are written at log level `info`: `LOG_LEVEL` has to be `info` or `debug` to keep them, and its default, `error`, drops them.
6. **Kratos.** Two things about the registration webhook are Kratos configuration, which the deployment owns; this service cannot check either.
   - It has to be called after the identity is stored: configured with `response.ignore: true`, and so with neither `can_interrupt` nor `response.parse`. Kratos calls a webhook that can interrupt, or whose response it parses, before it stores the identity: the personal tenant would be created for an identity that does not exist yet and may never.
   - It has to cancel itself (its Jsonnet body raises `error "cancel"`) for a registration through the `byo-sso` provider, the one company sign-ins come through. This service creates a personal tenant for every call whose account has no tenant and no pending invitation, and an account that auto-join admits has neither when it registers.
7. **Rollback.** The previous version runs on the migrated schema: the new columns have defaults, and it names the columns it reads and writes. Running the Down steps is not needed and loses the data listed above.
