# ADR-002: Progressive Decentralization via Kademlia DHT

**Date:** 2026-03-05
**Status:** Accepted
**Deciders:** Core Maintainers

---

## Context

A fully decentralized P2P system from day one is high-risk: it complicates bootstrapping, makes debugging harder, and slows time-to-first-node. However, a permanently centralized coordinator creates a single point of failure and trust, which conflicts with the project's permissionless ethos.

## Decision

**Phase 1–3:** Use a centralized coordinator with a well-defined gRPC interface for node registration, task dispatch, and FL orchestration.

**Phase 4:** Migrate node discovery and task routing to a **Kademlia DHT** (using `libp2p` or a Go Kademlia implementation). The coordinator becomes one of many routing nodes rather than the sole authority.

The coordinator's gRPC interface is designed from day one to be implementable by any node, making the migration non-breaking.

## Consequences

### Positive
- Fast, debuggable bootstrapping in Phase 1–3.
- Clear migration path; the interface is the contract, not the implementation.
- Reduces single-point-of-failure risk over time.
- Aligns with Bitcoin's own progressive decentralization philosophy.

### Negative
- Phase 1–3 nodes must trust the coordinator for task assignment (mitigated by signed task proofs and verifiable payments).
- Phase 4 migration requires careful protocol versioning.

### Mitigation
- All task assignments are signed by the coordinator; nodes can verify they received real tasks.
- Coordinator source code is public — operators can audit or run their own.
- Treasury and payment logic is always on-chain / Lightning (coordinator cannot steal funds).

## Alternatives Considered

| Option | Reason Rejected |
|---|---|
| Fully P2P from day one | Too complex to bootstrap; debugging distributed consensus is hard without established node base |
| Permanent centralized coordinator | Violates permissionless ethos; single point of failure/censorship |
