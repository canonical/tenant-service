# Tenant Service - AI Coding Agent Instructions

## Project Overview

This is the Tenant Service for the Identity Platform, providing authorization-aware tenant management.
- **API**: gRPC with HTTP/JSON gateway (defined in `api/proto/v0/tenant.proto`).
- **Authorization**: Fine-grained authorization via OpenFGA (see `internal/authorization`).
- **Database**: PostgreSQL with `pressly/goose` for migrations.
- **Observability**: OpenTelemetry tracing and Prometheus metrics.

## Architecture

### Core Components
- **API Layer**: `api/proto/v0/` defines the contract. `pkg/web/router.go` wires handlers.
- **Service Layer**: Business logic lives in `pkg/<domain>/` (e.g., `pkg/tenant/`).
  - Implements the gRPC server interface.
  - orchestrates storage, authorization, and other dependencies.
- **Storage Layer**: `internal/storage/` handles all database interactions.
  - Uses `Masterminds/squirrel` for query building.
  - **Transaction Middleware**: HTTP requests are automatically wrapped in lazy transactions.
- **Command Layer**: `cmd/` contains entrypoints (`serve`, `migrate`).

### Data Flow
1. **Handler**: gRPC/HTTP handler receives request.
2. **Service**:
   - Checks authorization using `internal/authorization`.
   - Validates business rules.
   - Calls `internal/storage` for data persistence.
3. **Storage**: Executes SQL queries within the context's transaction.

### Dependency Injection
- **Constructor Pattern**: `NewService(dependencies..., tracer, monitor, logger)`.
- **Order**: `tracer`, `monitor`, `logger` are always the last three arguments.
- **Wiring**: All dependencies are initialized and injected in `cmd/serve.go`.

## Development Workflows

### Build & Test
- **Mock Generation**: `make mocks` (using `go.uber.org/mock/mockgen`).
  - Run this after changing any `interfaces.go`.
- **Testing**: `make test`.
  - Use standard library `testing` (no `testify`).
  - Use table-driven tests.
  - Mock dependencies using the generated mocks.
- **Run Locally**: `make dev` (starts full stack via `start.sh`).

### Database Migrations
- Managed via `goose` embedded in `cmd/migrate.go`.
- **Add Migration**: Create SQL files in `migrations/`.
- **Run**: `go run . migrate up`.

## Code Conventions

### File Headers
All files must start with:
```go
// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0
```

### Error Handling
- **Wrap Errors**: Use `%w` only when crossing packaging boundaries or when the caller needs to inspect the error.
- **Opaque Errors**: Use `%v` for internal implementation details to prevent leakage.
- **Sentinel Errors**: Define in `errors.go` (e.g., `ErrNotFound`).
- **Storage Errors**:
  - `internal/db` helper functions: `IsDuplicateKeyError(err)`, `IsForeignKeyViolation(err)`.
  - Map DB errors to domain errors in the Service layer.

### Interfaces & Naming
- **Interface-Driven**: define `interfaces.go` in every package for its dependencies.
- **Receivers**: Explicitly name receivers (e.g., `s *Service`), do not use generic names like `this` or `self`.
- **Context**: First parameter of every function performing I/O.

## Example Patterns

### Service Method (Tracing & Error Handling)
```go
func (s *Service) CreateTenant(ctx context.Context, req *v0.CreateTenantRequest) (*v0.CreateTenantResponse, error) {
    ctx, span := s.tracer.Start(ctx, "tenant.Service.CreateTenant")
    defer span.End()

    // Authorization
    if allowed, err := s.authz.Check(ctx, ...); err != nil {
         return nil, status.Errorf(codes.Internal, "auth check failed: %v", err)
    } else if !allowed {
         return nil, status.Error(codes.PermissionDenied, "permission denied")
    }

    // Storage
    tenant, err := s.storage.CreateTenant(ctx, ...)
    if err != nil {
        return nil, fmt.Errorf("failed to create tenant: %v", err)
    }
    return &v0.CreateTenantResponse{Tenant: tenant}, nil
}
```

### Storage Method (Squirrel & Idempotency)
```go
func (s *Storage) DeleteTenant(ctx context.Context, id string) error {
    _, err := s.db.Statement(ctx).
        Delete("tenants").
        Where(sq.Eq{"id": id}).
        ExecContext(ctx)
    // Delete is idempotent; do not check RowsAffected
    if err != nil {
        return fmt.Errorf("failed to delete tenant: %v", err)
    }
    return nil
}
```
