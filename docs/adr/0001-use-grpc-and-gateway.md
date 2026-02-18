# 1. Use gRPC and Protocol Buffers as the Interface Definition Language

Date: 2026-02-18

## Status

Accepted

## Context

We need a high-performance, strongly typed interface for internal microservice communication, while also supporting standard HTTP/JSON clients for external access (e.g., web UI, CLI). Maintaining two separate API definitions is error-prone and increases maintenance burden.

## Decision

We will define our API using Protocol Buffers (proto3) and use gRPC for service-to-service communication. To support HTTP/JSON clients, we will use `grpc-gateway` to automatically generate a reverse proxy that translates RESTful HTTP requests into gRPC calls.

## Consequences

### Positive
- Single source of truth for API definition (`.proto` files).
- Strong typing and code generation for both server and client.
- High performance for internal traffic.
- Automatic Swagger/OpenAPI documentation generation.

### Negative
- Requires `protoc` and related plugins in the build toolchain.
- Debugging gRPC calls can be more complex than plain HTTP (requires tools like `grpcurl`).
- HTTP mapping annotations in proto files can become verbose.
