## 1. API Contract and Dependencies

- [x] 1.1 In `identity-platform-api`, in `proto/v0/tenant/model.proto` and `proto/v0/tenant/service.proto`, add the services `TenantSignInService` (`ListSignInTenants`, `GetSignInContext`, `JoinTenant`, `CreatePersonalTenant`) and `TenantSSOPolicyService` (`GetTenantSSOPolicy`, `PutTenantSSOPolicy`, `SetTenantSSODomains`, `RemoveTenantSSOBinding`), each without an HTTP mapping. Add `GetTenantMFAPolicy`, `PutTenantMFAPolicy` and `RemoveTenantUser` to `TenantService`, each with an HTTP mapping and declared OpenAPI responses. Document the `"pending"` status of `InviteMemberResponse`. Regenerate the Go and OpenAPI artifacts.
- [x] 1.2 Update `go.mod` and `go.sum`. Require the commit of the pull request that adds the API change. The released version will replace this commit (see `design.md` Rollout). Add `github.com/testcontainers/testcontainers-go/modules/postgres` for the integration tests.
- [x] 1.3 Update `tests/e2e/go.mod` and `tests/e2e/go.sum` to the dependency versions of the root module (testcontainers `v0.44.0`).

## 2. Migrations

- [x] 2.1 Add `migrations/004_add_tenant_sso_policy.sql`. It adds `tenants.sso_enforcement` (`off`, `optional`, `required`, with the default `off`) and `tenants.sso_auto_join` with the check that auto-join needs `required`. It adds `tenant_sso_bindings` (primary key tenant and connection, connection unique). It adds `tenant_sso_domains` (lower case, primary key tenant and domain, index on domain).
- [x] 2.2 Add `migrations/005_add_tenants_personal_identity_id.sql`. It adds `tenants.personal_identity_id`, which is unique and is null for an organisation.
- [x] 2.3 Add `migrations/006_add_tenants_mfa_requirement.sql`. It adds `tenants.mfa_requirement` (`none`, `required`, with the default `none`).
- [x] 2.4 Add `migrations/007_add_tenant_invitations.sql`. It adds `tenant_invitations` (lower-case address, `expires_at`, primary key tenant and address, index on address).
- [x] 2.5 Name every CHECK constraint of `migrations/004_add_tenant_sso_policy.sql`, `migrations/006_add_tenants_mfa_requirement.sql` and `migrations/007_add_tenant_invitations.sql` (`<table>_<what>_check`).

## 3. Types

- [x] 3.1 In `internal/types/types.go`, add `PersonalIdentityID` and `MFARequirement` to `Tenant` with `IsPersonal`. Add `SignInTenant`, `SSOBinding`, `TenantSSOPolicy` with its derived values (`EffectiveEnforcement`, `RequiresSSO`, `ActiveConnectionIDs`, `AppliesToDomain`, `ListsDomain`, `InvitationAdmitsDomain`, `AutoJoinAdmitsDomain`), `TenantMFAPolicy`, `SignInContext`, `Invitation` and `PersonalTenantName`.
- [x] 3.2 Add tests for the derived policy values to `internal/types/types_test.go`.

## 4. Storage

