## Context

See `proposal.md` for the motivation. Three services take part in a company sign-in:

- The **login UI** decides what a user sees and if a sign-in is accepted.
- The **SSO service** stores the identity-provider connections of each tenant. It also talks to the providers.
- **This service** stores tenants and memberships. With this change it also stores the SSO policy of each tenant, its MFA policy, pending invitations and personal tenants.

This service calls neither of the other two. They call this service over gRPC with their own tokens. This service calls the admin API of Kratos and PostgreSQL. It also publishes permission events.

Three things are already true of the service, and they shape the design:

- It authenticates tokens and authorizes nothing. The gateway authorizes HTTP routes. The service accepts a gRPC call with any token that its authentication accepts.
- The transaction of a request starts at its first statement, not when the request arrives. It runs at READ COMMITTED.
- The service publishes permission events from inside the request. It does this asynchronously, before the transaction commits.

Terms:

- An **account** is a Kratos identity.
- An **address** is an email address. An address can have no account.
- A **company sign-in** is a sign-in through one of the connections of a tenant.
- The **effective enforcement** of a tenant is its stored enforcement if the tenant has an active binding. Otherwise it is off.

## Goals / Non-Goals

**Goals:**
- Keep every rule about the sign-in of a tenant in one place. The login UI and the SSO service then ask. They do not decide.
- Make concurrent admin writes of one policy safe. Do not use a version that travels between services.
- Have one way to accept an invitation and one way to create a personal tenant.
- Limit every wait on PostgreSQL and on Kratos.

**Non-Goals:**
- A feature flag in this service. Rollout says what this means for a deployment.
- Everything that `proposal.md` and the specs list as a non-goal.

## Decisions

### Decision 1: Enforcement and auto-join are columns of `tenants`; bindings and domains are tables
- **Rationale**: The row of the tenant already carries the MFA policy. A policy write locks this row. With the two values on the row, every tenant has a policy from the moment it exists (`off`, no auto-join). So there is no case "no row means off", and no row to create on the first write. Bindings and domains are sets, so they are rows. Each row has a constraint that the database keeps. A connection is bound once. A domain is lower case and listed once for each tenant. A read is one statement that joins the three tables. The service puts the rows together.
- **Alternative considered**: a `tenant_sso_policies` table with one row for each tenant. We turned it down for two reasons. A row that can be missing cannot be locked, so the first two writers of a tenant race. Every reader also needs the case "missing means off". A one-to-one table adds a join and a second lock target. It stores nothing that the row of the tenant cannot store.

### Decision 2: A write changes one part of a policy, under the tenant's row lock; there is no version
- **Rationale**: The parts have different writers. Tenant admins decide enforcement, auto-join and bindings. Platform admins decide the domains. The SSO service removes a binding when a connection is deleted. Each write (`PutTenantSSOPolicy`, `SetTenantSSODomains`, `RemoveTenantSSOBinding`) does these steps: it locks the row of the tenant, reads the stored policy, applies its part, checks the whole policy and commits. Two writes of different parts cannot undo each other. A rule that covers more than one part (auto-join needs domains) is checked against the stored state. The write reads the policy in its own statement after the lock. The reason: under READ COMMITTED, a statement that waited for the lock still sees the bindings and domains as they were when the statement started.
- **Alternative considered**: one `Put` of the whole policy with an `expected_version`. The caller reads the policy, changes its part, writes everything back, and retries on a conflict. We turned it down. It needs a version column, a conflict error and a retry loop in another service. The reason is that the API takes the whole policy, but every caller changes one part.
- **The lock needs its transaction**: The transaction of a request starts at its first statement. If it cannot start, the statement runs on its own. The lock then ends with the statement that took it. So the read that locks fails when no transaction is open, and the write fails with it.
- **Lock mode**: `FOR NO KEY UPDATE`. An `UPDATE` of the row takes this lock anyway. We turned down `FOR UPDATE`. A new membership or invitation takes a key-share lock on the row of the tenant for its foreign key. With `FOR UPDATE` it waits behind the write of an admin.
- **Trade-off**: If two writes change the same part, the last write wins.

