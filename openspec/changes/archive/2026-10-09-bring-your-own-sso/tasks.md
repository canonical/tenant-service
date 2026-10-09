## 1. API Contract and Dependencies

- [x] 1.1 In `identity-platform-api`, `proto/v0/tenant/model.proto` and `proto/v0/tenant/service.proto`: add the services `TenantSignInService` (`ListSignInTenants`, `GetSignInContext`, `JoinTenant`, `CreatePersonalTenant`) and `TenantSSOPolicyService` (`GetTenantSSOPolicy`, `PutTenantSSOPolicy`, `SetTenantSSODomains`, `RemoveTenantSSOBinding`), neither with an HTTP mapping; add `GetTenantMFAPolicy`, `PutTenantMFAPolicy` and `RemoveTenantUser` to `TenantService` (HTTP mapped, OpenAPI responses declared); document the `"pending"` status of `InviteMemberResponse`; regenerate the Go and OpenAPI artifacts.
- [x] 1.2 Update `go.mod` and `go.sum`: build against the API change through a local `replace` directive (to be replaced by the released version, see `design.md` Rollout), and add `github.com/testcontainers/testcontainers-go/modules/postgres` for the integration tests.
- [x] 1.3 Update `tests/e2e/go.mod` and `tests/e2e/go.sum` to the dependency versions of the root module (testcontainers `v0.44.0`).

## 2. Migrations

- [x] 2.1 Add `migrations/004_add_tenant_sso_policy.sql`: `tenants.sso_enforcement` (`off`, `optional`, `required`; default `off`) and `tenants.sso_auto_join` with the check that auto-join needs `required`; `tenant_sso_bindings` (primary key tenant and connection, connection unique); `tenant_sso_domains` (lower case, primary key tenant and domain, index on domain).
- [x] 2.2 Add `migrations/005_add_tenants_personal_identity_id.sql`: `tenants.personal_identity_id`, unique, null for an organisation.
- [x] 2.3 Add `migrations/006_add_tenants_mfa_requirement.sql`: `tenants.mfa_requirement` (`none`, `required`; default `none`).
- [x] 2.4 Add `migrations/007_add_tenant_invitations.sql`: `tenant_invitations` (lower-case address, `expires_at`, primary key tenant and address, index on address).
- [x] 2.5 Name every CHECK constraint of `migrations/004_add_tenant_sso_policy.sql`, `migrations/006_add_tenants_mfa_requirement.sql` and `migrations/007_add_tenant_invitations.sql` (`<table>_<what>_check`).

## 3. Types

- [x] 3.1 In `internal/types/types.go`: add `PersonalIdentityID` and `MFARequirement` to `Tenant` with `IsPersonal`; add `SignInTenant`, `SSOBinding`, `TenantSSOPolicy` with its derived values (`EffectiveEnforcement`, `RequiresSSO`, `ActiveConnectionIDs`, `AppliesToDomain`, `ListsDomain`, `InvitationAdmitsDomain`, `AutoJoinAdmitsDomain`), `TenantMFAPolicy`, `SignInContext`, `Invitation` and `PersonalTenantName`.
- [x] 3.2 Add tests for the derived policy values to `internal/types/types_test.go`.

## 4. Storage