- [x] 4.1 In `internal/storage/storage.go`, read the new tenant columns in every tenant query. Add `CreatePersonalTenant` (an insert that does nothing on a conflict) and `GetPersonalTenantByUserID`. Order `ListTenantsByUserID` with the personal tenant first. Make `AddMember` absorb a duplicate and not abort the transaction. Add `DeleteMember`. Add the row lock (`FOR NO KEY UPDATE`) to the internal tenant read. That read fails with `ErrNoTransaction` outside a transaction.
- [x] 4.2 Add `internal/storage/sso.go` with `GetTenantSSOPolicy` (one joined statement), `LockTenantSSOPolicy`, `UpdateTenantSSOPolicy`, `UpdateTenantSSODomains`, `DeleteTenantSSOBinding` and `ListAutoJoinCandidatesByDomain`.
- [x] 4.3 Add `internal/storage/invitations.go` with `AddInvitation` (it renews an existing invitation), `HasInvitationByTenantAndEmail`, `DeleteInvitation` and `ListInvitedTenantsByEmail`. The last method does not list the invitations that the domains of a `required` tenant no longer allow. All these methods use the database's clock.
- [x] 4.4 Add `internal/storage/mfa.go` with `UpdateTenantMFARequirement`, which locks the row and returns the value that it replaced.
- [x] 4.5 In `internal/storage/errors.go`, add `IsLockTimeout`, `IsStatementTimeout` and `ErrNoTransaction`. Extend `internal/storage/interfaces.go` with the new methods.
- [x] 4.6 Add `internal/testhelpers/containers.go`, `internal/testhelpers/shared.go` and `internal/testhelpers/migrations.go`. They give a test binary a shared PostgreSQL container, with a migrated database per test.
- [x] 4.7 Add `internal/storage/storage_integration_test.go`. It tests migrations down and up, personal tenants and list order, and the SSO policy and its parts. It tests 2 concurrent policy writers, the lock of the policy outside a transaction, and the schema constraints. It tests the MFA policy value, members and pending invitations (expiry, renewal, lower case). It tests the timeouts that the server reports on a pool connection. It tests the lock timeout and the statement timeout against PostgreSQL.

## 5. Database Client, Kratos Client and Configuration

- [x] 5.1 In `internal/db/storage.go`, send `lock_timeout`, `statement_timeout` and `idle_in_transaction_session_timeout` with every connection of the pool. Add `InTx`, which reports if the statements of a context run in a transaction.
- [x] 5.2 In `internal/db/grpc_interceptor.go` and `internal/db/grpc_interceptor_test.go`, answer a failed commit with a fixed message.
- [x] 5.3 In `internal/kratos/client.go`, limit every call to 5 s. Return `ErrNotFound` from `GetIdentity` for an unknown identity.
- [x] 5.4 In `internal/config/specs.go`, parse `INVITATION_LIFETIME` as a duration.

## 6. Tenant Service Layer

- [x] 6.1 Add `pkg/tenant/errors.go` and `pkg/tenant/errors_test.go`. The first file has the service error with its gRPC code and reason, and the refusals. It also has the mapping of database timeouts to `ABORTED` and `UNAVAILABLE`.
- [x] 6.2 Add `pkg/tenant/validation.go` and `pkg/tenant/validation_test.go`. The first file has the binding limit and duplicates, and the domain normalisation, syntax and limit. It also has the domain of an address.
- [x] 6.3 Add `pkg/tenant/service_sso.go` with `GetTenantSSOPolicy`, `PutTenantSSOPolicy`, `SetTenantSSODomains`, `RemoveTenantSSOBinding`, `ListSignInTenants` and `GetSignInContext`. Each of the 3 writes runs under the tenant's row lock and checks the rules of the policy in order.
- [x] 6.4 Add `pkg/tenant/service_mfa.go` with `GetTenantMFAPolicy` and `PutTenantMFAPolicy`. In that file and in `pkg/tenant/service_sso.go`, put the values before and after each policy write into its admin-action record.
- [x] 6.5 In `pkg/tenant/service.go`, rework `InviteMember`. It has the 4 outcomes and calls Kratos before the first statement. It creates the account and the recovery link before the first write. Add `CreatePersonalTenant`, which uses the address that the caller gives, or reads the address from Kratos. Add `JoinTenant`, which reads the account from Kratos by id and then does the admission check for its address. Add `RemoveTenantUser`. Add a membership in one function for the 3 RPCs that add one. Make `ProvisionUser` refuse a personal tenant and a member, and spend the invitation. Answer `NOT_FOUND` for an unknown tenant in `UpdateTenant`.
- [x] 6.6 Add `pkg/tenant/helpers.go`. It has memberships as sign-in tenants, the merge of candidates and the address of an identity. It also has a policy as an admin-action label. Extend `pkg/tenant/interfaces.go` with the new service, storage and Kratos methods.
- [x] 6.7 Update and add unit tests in `pkg/tenant/service_test.go`, `pkg/tenant/service_sso_test.go` and `pkg/tenant/service_mfa_test.go`. Include the order of the Kratos call and the first statement, and the admin-action record of a policy write.