### Decision 3: This service owns every rule that needs no connection data
- **Rationale**: These rules are in this service:
  - `required` needs an active binding.
  - Auto-join needs `required` and a domain.
  - The syntax of a domain.
  - At most five bindings and fifty domains.
  - A connection is bound once.
  - A personal tenant has no policy.
  
  Most of these rules relate parts that different RPCs write. So the service must check them on the stored state inside the locked transaction. We keep the other rules with them, so that one service decides. The SSO service checks only what it alone knows. A binding names a connection that the tenant owns. An active binding names a tested connection.
- **Alternative considered**: all checks in the SSO service, the only caller. We turned it down. The SSO service must read the policy first. This brings back the alternative of Decision 2.

### Decision 4: What only services call is in two gRPC services of its own, with no HTTP route
- **Rationale**: The login UI and the SSO service call `TenantSignInService` (`ListSignInTenants`, `GetSignInContext`, `JoinTenant`, `CreatePersonalTenant`). The SSO service calls `TenantSSOPolicyService` (the four SSO policy RPCs). Each of these RPCs is harmful on a route that a browser can reach:
  - The list says which tenants invited an address or auto-join its domain.
  - The sign-in context says if an address has an account and is a member.
  - `JoinTenant` creates a membership because the caller says that the user signed in.
  - `CreatePersonalTenant` creates a tenant for any account that it gets.
  - A policy write on such a route skips the connection checks of the SSO service.
  
  A service with no HTTP mapping is not registered with the gateway. So no gateway rule can open a route to it. `TenantService` keeps what people call. The MFA policy RPCs and `RemoveTenantUser` have routes, because tenant admins call them.
- **Alternative considered**: the same RPCs in `TenantService`, each without an HTTP mapping. We turned it down. With it, one service serves two audiences. What a person can reach depends on each RPC that has no mapping.
- **Alternative considered**: HTTP routes that gateway rules close. We turned it down. Nothing needs the RPCs over HTTP, and someone can open a route that exists by mistake.
- **Alternative considered**: a token for the login UI's calls to `TenantSignInService`. We turned it down. With a token, the login UI needs a client, a token endpoint and the certificate authority of that endpoint. These calls never leave the internal network. On this network the login UI already calls `LookupTenants` and the admin API of the identity provider with no token. The service serves `TenantSignInService` without a token, as it does for `LookupTenants`. `TenantSSOPolicyService` authenticates its caller like every other RPC.
- **Trade-off**: `TenantSignInService` trusts every caller that can reach the gRPC port. Anything on the internal network can ask for the tenants of a sign-in. It can also make `JoinTenant` create a membership that an invitation or auto-join admits.

### Decision 5: An invitation is accepted in one way, by signing in to its tenant
- **Rationale**: The login UI calls `JoinTenant` when it accepts a sign-in to the tenant. This happens after the sign-in met the requirements of the tenant. So nobody becomes a member of a tenant that requires company sign-in unless that person passed the company sign-in of that tenant. There is also one place where an invitation turns into a membership.
- **Alternatives considered**:
  - *The membership at once for an existing account* (the previous behaviour). We turned it down. The account becomes a member of a tenant that it never signed in to, and its user did nothing.
  - *A recovery link for an existing account* (the previous behaviour). We turned it down. The service returns the link to the admin who invites, and the link takes over the account.
  - *Accepting at registration*: the registration webhook turns the pending invitations of a registered address into memberships. We turned it down. It is a second way in with its own conditions (is the address verified?). It also made the personal-tenant call do two jobs.
  - *A join page with a token*. We turned it down. The normal sign-in already covers the invitee. With it, the service returns the token to the admin who invites, so the token proves nothing.
- **Trade-off**: An invitation can expire unused. An invited account is not in the member list until its first sign-in to the tenant.

### Decision 6: The tenants of a sign-in are listed by an RPC of their own
- **Rationale**: `LookupTenants` is unauthenticated and has an HTTP route. Some of its callers cannot complete a join. A login UI without the feature never calls `JoinTenant`. If the lookup lists candidates, that login UI shows tenants that its users cannot enter. `ListSignInTenants` adds the invitations and the auto-join candidates of an address to its memberships. It works over gRPC only, for the login UI on the internal network. `LookupTenants` lists memberships only.
- **Alternative considered**: a field on `LookupTenants` that asks for the candidates. We turned it down. Anyone who reaches the HTTP route of the lookup can set a field. That person can then learn which tenants invited an address or auto-join its domain.
- **What an entry shows**: the id and the name of a tenant, and that it invited the address or auto-joins its domain. It does not show if the address has an account.

