## 1. Dependencies

- [x] 1.1 Promote `github.com/grpc-ecosystem/go-grpc-prometheus` from indirect to direct dependency in `go.mod`
- [x] 1.2 Run `go mod tidy` to ensure dependency graph is consistent

## 2. Wire Interceptors in Server Startup

- [x] 2.1 Import `grpc_prometheus`, `grpcutil`, and `logging` packages in `cmd/serve.go`
- [x] 2.2 Build read-only method set using `grpcutil.ReadOnlyMethods(v0.TenantService_ServiceDesc)` before `grpc.NewServer`
- [x] 2.3 Replace `grpc.UnaryInterceptor(authMiddleware.GRPCInterceptor)` with `grpc.ChainUnaryInterceptor` containing all four interceptors in order: logging → monitoring → auth → transaction
- [x] 2.4 Add `grpc_prometheus.Register(grpcServer)` after `grpc.NewServer`
- [x] 2.5 Verify `go build ./...` passes after wiring changes

## 3. Unit Tests for Transaction Interceptor

- [x] 3.1 Add test for `TransactionUnaryInterceptor` — mutating method starts transaction and commits on success
- [x] 3.2 Add test for `TransactionUnaryInterceptor` — mutating method rolls back on handler error
- [x] 3.3 Add test for `TransactionUnaryInterceptor` — read-only method bypasses transaction entirely
- [x] 3.4 Add test for `TransactionUnaryInterceptor` — commit failure returns `codes.Internal` while preserving successful handler response if handler succeeded
- [x] 3.5 Add test for `TransactionUnaryInterceptor` — handler error is returned directly (not wrapped) so original gRPC status code is preserved

## 4. Unit Tests for Logging Interceptor

- [x] 4.1 Add test for `LoggingUnaryInterceptor` — successful call logs method, OK status, and duration
- [x] 4.2 Add test for `LoggingUnaryInterceptor` — failed call logs method, non-OK status, and duration
- [x] 4.3 Add test for `LoggingUnaryInterceptor` — logger receives Debugf call with expected format string

## 5. Integration Verification

- [x] 5.1 Run `make test` and verify all existing tests pass with no regressions
- [x] 5.2 Run `go vet ./...` and resolve any warnings
- [x] 5.3 Verify `grpc_server_handling_seconds` metric appears in Prometheus registry output
- [x] 5.4 Confirm read-only methods (ListTenants, LookupTenants) do not emit transaction spans in traces
