# ADR-004: LND as Primary Lightning Implementation

**Date:** 2026-03-05
**Status:** Accepted
**Deciders:** Core Maintainers

---

## Context

OWM requires a production Lightning Network node for three distinct roles:
1. **Payment dispatch** — paying node operators for compute and mining contributions.
2. **Stake verification** — querying channel state to verify PoS requirements.
3. **Slashing** — force-closing channels of misbehaving nodes.

These roles require different permission levels and must be isolated to prevent the payment credential from being used for slashing and vice versa.

## Decision

Use **LND (Lightning Network Daemon)** as the primary Lightning implementation for the treasury node, with:
- `lnd-grpc` Python client for the node daemon and API service.
- Separate **macaroon credentials** per role:
  - `payment.macaroon` — SendPayment, AddInvoice (used by coordinator payment dispatcher)
  - `readonly.macaroon` — ListChannels (used by stake verifier)
  - `slashing.macaroon` — ForceCloseChan only (used exclusively by stake manager; stored separately with HSM access control)
- **CLN (Core Lightning)** supported as an alternative via the CLNREST plugin; the coordinator abstracts over both via an `LNClient` interface.

## Consequences

### Positive
- LND is the most widely deployed Lightning implementation; extensive tooling and documentation.
- Macaroon-based access control enables fine-grained credential separation without custom auth.
- `ForceCloseChan` is a well-tested LND RPC; force-close behavior is deterministic.
- CLN fallback via interface abstraction future-proofs against LND-specific issues.

### Negative
- LND is a large Go binary with significant resource requirements (RAM, disk for channel DB).
- gRPC interface changes between LND versions can break integrations — pin LND version in deployment.

### Credential Isolation Architecture

```
owm-coordinator/
  internal/
    lightning/
      client.go         ← LNClient interface (abstract over LND/CLN)
      lnd_client.go     ← LND implementation
      cln_client.go     ← CLN implementation
      credentials/
        payment/        ← payment.macaroon (SendPayment, AddInvoice)
        readonly/       ← readonly.macaroon (ListChannels, GetInfo)
        slashing/       ← custom-baked macaroon (ForceCloseChan only)
                           stored in HSM / separate secret store
```

## Alternatives Considered

| Option | Reason Rejected |
|---|---|
| CLN (primary) | Smaller community; less tooling; gRPC interface (via plugin) less mature |
| LDK (embedded) | No production-grade treasury node use case; designed for wallets, not servers |
| Eclair | JVM dependency; higher operational complexity |
