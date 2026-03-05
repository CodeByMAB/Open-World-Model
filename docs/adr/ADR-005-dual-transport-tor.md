# ADR-005: Dual-Transport — Clearnet + Tor Hidden Service

**Date:** 2026-03-05
**Status:** Accepted
**Deciders:** Core Maintainers

---

## Context

OWM node operators are pseudonymous contributors whose IP addresses may expose
their physical location, infrastructure provider, or jurisdiction. A subset of
operators will run in politically sensitive environments where unobfuscated
clearnet connections to the coordinator could trigger surveillance or blocking.

At the same time, OWM's data-plane traffic (FL gradient uploads, model weight
downloads, Stratum v2 mining share submission) is high-bandwidth
and latency-sensitive. Routing bulk data through Tor would impose an effective
throughput ceiling of ~1–3 Mbps and ~100–300 ms of added latency per hop —
making FL round times and mining stale-share rates unacceptably high.

The network therefore needs **two clearly scoped transport stacks**:

1. **Control plane** (registration, heartbeat, task dispatch, stake queries) —
   low bandwidth, latency-tolerant, high privacy value.
2. **Data plane** (gradient uploads, model downloads, Stratum v2) — high
   bandwidth, latency-sensitive, clearnet-only.

Lightning Network channel management is handled by LND, which already
supports Tor `.onion` addresses natively — no OWM-specific work is required
for that path.

## Decision

- The coordinator exposes **two gRPC endpoints** for the control plane:
  - **Clearnet**: the primary endpoint (e.g. `coordinator.owm.network:9000`)
  - **Tor hidden service**: a `.onion` v3 address bound to a local-only port
    (e.g. `127.0.0.1:9002`) that Tor forwards to the same gRPC server instance.
- Node operators may configure a **SOCKS5 proxy** (typically `127.0.0.1:9050`)
  to route their control-plane gRPC connection through Tor.
- Nodes MAY register an `onion_address` alongside their clearnet `ln_node_uri`
  so other network participants can reach them via Tor.
- **Data-plane traffic is clearnet-only**. Tor is explicitly unsupported for:
  - FL gradient uploads / model weight downloads
  - Stratum v2 mining connections
- **Lightning channels** may use Tor `.onion` addresses — this is native LND
  capability and requires no coordinator changes.

## Transport Scope Matrix

| Traffic Type | Clearnet | Tor |
|---|---|---|
| gRPC control plane (register, heartbeat, task dispatch, stake) | ✓ default | ✓ opt-in via SOCKS5 |
| FL gradient uploads | ✓ required | ✗ too slow |
| Model weight downloads | ✓ required | ✗ too slow |
| Stratum v2 mining | ✓ required | ✗ latency-sensitive |
| Lightning channel (ln_node_uri) | ✓ default | ✓ native LND support |
| Coordinator `.onion` endpoint | — | ✓ published alongside clearnet |

## Implementation

```
owm-coordinator/
  cmd/coordinator/main.go   ← dual net.Listener (clearnet + Tor local bind)
  internal/config/config.go ← TorConfig struct
  internal/registry/        ← onion_address stored per node (optional)
  proto/coordinator/v1/     ← onion_address field in RegisterNodeRequest

Tor daemon (operator-managed):
  HiddenServiceDir /var/lib/tor/owm-coordinator/
  HiddenServicePort 9000 127.0.0.1:9002   # forwards .onion:9000 → local:9002
```

Nodes routing through Tor configure:
```toml
[tor]
enabled        = true
socks5_addr    = "127.0.0.1:9050"   # local Tor SOCKS5 proxy
```

The coordinator reads its `.onion` hostname from
`HiddenServiceDir/hostname` at startup and logs it so operators can publish it.

## Consequences

### Positive
- Node operators can contribute without exposing their IP address.
- Censorship-resistant onboarding — nodes in restrictive jurisdictions can
  register and receive tasks without unobfuscated clearnet exposure.
- Minimal performance impact — Tor is restricted to low-bandwidth control-plane
  messages; all bulk data stays on clearnet.
- The single gRPC server instance handles both listeners — no code duplication.
- Lightning's native Tor support is leveraged at zero implementation cost.

### Negative
- Operators must run a local Tor daemon to use the `.onion` endpoint (small
  operational overhead).
- The coordinator must manage two listeners and publish two addresses.
- SOCKS5 dial support in the gRPC client (node side) adds a dependency on
  `golang.org/x/net/proxy`.

## Alternatives Considered

| Option | Reason Rejected |
|---|---|
| Tor for all traffic | Gradient uploads / mining would be unacceptably slow |
| I2P instead of Tor | Much smaller anonymity set; less tooling; no LND integration |
| VPN-only (WireGuard) | Requires coordinator-managed VPN infrastructure; node operators still expose IP to VPN server |
| No Tor support | Excludes privacy-conscious operators; weakens censorship resistance |
