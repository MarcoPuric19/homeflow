# ADR 0001: Go modular monolith

- Status: Accepted
- Date: 2026-08-31

## Decision

Use Go 1.26 and the standard `net/http` stack for a modular monolith plus an
in-process worker boundary.

## Rationale

Go is already installed, produces small static binaries, has predictable
resource usage and supports concurrent HTTP and worker workloads well. The
standard library avoids a web-framework dependency. Modules remain separable
through explicit repository and connector interfaces.

## Consequences

There is one deployable backend for version 1. Module boundaries require code
review discipline, but operational overhead is much lower than microservices.