### Decision 7: The personal-tenant call refuses an account that has a tenant, and registration is that same call
- **Rationale**: A personal tenant exists so that every account has a tenant to sign in to. Two callers ask for it: Kratos's registration webhook, and the login UI. The login UI uses `CreatePersonalTenant` when an account signs in with no tenant. Neither caller knows about the other. Neither knows about the memberships or the pending invitations of the account. The rule is in the one function that both use. So the two calls can run in any order or at the same time. They end in the same state: the existing personal tenant, a new one, or `HAS_TENANT`. On `HAS_TENANT` the login UI lists the tenants of the account again. The webhook has nothing more to do. A pending invitation counts as a tenant, because the account gets a tenant when it accepts the invitation (Decision 5).
- **Alternatives considered**:
  - *A personal tenant for every account*, which removes the refusal. We did not choose it now. It is still possible later.
  - *The callers decide*. We turned it down, because the webhook has nothing to decide with.
  - *A field on `CreateTenant`*. We turned it down. `CreateTenant` has an HTTP route for platform admins. It takes a name and publishes no permission event. A service asks for a personal tenant. The tenant names itself. This service publishes the event of its owner.
- **Uniqueness**: `tenants.personal_identity_id` is unique. On a conflict the insert does nothing, and the loser reads the row of the winner.

### Decision 8: A personal tenant is created enabled
- **Rationale**: The lookup lists enabled tenants only. The login and token webhooks refuse a disabled tenant. A user whose only tenant is disabled can sign in nowhere.
- **Alternative considered**: disabled until an activation step, as for self-registered tenants. We turned it down. There is no activation step, so those tenants stayed unusable until a platform admin enabled them.

### Decision 9: Kratos is asked before the request's first statement
- **Rationale**: The transaction starts at the first statement. A Kratos call after that statement keeps a transaction open. After the first write, the call also keeps a row locked. This lasts as long as Kratos takes. So the lookup of the account comes first in `InviteMember`, `ProvisionUser`, `JoinTenant` and `CreatePersonalTenant`. The registration webhook makes no Kratos call, because Kratos sends the account's id and address with it. The creation of an account cannot come first, because the tenant and its policy decide if the service creates an account. It comes after the reads and before the first write, when nothing is locked.
- **Alternative considered**: call Kratos where the flow reaches it. This was the previous order: create the account, write the membership, then create the recovery link. We turned it down. The transaction stayed open, with the new membership row in it, while Kratos answered. If the recovery link failed, the service rolled back the membership after it had published its `can_view` event.

### Decision 10: Database timeouts are constants
- **Rationale**: The service sends `lock_timeout` 2 s, `statement_timeout` 5 s and `idle_in_transaction_session_timeout` 15 s with every connection. They depend on each other and on the Kratos timeout of 5 s. Two Kratos calls inside an open transaction stay under 15 s. No deployment has a reason to use other values. A setting can be zero, and this brings back the wait without limit.
- **Alternative considered**: environment settings. We turned them down for the reason above.
- **Status codes**: A lock timeout is `ABORTED` and a statement timeout is `UNAVAILABLE`. So a caller can retry. A Kratos failure stays `INTERNAL`.

### Decision 11: An invitation is a row keyed by tenant and address, and expires by the database's clock
- **Rationale**: The address can have no account, so the row cannot refer to one. The service computes `expires_at` and compares it in SQL (`NOW()`). So replicas with different clocks agree. Every read filters the expired rows. Nothing deletes them on expiry. A new invitation of the same address renews the row.
- **Alternative considered**: a clean-up job. We did not build it. The table holds at most one row for each tenant and address.

## Data and control flow

### A sign-in

This is what the service gets asked, in order, when a user signs in to an application:

