## Purpose

This change adds calls that every sign-in makes and writes that lock a tenant's row. A request that waits without limit for a lock, for a slow statement or for Kratos would hold a database connection, and sometimes a lock, for as long as it waits, and the sign-ins queued behind it would wait too. This spec bounds every such wait and says what the caller is told when a bound is reached.

Key decisions:

- The database timeouts are constants sent with every connection. Settings were turned down: the values depend on each other and on the Kratos timeout, and a setting can be set to zero, which brings the unbounded wait back.
- A database timeout is reported with a status code that says "try again" (`ABORTED`, `UNAVAILABLE`). `INTERNAL` for every failure was turned down: a caller could not tell a busy service from a broken one.
- Kratos is asked before a request's first statement. A request's transaction begins at its first statement, so nothing is open in the database while the service waits for Kratos.
- A migration waits for a table lock for 3 s at most. Waiting without limit was turned down: every read of the table queues behind a migration that waits for its lock.
- An unexpected error never carries its text to the caller.

**Non-goals:** retries inside the service (callers retry); a circuit breaker; a setting for any of the timeouts; changing the 60-second limit of a transaction; a bound on publishing permission events, which stays asynchronous.

## ADDED Requirements

### Requirement: Every database connection carries the same timeouts
The system SHALL open every connection of its pool with `lock_timeout` 2 s, `statement_timeout` 5 s and `idle_in_transaction_session_timeout` 15 s. The `migrate` command SHALL open its connection with `lock_timeout` 3 s. The values are fixed in the service and cannot be configured.

#### Scenario: What the server reports
- **WHEN** a connection of the pool runs `SHOW lock_timeout`, `SHOW statement_timeout` and `SHOW idle_in_transaction_session_timeout`
- **THEN** the answers are `2s`, `5s` and `15s`

#### Scenario: A migration cannot lock its table
- **WHEN** `migrate up` runs a migration that alters a table another transaction keeps locked for longer than 3 s
- **THEN** the command fails, the migration is not applied, and running it again applies it once the lock is free

### Requirement: Failures and refusals tell the caller what to do
The system SHALL report the outcome of `InviteMember`, `UpdateTenant`, `ProvisionUser`, `RemoveTenantUser`, `LookupTenants`, the MFA policy RPCs and every RPC of `TenantSignInService` and `TenantSSOPolicyService` as follows:

- A refusal by a rule: the rule's status code. When the rule has a reason (`PERSONAL_TENANT`, `HAS_TENANT`, `NOT_ADMITTED`, `DOMAIN_NOT_ALLOWED`, `REQUIRED_NEEDS_ACTIVE_BINDING`, `AUTO_JOIN_NEEDS_REQUIRED_AND_DOMAINS`), the reason is the prefix of the message (`<REASON>: <message>`) and is also attached as a `google.rpc.ErrorInfo` detail with the domain `tenant-service`. Over HTTP only the message carries it.
- A lock that was not granted within the lock timeout: `ABORTED`.
- A statement cancelled by the statement timeout: `UNAVAILABLE`.
- Any other failure, a failed or timed-out Kratos call included: `INTERNAL` with a fixed message that names the operation and nothing else. The error itself is written to the log.

The other RPCs (`ListMyTenants`, `ListTenants`, `ListUserTenants`, `ListTenantUsers`, `CreateTenant`, `DeleteTenant`) report failures as before. For every RPC that runs in a transaction, a commit that fails SHALL answer `INTERNAL` with the fixed message `transaction failed`; the error itself is written to the log.

#### Scenario: A refusal with a reason
- **WHEN** `JoinTenant` refuses an address that is not admitted
- **THEN** the status is `FAILED_PRECONDITION`, its message starts with `NOT_ADMITTED: `, and it has an `ErrorInfo` detail with reason `NOT_ADMITTED` and domain `tenant-service`

#### Scenario: A policy write that cannot get the lock
- **WHEN** `PutTenantSSOPolicy` waits for the tenant's row longer than the lock timeout
- **THEN** it fails with `ABORTED` and a message that says to try again

#### Scenario: A lookup that takes too long
- **WHEN** a statement of `LookupTenants` is cancelled by the statement timeout
- **THEN** the lookup fails with `UNAVAILABLE`

#### Scenario: The commit fails
- **WHEN** the handler of a writing RPC succeeds and its transaction cannot be committed
- **THEN** the status is `INTERNAL` with the message `transaction failed`, and the database's error text is not in it

#### Scenario: An unexpected error
- **WHEN** a database error that is not a timeout reaches a handler
- **THEN** the status is `INTERNAL`, and neither the error text nor a table or host name is in the message

### Requirement: Kratos is called with a deadline and outside the database
The system SHALL bound every call to Kratos by 5 s. In `InviteMember`, `ProvisionUser`, `JoinTenant` and `CreatePersonalTenant`, it SHALL ask Kratos for the account before the request's first statement, so that no transaction is open while it waits. Where an account has to be created (an invitation or a provisioning of a new address), it SHALL create it, and the recovery link of an invitation, after the request's reads and before its first write: the transaction is then open and holds no row lock, and two calls stay under the idle-in-transaction limit. `ListSignInTenants`, `GetSignInContext`, `GetTenantSSOPolicy` and `LookupTenants` run without a transaction. The SSO policy writes, the MFA policy write, `RemoveTenantUser` and the registration webhook do not call Kratos.

#### Scenario: Kratos does not answer during a join
- **WHEN** `JoinTenant` is called and Kratos does not answer within 5 s
- **THEN** the call fails with `INTERNAL`, no statement was run and no transaction was opened

#### Scenario: Inviting a new address
- **WHEN** a new address is invited to a tenant that does not require company sign-in
- **THEN** the account and its recovery link are created in Kratos before the membership is written, so that no row is locked while Kratos answers

#### Scenario: A sign-in context while Kratos is slow
- **WHEN** `GetSignInContext` waits for Kratos
- **THEN** it holds no transaction, and a policy write of the same tenant is not delayed by it