- [x] 4.1 In `internal/storage/storage.go`: read the new tenant columns in every tenant query; add `CreatePersonalTenant` (insert that does nothing on a conflict) and `GetPersonalTenantByUserID`; order `ListTenantsByUserID` with the personal tenant first; make `AddMember` absorb a duplicate without aborting the transaction; add `DeleteMember`; add the row lock (`FOR NO KEY UPDATE`) to the internal tenant read, which fails with `ErrNoTransaction` outside a transaction.
- [x] 4.2 Add `internal/storage/sso.go`: `GetTenantSSOPolicy` (one joined statement), `LockTenantSSOPolicy`, `UpdateTenantSSOPolicy`, `UpdateTenantSSODomains`, `DeleteTenantSSOBinding` and `ListAutoJoinCandidatesByDomain`.
- [x] 4.3 Add `internal/storage/invitations.go`: `AddInvitation` (renews an existing one), `HasInvitationByTenantAndEmail`, `DeleteInvitation` and `ListInvitedTenantsByEmail` (not the invitations a `required` tenant's domains no longer allow), all on the database's clock.
- [x] 4.4 Add `internal/storage/mfa.go`: `UpdateTenantMFARequirement`, which locks the row and returns the value it replaced.
- [x] 4.5 In `internal/storage/errors.go`: add `IsLockTimeout`, `IsStatementTimeout` and `ErrNoTransaction`; extend `internal/storage/interfaces.go` with the new methods.
- [x] 4.6 Add `internal/testhelpers/containers.go`, `internal/testhelpers/shared.go` and `internal/testhelpers/migrations.go`: a PostgreSQL container shared by a test binary, with a migrated database per test.
- [x] 4.7 Add `internal/storage/storage_integration_test.go`: migrations down and up, personal tenants and list order, the SSO policy and its parts, two concurrent policy writers, the policy's lock outside a transaction, the schema constraints, the MFA policy value, members, pending invitations (expiry, renewal, lower case), the timeouts the server reports on a pool connection, and the lock and statement timeouts against PostgreSQL.

## 5. Database Client, Kratos Client and Configuration

- [x] 5.1 In `internal/db/storage.go`: send `lock_timeout`, `statement_timeout` and `idle_in_transaction_session_timeout` with every connection of the pool; add `InTx`, which reports whether a context's statements run in a transaction.
- [x] 5.2 In `internal/db/grpc_interceptor.go` and `internal/db/grpc_interceptor_test.go`: answer a failed commit with a fixed message.
- [x] 5.3 In `internal/kratos/client.go`: bound every call by 5 s and return `ErrNotFound` from `GetIdentity` for an unknown identity.
- [x] 5.4 In `internal/config/specs.go`: parse `INVITATION_LIFETIME` as a duration.

## 6. Tenant Service Layer

- [x] 6.1 Add `pkg/tenant/errors.go` (the service error with its gRPC code and reason, the refusals, and the mapping of database timeouts to `ABORTED` and `UNAVAILABLE`) and `pkg/tenant/errors_test.go`.
- [x] 6.2 Add `pkg/tenant/validation.go` (binding limit and duplicates, domain normalisation, syntax and limit, the domain of an address) and `pkg/tenant/validation_test.go`.
- [x] 6.3 Add `pkg/tenant/service_sso.go`: `GetTenantSSOPolicy`, `PutTenantSSOPolicy`, `SetTenantSSODomains`, `RemoveTenantSSOBinding` (each write under the tenant's row lock, the policy's rules in order), `ListSignInTenants` and `GetSignInContext`.
- [x] 6.4 Add `pkg/tenant/service_mfa.go`: `GetTenantMFAPolicy` and `PutTenantMFAPolicy`; in it and in `pkg/tenant/service_sso.go`, put the values before and after each policy write into its admin-action record.
- [x] 6.5 In `pkg/tenant/service.go`: rework `InviteMember` (the four outcomes, Kratos before the first statement, account and recovery link before the first write); add `CreatePersonalTenant` (the address given by the caller, or read from Kratos), `JoinTenant` (the account read from Kratos by id, then the admission check for its address), and `RemoveTenantUser`; add a membership in one function for the three RPCs that add one; make `ProvisionUser` refuse a personal tenant and a member and spend the invitation; answer `NOT_FOUND` for an unknown tenant in `UpdateTenant`.
- [x] 6.6 Add `pkg/tenant/helpers.go` (memberships as sign-in tenants, merging candidates, the address of an identity, a policy as an admin-action label) and extend `pkg/tenant/interfaces.go` with the new service, storage and Kratos methods.
- [x] 6.7 Update and add unit tests in `pkg/tenant/service_test.go`, `pkg/tenant/service_sso_test.go` and `pkg/tenant/service_mfa_test.go`, including the order of the Kratos call and the first statement, and the admin-action record of a policy write.

## 7. Tenant Handlers

- [x] 7.1 In `pkg/tenant/handlers.go`: map errors through one function; add `RemoveTenantUser`; answer `"pending"` from `InviteMember`.
- [x] 7.2 Add `pkg/tenant/handlers_signin.go` (`SignInHandler`: the four RPCs of `TenantSignInService`), `pkg/tenant/handlers_sso.go` (`SSOPolicyHandler`: the four of `TenantSSOPolicyService`) and `pkg/tenant/handlers_mfa.go` (the two MFA policy RPCs).
- [x] 7.3 Add `pkg/tenant/converters.go`: conversions between the service types and the API messages.
- [x] 7.4 Update and add unit tests in `pkg/tenant/handlers_test.go`, `pkg/tenant/handlers_signin_test.go`, `pkg/tenant/handlers_sso_test.go` and `pkg/tenant/handlers_mfa_test.go`: validation, status codes, reasons.

## 8. Webhooks and Router

- [x] 8.1 In `pkg/webhooks/service.go` and `pkg/webhooks/interfaces.go`: make the registration webhook call the personal-tenant creation of `pkg/tenant` with the address of its request and treat `HAS_TENANT` as success; drop the webhook's own tenant and membership writes and its publisher.
- [x] 8.2 Update `pkg/webhooks/service_test.go` for the new registration outcomes.
- [x] 8.3 In `pkg/web/router.go`: pass the tenant service to the webhooks instead of the publisher.

## 9. Command and HTTP Client

- [x] 9.1 In `cmd/serve.go`: refuse a non-positive `INVITATION_LIFETIME`; register `TenantSignInService` and `TenantSSOPolicyService` on the gRPC server, behind the same interceptors as `TenantService` and not on the gateway; name `ListSignInTenants`, `GetSignInContext` and `GetTenantSSOPolicy` as read-only for the transaction interceptor; wire the tenant service into the router.
- [x] 9.2 Add `cmd/serve_test.go`: every RPC of the three services is either read-only or runs in a transaction.
- [x] 9.3 Regenerate `client/http/client.gen.go` from the API's OpenAPI document.
- [x] 9.4 In `cmd/client_http.go`: add `RemoveTenantUser`, `GetTenantMFAPolicy` and `PutTenantMFAPolicy`.
- [x] 9.5 In `cmd/migrate.go`: open the migrate connection with `lock_timeout` 3 s.

## 10. Documentation and Tooling

- [x] 10.1 Update `README.md`: the two invitation outcomes, the positive `INVITATION_LIFETIME`, and the log level admin-action records are written at.
- [x] 10.2 Update `docs/SEQUENCE_DIAGRAMS.md`: registration creating an enabled personal tenant (or nothing, for an account with a tenant or a pending invitation), the invitation outcomes, accepting a pending invitation through `JoinTenant`, and invite compared with provision.
- [x] 10.3 Add the `test-unit` target to `Makefile` (mocks, vet, `go test ./... -short`).

## 11. Verification

- [x] 11.1 Run `make test-unit`: `go vet ./...` and `go test ./... -short` pass in every package.
- [x] 11.2 Run `make test`: the integration tests of `internal/storage` pass against PostgreSQL in a container.
- [x] 11.3 Run `go vet ./...` in `tests/e2e`: the e2e module builds against the changed service.
- [x] 11.4 Run `openspec validate bring-your-own-sso --strict`.