1. `ListSignInTenants{email}` from the login UI. Kratos: the account of the address. Database: the enabled tenants of the account, the tenants that invited the address, and the tenants that auto-join its domain.
2. `GetSignInContext{tenant_id, email}` for the tenant that the user picked. Database: the tenant. Kratos: the account. Database: the membership, the SSO policy and the invitation. The login UI shows what the answer allows.
3. The user signs in. For a company sign-in, the SSO service also asks `GetSignInContext` before it lets the address through. Kratos creates the account for a new address during that sign-in. This service does not create it.
4. `JoinTenant{tenant_id, identity_id}` when the user is not a member and the tenant admits the address. Kratos: the account, for its address. Database, in one transaction: tenant, membership, policy and invitation. Then the service inserts the membership and deletes the invitation. Event: `can_view`.
5. `CreatePersonalTenant{identity_id}` when the account has no tenant. On `HAS_TENANT` the login UI repeats step 1.
6. The token webhook checks the active membership, as before.

### An invitation

1. A tenant admin calls `InviteMember`. Kratos: the account of the address. Database: the tenant and its policy. Then one of four outcomes follows (spec `tenant-invitations`): nothing, a pending invitation (an upsert of `tenant_invitations`), or an account with a recovery link and a membership.
2. While the invitation is pending, step 1 of a sign-in lists the tenant with `invited`. Step 2 answers `invitation_admits`.
3. The user signs in to the tenant. Step 4 above makes the membership and deletes the invitation.
4. If the user does not sign in, the invitation expires. The service ignores it from then on.

### A policy write

1. A tenant admin changes bindings or enforcement at the SSO service. The SSO service checks what it knows about the connections and calls `PutTenantSSOPolicy`.
2. `SELECT … FROM tenants WHERE id = $1 FOR NO KEY UPDATE`: the transaction starts and waits at most 2 s for the row. If no transaction is open after it, the write fails.
3. One `SELECT` joins the tenant to its bindings and domains. The result is the stored policy.
4. The checks of the spec, in order.
5. `UPDATE tenants` (enforcement, auto-join), `DELETE` the bindings of the tenant, `INSERT` the new set. A unique violation here means that another tenant binds one of the connections.
6. The service logs the admin action with the part as it was and as it is now. The handler returns, the interceptor commits and the lock is released.

`SetTenantSSODomains` (a change by a platform admin) and `RemoveTenantSSOBinding` (before a connection is deleted) follow the same steps. They use their own part in step 5.

## Failure handling

| What fails | The caller sees | What is left |
|---|---|---|
| Kratos is down or slower than 5 s | `INTERNAL` | Nothing, if the call was the lookup of the account |
| The recovery link cannot be created for a new invitee | `INTERNAL` | An account with no membership. If an admin invites the address again, the service finds the account and stores a pending invitation |
| A database write fails after the service created an account (invitation or provisioning of a new address) | `INTERNAL`, or the timeout codes below | The same account with no membership |
| The database does not grant a row lock within 2 s | `ABORTED` | Nothing. The service rolls back the transaction |
| The transaction of a policy write cannot start | `INTERNAL` | Nothing. The write stops after the read that must lock the row |
| A statement runs longer than 5 s | `UNAVAILABLE` | Nothing |
| The commit fails | `INTERNAL` over gRPC. Over HTTP the service already sent the response, as for every HTTP write of this service | Nothing in the database. The permission event and the admin-action log line of the request are already out |
| The registration webhook fails | 500 | No personal tenant. The next `CreatePersonalTenant` for the account creates it |
| Two personal-tenant calls, or two `JoinTenant` calls, run at the same time | Both succeed | One tenant, one membership, one event |
| A policy changes while the service decides an invitation or a join | The decision is on the policy that the service read | `InviteMember` and `JoinTenant` take no lock on the tenant |

## Risks / Trade-offs

- **[Risk] No feature flag** → Mitigation: deploy with the feature of the login UI on. Rollout lists the differences.
- **[Risk] `ListSignInTenants` tells its caller which tenants invited an address or auto-join its domain** → Mitigation: it is gRPC only (Decision 4). An entry carries the id and the name of the tenant and two flags.
- **[Risk] `JoinTenant` trusts its caller that the user signed in** → Mitigation: it is gRPC only (Decision 4). The service still checks that the tenant admits the address.
- **[Risk] The service admits a removed member of an auto-join tenant again at the next sign-in** → Mitigation: none here. Someone must remove the user at the identity provider.
- **[Risk] A new invitation gives an existing account no recovery link** → Mitigation: the user recovers the account with the normal recovery flow.
- **[Risk] The service logs an admin action and publishes an event before the commit** → Mitigation: none in this change. Publishing inside the request is how the service works today.
- **[Risk] Self-registered tenants are usable at once. The earlier decision to keep them disabled until activation (`docs/adr/0003-tenant-activation.md`) no longer holds for them** → Mitigation: accepted (Decision 8).
- **[Trade-off] Expired invitations stay in the table** → The table holds at most one row for each tenant and address.
- **[Trade-off] The service sets the timeouts as connection start-up parameters** → A connection pooler such as PgBouncer can sit between the service and PostgreSQL. It rejects a connection that sends them, unless someone configures it to pass them on or to ignore them. If it ignores them, someone must set the same values on the database role.
- **[Trade-off] The Down steps of migrations 004–007 drop the policies, the invitations and the mark that makes a tenant personal.**