## 7. Tenant Handlers

- [x] 7.1 In `pkg/tenant/handlers.go`, map errors through one function. Add `RemoveTenantUser`. Answer `"pending"` from `InviteMember`.
- [x] 7.2 Add `pkg/tenant/handlers_signin.go` with `SignInHandler`, which has the 4 RPCs of `TenantSignInService`. Add `pkg/tenant/handlers_sso.go` with `SSOPolicyHandler`, which has the 4 RPCs of `TenantSSOPolicyService`. Add `pkg/tenant/handlers_mfa.go` with the 2 MFA policy RPCs.
- [x] 7.3 Add `pkg/tenant/converters.go` with the conversions between the service types and the API messages.
- [x] 7.4 Update and add unit tests in `pkg/tenant/handlers_test.go`, `pkg/tenant/handlers_signin_test.go`, `pkg/tenant/handlers_sso_test.go` and `pkg/tenant/handlers_mfa_test.go`. The tests cover validation, status codes and reasons.

## 8. Webhooks and Router

- [x] 8.1 Change `pkg/webhooks/service.go` and `pkg/webhooks/interfaces.go`. Make the registration webhook call the personal-tenant creation of `pkg/tenant` with the address of its request. Make the webhook treat `HAS_TENANT` as success. Remove the webhook's own tenant and membership writes and its publisher.
- [x] 8.2 Update `pkg/webhooks/service_test.go` for the new registration outcomes.
- [x] 8.3 In `pkg/web/router.go`, pass the tenant service to the webhooks instead of the publisher.

## 9. Command and HTTP Client

- [x] 9.1 In `cmd/serve.go`, refuse a non-positive `INVITATION_LIFETIME`. Register `TenantSignInService` and `TenantSSOPolicyService` on the gRPC server, behind the same interceptors as `TenantService`. Do not register them on the gateway. Name `ListSignInTenants`, `GetSignInContext` and `GetTenantSSOPolicy` as read-only for the transaction interceptor. Wire the tenant service into the router.
- [x] 9.2 Add `cmd/serve_test.go`. It tests that every RPC of the 3 services is read-only or runs in a transaction.
- [x] 9.3 Regenerate `client/http/client.gen.go` from the API's OpenAPI document.
- [x] 9.4 In `cmd/client_http.go`, add `RemoveTenantUser`, `GetTenantMFAPolicy` and `PutTenantMFAPolicy`.
- [x] 9.5 In `cmd/migrate.go`, open the migrate connection with `lock_timeout` 3 s.

## 10. Documentation and Tooling

- [x] 10.1 Update `README.md` with the 2 invitation outcomes, the positive `INVITATION_LIFETIME`, and the log level of admin-action records.
- [x] 10.2 Update `docs/SEQUENCE_DIAGRAMS.md`. Show that registration creates an enabled personal tenant, or nothing for an account with a tenant or a pending invitation. Show the invitation outcomes and the acceptance of a pending invitation through `JoinTenant`. Show invite compared with provision.
- [x] 10.3 Add the `test-unit` target to `Makefile` (mocks, vet, `go test ./... -short`).

## 11. Verification

- [x] 11.1 Run `make test-unit`. `go vet ./...` and `go test ./... -short` pass in every package.
- [x] 11.2 Run `make test`. The integration tests of `internal/storage` pass against PostgreSQL in a container.
- [x] 11.3 Run `go vet ./...` in `tests/e2e`. The e2e module builds against the changed service.
- [x] 11.4 Run `openspec validate bring-your-own-sso --strict`.
