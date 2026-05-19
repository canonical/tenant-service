## Purpose

The tenant-service's HTTP path logs every request (method, status, duration) via
`middleware.RequestLogger`. Direct gRPC calls produce no equivalent per-request log entries,
making gRPC traffic invisible during debugging and incident investigation.

This spec defines per-RPC request logging for the gRPC server using a lightweight custom
interceptor, achieving parity with the HTTP `LogFormatter` behaviour.

---

## ADDED Requirements

### Requirement: gRPC calls are logged with severity-aware log levels
The system SHALL log every gRPC unary and streaming RPC call with the full method name, gRPC status code, and duration. The log level SHALL reflect the severity of the outcome: server-side error codes (`Internal`, `Unknown`, `Unavailable`, `DataLoss`, `DeadlineExceeded`) SHALL be logged at **Error** level; all other outcomes (including client errors such as `NotFound` or `InvalidArgument`) SHALL be logged at **Debug** level. The log entry SHALL be emitted after handler completion.

#### Scenario: Successful gRPC call is logged at Debug
- **WHEN** a gRPC client calls any unary RPC and the handler succeeds
- **THEN** a Debug log entry is emitted containing the full method name, status code `OK`, and elapsed duration

#### Scenario: Client error is logged at Debug
- **WHEN** a gRPC client calls any unary RPC and the handler returns a client-error code (e.g., `NotFound`, `InvalidArgument`)
- **THEN** a Debug log entry is emitted containing the full method name, the non-OK status code, and elapsed duration
- **AND** the error is preserved for return to the client

#### Scenario: Server error is logged at Error
- **WHEN** a gRPC handler returns a server-side error code (`Internal`, `Unknown`, `Unavailable`, `DataLoss`, or `DeadlineExceeded`)
- **THEN** an Error log entry is emitted containing the full method name, the status code, and elapsed duration
- **AND** the error is preserved for return to the client

#### Scenario: Streaming RPC is logged on stream completion
- **WHEN** a streaming RPC stream closes (normally or with an error)
- **THEN** a log entry is emitted at the appropriate level (Debug or Error) with the full method name, final status code, and total stream duration

### Requirement: gRPC logging format matches HTTP logging
The system SHALL use a logging format consistent with HTTP request logging, including method name (analogous to HTTP path), status code, and duration, to ensure familiarity for operators reading the logs.

#### Scenario: Log format similarity
- **WHEN** an operator reads logs from both HTTP and gRPC traffic
- **THEN** both log types contain comparable information: request identifier (path or method), response status, and time taken
- **AND** the gRPC log does not contain additional fields that would confuse log parsing pipelines