## Rollout

1. **API.** `go.mod` and `tests/e2e/go.mod` require the commit of the pull request that adds the `v0/tenant` changes to `identity-platform-api`. Before this merges, the team must release those changes there. Then the team must raise the required version to that release in `go.mod` and in `tests/e2e/go.mod`.
2. **Migrations 004–007** only add columns and tables, with no backfill. Three of them lock `tenants` exclusively for a moment. `migrate` waits at most 3 s for a table lock and then fails. It can run again. Every CHECK constraint that the migrations add has a name (`<table>_<what>_check`), so that a later migration can replace it by name. Every existing tenant is off, has the MFA requirement `none` and is not personal. The migrations do not convert a disabled tenant that an earlier registration created. An account whose only tenant is such a tenant gets a personal tenant the first time `CreatePersonalTenant` is called for it. The disabled tenant stays.
3. **Deploy with the feature of the login UI on.** With the feature off, a deployment notices these differences:
   - An invitation of an existing account is a pending invitation. Only a login UI that calls `JoinTenant` can accept it. A new invitation of a member returns no link.
   - A newly registered account has an enabled personal tenant. The lookup lists it, and the login and token webhooks accept it. The exceptions are an account whose address has a pending invitation and an account that is a member of an enabled tenant.
   - A personal tenant refuses `InviteMember`, `ProvisionUser` and `RemoveTenantUser`.
   - `ListMyTenants`, `ListUserTenants` and the lookup put the personal tenant first.
   - For the RPCs named in spec `database-resilience`, a lock wait over 2 s answers `ABORTED`. A statement over 5 s answers `UNAVAILABLE`. A Kratos call over 5 s answers `INTERNAL`. An unexpected error no longer carries its text. A failed commit answers `INTERNAL` with the fixed message `transaction failed`, for every RPC that runs in a transaction.
   - `InviteMember`, `ProvisionUser` and `UpdateTenant` answer `NOT_FOUND` for an unknown tenant. `ProvisionUser` answers `ALREADY_EXISTS` for a member. These answers were `INTERNAL`.
4. **Gateway.** The new routes (`…/mfa-policy`, `DELETE …/users/{user_id}`) are under `/api/v0/tenants/*`. The gateway sends this path to the Authorization Service. The rules for the new routes stay there. They are not part of this change.
5. **Configuration.** `INVITATION_LIFETIME` must be a positive Go duration (`24h`). Otherwise the service does not start. The admin-action records are the only history of a policy change. The service writes them at log level `info`. To keep them, `LOG_LEVEL` must be `info` or `debug`. Its default, `error`, drops them.
6. **Kratos.** Two things about the registration webhook are Kratos configuration, which the deployment owns. This service cannot check either of them.
   - Kratos must call the webhook after it stores the identity. So configure it with `response.ignore: true`, and with neither `can_interrupt` nor `response.parse`. Kratos calls a webhook that can interrupt, or whose response it parses, before it stores the identity. Then the service creates the personal tenant for an identity that does not exist yet and perhaps never exists.
   - The webhook must cancel itself (its Jsonnet body raises `error "cancel"`) for a registration through the `byo-sso` provider. Company sign-ins come through this provider. This service creates a personal tenant for every call whose account has no tenant and no pending invitation. An account that auto-join admits has neither when it registers.
7. **Rollback.** The previous version runs on the migrated schema. The new columns have defaults, and the previous version names the columns that it reads and writes. You do not need to run the Down steps. They lose the data listed above.
