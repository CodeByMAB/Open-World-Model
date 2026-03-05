# ADR-001: Go for the Coordinator Service

**Date:** 2026-03-05
**Status:** Accepted
**Deciders:** Core Maintainers

---

## Context

The coordinator is the most network-intensive component: it handles thousands of simultaneous node heartbeats, gRPC streams, task dispatch, and FL round orchestration. It must be horizontally scalable, memory-efficient, and have excellent gRPC library support.

## Decision

Use **Go 1.22+** for `owm-coordinator`.

## Consequences

### Positive
- Native goroutine concurrency maps well to thousands of simultaneous node connections.
- First-class gRPC support via `google.golang.org/grpc` — no friction.
- Low memory overhead per goroutine (~2–8 KB) vs. Python threads (~1 MB).
- Single static binary simplifies deployment and Docker image size.
- Strong standard library for HTTP, TLS, and database access.
- `golangci-lint` provides excellent static analysis in CI.

### Negative
- Separate language from the node daemon (Python) — contributors must know both.
- Slightly more verbose than Python for non-performance-critical logic.

### Neutral
- Performance-critical sub-paths (e.g., gradient aggregation) remain in Rust via the node daemon; the coordinator delegates heavy compute rather than performing it.

## Alternatives Considered

| Option | Reason Rejected |
|---|---|
| Python (FastAPI) | Insufficient concurrency primitives for 10k+ simultaneous gRPC streams |
| Rust | Steeper onboarding; overkill for I/O-bound coordinator logic |
| Node.js | Weaker typing, less mature gRPC ecosystem |
