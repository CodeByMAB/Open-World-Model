# ADR-003: PostgreSQL for Coordinator State Storage

**Date:** 2026-03-05
**Status:** Accepted
**Deciders:** Core Maintainers

---

## Context

The coordinator manages mutable state: node registry, task queue, FL round state, stake channel records, slashing log, bounties, and payment history. This state must be durable, queryable, and consistent across coordinator replicas.

## Decision

Use **PostgreSQL 16+** as the coordinator's primary data store, with:
- WAL replication to one read replica for high availability.
- `pgx` as the Go driver (faster than `lib/pq`, supports `pgxpool`).
- `golang-migrate` for versioned schema migrations.
- Redis for ephemeral state: rate limiting, heartbeat cache, task queue hot path.

## Consequences

### Positive
- ACID guarantees prevent double-payment and double-task-assignment races.
- JSONB columns handle flexible metadata (sub-model hashes, OTS proof paths) without schema churn.
- Rich query support for analytics (e.g., per-node earnings, FL round stats).
- WAL replication enables zero-downtime failover.
- Mature Go ecosystem (`pgx`, `sqlx`, `golang-migrate`).

### Negative
- Operational overhead of managing a Postgres cluster vs. embedded storage.
- Single database is still a coordination bottleneck at extreme scale (>10k active nodes) — mitigated by Phase 4 DHT migration.

### Mitigation for scale
- Read replicas handle read-heavy workloads (node status queries, earnings lookups).
- Redis caches heartbeat state to avoid hitting Postgres on every 60-second ping.
- The slashing log is append-only and can be exported to an immutable store (e.g., IPFS or a Bitcoin OP_RETURN summary) in Phase 4.

## Alternatives Considered

| Option | Reason Rejected |
|---|---|
| SQLite | No horizontal replication; not suitable for multi-replica coordinator |
| MongoDB | Weaker ACID guarantees; JOIN-heavy queries are awkward |
| etcd | Designed for configuration, not relational data; poor query support |
| CockroachDB | Operationally complex; overkill for Phase 1–3 scale |
