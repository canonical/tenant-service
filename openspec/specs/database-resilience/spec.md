# database-resilience Specification

## Purpose

This change adds calls that every sign-in makes. It also adds writes that lock the row of a tenant. A request can wait for a lock, for a slow statement or for Kratos. If it waits without limit, it holds a database connection for as long as it waits. It sometimes holds a lock too. The sign-ins in the queue behind it then wait too.

This spec limits every such wait. It also says what the service tells the caller when a limit is reached.

Key decisions:

- The database timeouts are constants that the service sends with every connection. We turned down settings. The values depend on each other and on the Kratos timeout, and a setting can be zero, which brings back the wait without limit.
- The service reports a database timeout with a status code that means "try again" (`ABORTED`, `UNAVAILABLE`). We turned down `INTERNAL` for every failure, because then a caller cannot tell a busy service from a broken one.
- The service asks Kratos before the first statement of a request. The transaction of a request starts at its first statement. So no transaction is open in the database while the service waits for Kratos.
- A migration waits at most 3 s for a table lock. We turned down a wait without limit, because every read of the table queues behind a migration that waits for its lock.
- An unexpected error never carries its text to the caller.

**Non-goals:**

- Retries inside the service. The callers retry.
- A circuit breaker.
- A setting for any of the timeouts.
- A change to the 60-second limit of a transaction.
- A limit on the publishing of permission events. This stays asynchronous.

## Requirements
### Requirement: Every database connection carries the same timeouts
The system SHALL open every connection of its pool with `lock_timeout` 2 s, `statement_timeout` 5 s and `idle_in_transaction_session_timeout` 15 s. The `migrate` command SHALL open its connection with `lock_timeout` 3 s. The values are fixed in the service. An operator cannot configure them.

#### Scenario: What the server reports
- **WHEN** a connection of the pool runs `SHOW lock_timeout`, `SHOW statement_timeout` and `SHOW idle_in_transaction_session_timeout`
- **THEN** the answers are `2s`, `5s` and `15s`

#### Scenario: A migration cannot lock its table
- **WHEN** `migrate up` runs a migration that alters a table, and another transaction keeps the table locked for longer than 3 s
- **THEN** the command fails and the migration is not applied
- **AND** if the command runs again after the lock is free, it applies the migration

### Requirement: Failures and refusals tell the caller what to do
The system SHALL report the outcome of `InviteMember`, `UpdateTenant`, `ProvisionUser`, `RemoveTenantUser`, `LookupTenants`, the MFA policy RPCs and every RPC of `TenantSignInService` and `TenantSSOPolicyService` as follows:

- A rule refuses the request: the system answers with the status code of the rule. If the rule has a reason, the reason is the prefix of the message (`<REASON>: <message>`). The system also attaches the reason as a `google.rpc.ErrorInfo` detail with the domain `tenant-service`. Over HTTP only the message carries the reason. The reasons are `PERSONAL_TENANT`, `HAS_TENANT`, `NOT_ADMITTED`, `DOMAIN_NOT_ALLOWED`, `REQUIRED_NEEDS_ACTIVE_BINDING` and `AUTO_JOIN_NEEDS_REQUIRED_AND_DOMAINS`.
- The database did not grant a lock within the lock timeout: `ABORTED`.
- The statement timeout cancelled a statement: `UNAVAILABLE`.
- Any other failure: `INTERNAL` with a fixed message. The message names the operation and nothing else. A failed Kratos call and a Kratos call that timed out are also in this group. The system writes the error itself to the log.

The other RPCs report failures as before. These RPCs are `ListMyTenants`, `ListTenants`, `ListUserTenants`, `ListTenantUsers`, `CreateTenant` and `DeleteTenant`.

For every RPC that runs in a transaction, a commit that fails SHALL answer `INTERNAL` with the fixed message `transaction failed`. The system writes the error itself to the log.

#### Scenario: A refusal with a reason
- **WHEN** `JoinTenant` refuses an address that is not admitted
- **THEN** the status is `FAILED_PRECONDITION`
- **AND** the message starts with `NOT_ADMITTED: `
- **AND** the status has an `ErrorInfo` detail with reason `NOT_ADMITTED` and domain `tenant-service`

#### Scenario: A policy write that cannot get the lock
- **WHEN** `PutTenantSSOPolicy` waits for the tenant's row longer than the lock timeout
- **THEN** the call fails with `ABORTED`
- **AND** the message says to try again

#### Scenario: A lookup that takes too long
- **WHEN** the statement timeout cancels a statement of `LookupTenants`
- **THEN** the lookup fails with `UNAVAILABLE`

#### Scenario: The commit fails
- **WHEN** the handler of a writing RPC succeeds and the system cannot commit its transaction
- **THEN** the status is `INTERNAL` with the message `transaction failed`
- **AND** the message does not contain the error text of the database

#### Scenario: An unexpected error
- **WHEN** a database error that is not a timeout reaches a handler
- **THEN** the status is `INTERNAL`
- **AND** the message contains no error text and no name of a table or host

### Requirement: Kratos is called with a deadline and outside the database
The system SHALL limit every call to Kratos to 5 s. In `InviteMember`, `ProvisionUser`, `JoinTenant` and `CreatePersonalTenant`, the system SHALL ask Kratos for the account before the first statement of the request. So no transaction is open while the system waits.

Sometimes the system must create an account, for an invitation or for the provisioning of a new address. Then the system SHALL create the account after the reads of the request and before its first write. It SHALL create the recovery link of an invitation at the same point. The transaction is then open and holds no row lock. Two Kratos calls stay under the idle-in-transaction limit.

`ListSignInTenants`, `GetSignInContext`, `GetTenantSSOPolicy` and `LookupTenants` run without a transaction. The SSO policy writes, the MFA policy write, `RemoveTenantUser` and the registration webhook do not call Kratos.

#### Scenario: Kratos does not answer during a join
- **WHEN** `JoinTenant` is called and Kratos does not answer within 5 s
- **THEN** the call fails with `INTERNAL`
- **AND** the system ran no statement and opened no transaction

#### Scenario: Inviting a new address
- **WHEN** a new address is invited to a tenant that does not require company sign-in
- **THEN** the system creates the account and its recovery link in Kratos before it writes the membership
- **AND** no row is locked while Kratos answers

#### Scenario: A sign-in context while Kratos is slow
- **WHEN** `GetSignInContext` waits for Kratos
- **THEN** the call holds no transaction
- **AND** the call does not delay a policy write of the same tenant

