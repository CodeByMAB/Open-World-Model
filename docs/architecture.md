# OWM System Architecture

**Version:** 1.0.0
**Date:** 2026-03-05
**Status:** Approved

---

## Table of Contents

1. [Overview](#1-overview)
2. [Component Topology](#2-component-topology)
3. [Sequence Diagrams](#3-sequence-diagrams)
   - 3.1 [Node Registration & Stake Verification](#31-node-registration--stake-verification)
   - 3.2 [Task Dispatch & Compute Reward](#32-task-dispatch--compute-reward)
   - 3.3 [Federated Learning Round](#33-federated-learning-round)
   - 3.4 [Model Version Anchoring (OpenTimestamps)](#34-model-version-anchoring-opentimestamps)
   - 3.5 [Slashing Flow](#35-slashing-flow)
   - 3.6 [Mining Pool Share & Payout](#36-mining-pool-share--payout)
   - 3.7 [Commercial API Request (Lightning Gated)](#37-commercial-api-request-lightning-gated)
   - 3.8 [GitHub Bounty Payment](#38-github-bounty-payment)
4. [Database Schema (Coordinator)](#4-database-schema-coordinator)
5. [Network Communication Matrix](#5-network-communication-matrix)
6. [Deployment Topology](#6-deployment-topology)

---

## 1. Overview

The OWM system is composed of five independently deployable services that communicate over encrypted channels:

| Service | Language | Role |
|---|---|---|
| `owm-coordinator` | Go 1.22 | Central registry, task scheduler, FL orchestrator, PoS enforcer |
| `owm-node` | Python 3.11 + Rust | Contributor node daemon — inference, training, mining |
| `owm-pool` | Rust (SRI) | Stratum v2 Bitcoin mining pool |
| `owm-governance` | Python / HTMX | Rough-consensus governance portal |
| `bitcoin-node` | Bitcoin Core / Knots | Full node for pool `getblocktemplate` |

All inter-service communication between the coordinator and nodes uses **mTLS + gRPC**. The Lightning Network provides the payment rail. The Bitcoin blockchain (via OpenTimestamps) provides model provenance.

---

## 2. Component Topology

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                            Internet / P2P                                   │
│                                                                             │
│   Node Operators                          Miners / ASIC Operators           │
│   ┌──────────┐  ┌──────────┐             ┌──────────┐  ┌──────────┐        │
│   │ owm-node │  │ owm-node │             │ GPU Miner│  │   ASIC   │        │
│   │  (T1/T2) │  │  (T3)   │             │ (owm-mine│  │  Farmer  │        │
│   └────┬─────┘  └────┬─────┘             └────┬─────┘  └────┬─────┘        │
│        │ mTLS/gRPC   │ mTLS/gRPC              │ SV2         │ SV2          │
│        │             │                         │             │              │
│        └──────┬──────┘              ┌──────────┘             │              │
│               │                    ▼                         │              │
│               ▼           ┌─────────────────┐               │              │
│   ┌───────────────────┐   │   owm-pool      │◄──────────────┘              │
│   │  owm-coordinator  │   │ (Stratum v2)    │                              │
│   │                   │   │                 │                              │
│   │  ┌─────────────┐  │   │ ┌─────────────┐│                              │
│   │  │  Registry   │  │   │ │FPPS Engine  ││                              │
│   │  │  Scheduler  │  │   │ └──────┬──────┘│                              │
│   │  │  FL Engine  │  │   └────────┼────────┘                             │
│   │  │  Stake Mgr  │  │            │ 20% block reward (LN)                │
│   │  └──────┬──────┘  │            ▼                                      │
│   │         │         │   ┌─────────────────┐       ┌──────────────────┐  │
│   │  ┌──────▼──────┐  │   │  LND Treasury   │◄──────│  bitcoin-node    │  │
│   │  │  PostgreSQL │  │   │  (multisig)     │       │ (Core / Knots)   │  │
│   │  └─────────────┘  │   └────────┬────────┘       └──────────────────┘  │
│   └───────────────────┘            │ 80% rewards + compute pay (LN)       │
│                                    ▼                                       │
│                          ┌──────────────────┐                              │
│                          │   Public API      │  ┌────────────────────────┐ │
│                          │   (FastAPI)       │  │  owm-governance        │ │
│                          └──────────────────┘  │  (HTMX portal)         │ │
│                                                └────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 3. Sequence Diagrams

### 3.1 Node Registration & Stake Verification

```mermaid
sequenceDiagram
    actor Operator
    participant Node as owm-node
    participant Coord as owm-coordinator
    participant Stake as Stake Manager
    participant LND as Treasury LND

    Operator->>Node: owm-node start --config owm.toml
    Node->>Node: Generate Ed25519 identity keypair
    Node->>Node: Detect GPU tier (nvidia-smi / rocm-smi)
    Node->>Coord: RegisterNodeRequest(pubkey, ln_uri, capabilities, signature)
    Coord->>Coord: Verify Ed25519 signature
    Coord->>Stake: VerifyStake(node_pubkey, declared_tier)
    Stake->>LND: ListChannels(peer_pubkey=node_pubkey)
    LND-->>Stake: channel list

    alt Channel exists AND capacity >= tier_minimum
        Stake-->>Coord: STAKE_OK (channel_id, capacity_sats, bonus_multiplier)
        Coord->>Coord: Write node record (status=active)
        Coord-->>Node: RegisterNodeResponse(node_id, status=active, tasks_endpoint)
        Node-->>Operator: Node active ✓ | Stake: 100k sats | Tier: T1 | Multiplier: 1.0x
    else Channel missing or insufficient
        Stake-->>Coord: STAKE_INSUFFICIENT(required_sats, tier)
        Coord-->>Node: RegisterNodeResponse(status=pending, open_channel_instructions)
        Node-->>Operator: Open LN channel to treasury: <pubkey>@<host> with ≥100,000 sats
        Note over Operator,LND: Operator opens channel on-chain (≥1 block confirmation)
        Node->>Coord: RegisterNodeRequest(...) [retry]
        Coord->>Stake: VerifyStake(...)
        Stake->>LND: ListChannels(...)
        LND-->>Stake: channel confirmed
        Stake-->>Coord: STAKE_OK
        Coord-->>Node: RegisterNodeResponse(status=active)
    end

    loop Every 60s
        Node->>Coord: Heartbeat(node_id, metrics, signature)
        Coord-->>Node: HeartbeatAck(pending_tasks_count)
    end
```

---

### 3.2 Task Dispatch & Compute Reward

```mermaid
sequenceDiagram
    participant API as Public API
    participant Coord as owm-coordinator
    participant Sched as Task Scheduler
    participant Node as owm-node (T2)
    participant LND as Treasury LND

    API->>Coord: InferenceRequest(query, max_tokens)
    Coord->>Sched: ScheduleTask(type=inference, min_tier=T1)
    Sched->>Sched: Select node by (reliability_score * tier_multiplier * stake_bonus)
    Sched-->>Coord: assigned_node=Node, reward=25 sats
    Coord->>Node: Task(task_id, type=inference, encrypted_payload, timeout=30s)
    Node->>Node: Decrypt payload, run owm-code + owm-geo sub-models
    Node-->>Coord: TaskResult(task_id, output_hash, output, exec_ms, signature)
    Coord->>Coord: Verify signature, validate output_hash
    Coord-->>API: InferenceResponse(answer, model_version, ots_verified)

    Note over Coord,LND: Payment dispatch (async, ≤60s)
    Coord->>LND: SendPayment(dest=node.ln_uri, amount=25 sats, memo=task_id)
    LND-->>Coord: payment_preimage (success)
    Coord->>Coord: Log payment(task_id, node_id, 25 sats, preimage, paid)
```

---

### 3.3 Federated Learning Round

```mermaid
sequenceDiagram
    participant Coord as owm-coordinator (FL Engine)
    participant N1 as Node A (T2)
    participant N2 as Node B (T2)
    participant N3 as Node C (T1)
    participant OTS as OTS Calendar
    participant LND as Treasury LND

    Coord->>Coord: Select eligible nodes (online, uptime ≥ SLA, stake verified)
    Coord->>N1: FLRoundTask(round=142, model_hash, training_config)
    Coord->>N2: FLRoundTask(round=142, model_hash, training_config)
    Coord->>N3: FLRoundTask(round=142, model_hash, training_config)

    par Local training
        N1->>N1: Load model (verify OTS proof), train on local data N steps
        N1->>N1: Compute top-K sparse gradient delta (10% sparsification)
        N1-->>Coord: GradientUpdate(round=142, delta, signature)
    and
        N2->>N2: Load model, train, compute gradient
        N2-->>Coord: GradientUpdate(round=142, delta, signature)
    and
        N3->>N3: Load model, train, compute gradient
        N3-->>Coord: GradientUpdate(round=142, delta, signature)
    end

    Coord->>Coord: Verify all signatures
    Coord->>Coord: Anomaly detection (clip L2 norm, flag outliers >3σ)
    Coord->>Coord: FedAvg aggregation (weighted by tier × stake_bonus)
    Coord->>Coord: Produce new checkpoint owm-v0.4.3, compute SHA-256

    Coord->>OTS: StampHash(sha256=checkpoint_hash)
    OTS-->>Coord: OTSReceipt (pending Bitcoin confirmation)
    Coord->>Coord: Store checkpoint + OTS receipt

    Coord->>N1: NewCheckpoint(version, hash, ots_proof_url)
    Coord->>N2: NewCheckpoint(version, hash, ots_proof_url)
    Coord->>N3: NewCheckpoint(version, hash, ots_proof_url)

    Note over Coord,LND: FL participation rewards (async)
    Coord->>LND: BatchPay([N1: 75 sats, N2: 75 sats, N3: 30 sats])
    LND-->>Coord: all payments confirmed
```

---

### 3.4 Model Version Anchoring (OpenTimestamps)

```mermaid
sequenceDiagram
    participant Coord as owm-coordinator
    participant OTS1 as OTS Calendar 1 (public)
    participant OTS2 as OTS Calendar 2 (public)
    participant OTS3 as OTS Calendar 3 (owm.network)
    participant BTC as Bitcoin Network
    participant Node as owm-node

    Coord->>Coord: Produce checkpoint, compute SHA-256(weights_file)
    par Submit to 3 calendars
        Coord->>OTS1: POST /digest {sha256: "a3f9..."}
        OTS1-->>Coord: OTS receipt (merkle inclusion proof, pending)
    and
        Coord->>OTS2: POST /digest {sha256: "a3f9..."}
        OTS2-->>Coord: OTS receipt
    and
        Coord->>OTS3: POST /digest {sha256: "a3f9..."}
        OTS3-->>Coord: OTS receipt
    end

    Coord->>Coord: Store .ots proof files alongside checkpoint
    Coord->>Coord: Publish ModelVersion manifest (ots_verified=pending)

    Note over OTS1,BTC: Bitcoin confirms (typically 1 block ≈ 10 min)
    OTS1->>BTC: Anchor merkle root in OP_RETURN
    BTC-->>OTS1: Block confirmation (height=890421)
    OTS1->>Coord: Upgrade receipt → full Bitcoin proof

    Coord->>Coord: Update ModelVersion (ots_verified=true, btc_block=890421)

    Node->>Coord: GetModelVersion(current)
    Coord-->>Node: ModelVersion(hash, ots_proof_url, btc_block=890421)
    Node->>Node: Download checkpoint
    Node->>Node: ots verify checkpoint.ots (must pass before load)
    Node->>Node: Load verified weights
```

---

### 3.5 Slashing Flow

```mermaid
sequenceDiagram
    participant Anom as Anomaly Detector
    participant Stake as Stake Manager
    participant Coord as owm-coordinator
    participant LND as Treasury LND
    participant Log as Slashing Log (DB)
    participant Node as Misbehaving Node

    Anom->>Stake: MisbehaviorSignal(node_id, type=poisoned_gradient, evidence_hash)
    Stake->>Stake: Log signal #1 (node_id, evidence_hash, timestamp)

    Note over Anom,Stake: Signal #2 received...
    Anom->>Stake: MisbehaviorSignal(node_id, type=poisoned_gradient, evidence_hash_2)
    Stake->>Stake: Log signal #2

    Note over Anom,Stake: Signal #3 — threshold reached (T1 node)
    Anom->>Stake: MisbehaviorSignal(node_id, type=poisoned_gradient, evidence_hash_3)
    Stake->>Stake: signal_count=3 → SLASH threshold reached

    alt Tier 1 (automated)
        Stake->>Coord: InitiateSlash(node_id, evidence_bundle_hash)
        Coord->>Coord: Set node status = suspended
        Coord->>LND: ForceCloseChan(channel_id=node.stake_channel_id) [dedicated slash credential]
        LND-->>Coord: ForceCloseResponse (pending on-chain)
        Coord->>Log: WriteSlashEvent(node_id, channel_id, reason, evidence_hash, timestamp, cooldown_expires=+30d)
        Coord-->>Node: NextHeartbeatAck → SUSPENDED (cooldown until <date>)
    else Tier 2 or 3 (requires 2 maintainer acks)
        Stake->>Coord: SlashProposal(node_id, evidence_bundle_hash, tier=T2)
        Coord->>Coord: Notify maintainers via alert
        Note over Coord: Maintainer A signs SlashAck
        Note over Coord: Maintainer B signs SlashAck → 2/2 reached
        Coord->>LND: ForceCloseChan(channel_id)
        LND-->>Coord: ForceCloseResponse
        Coord->>Log: WriteSlashEvent(...)
    end
```

---

### 3.6 Mining Pool Share & Payout

```mermaid
sequenceDiagram
    participant Miner as GPU/ASIC Miner (owm-mine)
    participant Pool as owm-pool (SRI)
    participant BNode as bitcoin-node (Core/Knots)
    participant LND as Treasury LND
    participant Coord as owm-coordinator

    Pool->>BNode: getblocktemplate
    BNode-->>Pool: block template (prev_hash, tx_list, target)
    Pool->>Miner: MiningNotify(job_id, template) [SV2 encrypted]

    loop Mining
        Miner->>Miner: Hash candidates (GPU/ASIC)
        Miner->>Pool: SubmitShare(job_id, nonce, ntime, worker=node_pubkey)
        Pool->>Pool: Validate share (correct job_id, meets pool difficulty)
        Pool->>Pool: Accrue FPPS credit: (subsidy / pool_diff) × 0.80 sats

        alt Pool finds a valid block
            Pool->>BNode: submitblock(solved_block)
            BNode-->>Pool: Block accepted!
            Pool->>Coord: BlockFound(height, reward_sats, miner_shares)
            Note over Pool,LND: Batch payout within 60 minutes
            Pool->>LND: BatchPay([miner1: X sats, miner2: Y sats, treasury: Z sats])
            LND-->>Pool: all payments confirmed
        end
    end
```

---

### 3.7 Commercial API Request (Lightning Gated)

```mermaid
sequenceDiagram
    actor Client
    participant API as Public API (FastAPI)
    participant LND as Treasury LND
    participant Coord as owm-coordinator
    participant Node as owm-node

    Client->>API: GET /v1/payments/invoice?tier=commercial
    API->>LND: AddInvoice(amount=5 sats, memo="OWM API access 24h", expiry=3600)
    LND-->>API: BOLT11 invoice string
    API-->>Client: {invoice: "lnbc5n1p...", expires_in: 3600}

    Client->>LND: Pay invoice (via user's LN wallet)
    LND-->>API: InvoiceSettled webhook (payment_hash, preimage)
    API->>API: Cache access_token → payment_hash (TTL 24h)

    Client->>API: POST /v1/query {query: "...", Authorization: Bearer <payment_hash>}
    API->>API: Validate token (payment_hash exists, not expired)
    API->>Coord: InferenceRequest(query)
    Coord->>Node: Task(inference, payload)
    Node-->>Coord: TaskResult
    Coord-->>API: answer, model_version, ots_verified, latency_ms, cost_sats
    API-->>Client: 200 OK {answer, model_version, ots_verified: true}
```

---

### 3.8 GitHub Bounty Payment

```mermaid
sequenceDiagram
    participant GH as GitHub
    participant App as GitHub App (owm.github)
    participant Bounty as Bounty Manager
    participant DB as Coordinator DB
    participant LND as Treasury LND
    participant Dev as Developer's LN Wallet

    Note over GH: Maintainer creates Issue with label "owm-bounty: 50000"
    GH->>App: IssueEvent(labeled, issue_url, label="owm-bounty: 50000")
    App->>Bounty: RegisterBounty(issue_url, repo, amount_sats=50000)
    Bounty->>DB: Insert bounty (status=open)

    Note over GH: Developer opens PR linked to bounty issue
    Note over GH: Maintainer reviews and merges PR
    GH->>App: PullRequestEvent(merged, pr_url, linked_issue_url)
    App->>Bounty: CheckBounty(linked_issue_url)
    Bounty->>DB: Find bounty (status=open, amount=50000 sats)
    Bounty->>DB: Get developer's registered LN address

    alt Developer has registered LN address
        Bounty->>LND: SendPayment(dest=dev_ln_address, amount=50000 sats, memo=pr_url)
        LND-->>Bounty: payment_preimage
        Bounty->>DB: Update bounty (status=paid, preimage, merged_pr_url)
        Bounty->>GH: Comment on PR "Bounty of 50,000 sats paid! ⚡ <preimage>"
    else Developer not registered
        Bounty->>GH: Comment on PR "Register your LN address at owm.network/register to claim 50,000 sats"
        Note over Bounty: Payment held for 30 days, then returned to treasury
    end
```

---

## 4. Database Schema (Coordinator)

All coordinator state lives in PostgreSQL. The schema is versioned with migrations.

```sql
-- nodes
CREATE TABLE nodes (
    node_id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    public_key      TEXT NOT NULL UNIQUE,         -- Ed25519 hex
    ln_node_uri     TEXT NOT NULL,                -- pubkey@host:port
    tier            TEXT NOT NULL CHECK (tier IN ('t1','t2','t3')),
    vram_gb         NUMERIC(6,2),
    ram_gb          NUMERIC(6,2),
    bandwidth_mbps  NUMERIC(8,2),
    reliability     NUMERIC(5,4) DEFAULT 1.0,     -- 0.0–1.0 rolling 7d
    total_tasks     INTEGER DEFAULT 0,
    total_sats      BIGINT DEFAULT 0,
    status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending','active','degraded','suspended')),
    registered_at   TIMESTAMPTZ DEFAULT now(),
    last_heartbeat  TIMESTAMPTZ
);

-- node_stakes
CREATE TABLE node_stakes (
    node_id             UUID PRIMARY KEY REFERENCES nodes(node_id),
    channel_id          TEXT NOT NULL UNIQUE,     -- LN channel ID hex
    channel_capacity    BIGINT NOT NULL,           -- total sats
    local_balance       BIGINT NOT NULL,           -- node's locked sats
    tier_minimum        BIGINT NOT NULL,           -- required for tier
    bonus_multiplier    NUMERIC(4,3) DEFAULT 1.0, -- 1.000–2.000
    stake_status        TEXT NOT NULL DEFAULT 'active'
                        CHECK (stake_status IN ('active','degraded','force_closed')),
    opened_at           TIMESTAMPTZ DEFAULT now(),
    last_verified_at    TIMESTAMPTZ DEFAULT now(),
    degraded_since      TIMESTAMPTZ
);

-- tasks
CREATE TABLE tasks (
    task_id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_type       TEXT NOT NULL,
    assigned_node   UUID REFERENCES nodes(node_id),
    status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending','running','completed','failed','timeout')),
    input_hash      TEXT,
    output_hash     TEXT,
    reward_sats     INTEGER,
    payment_status  TEXT DEFAULT 'pending'
                    CHECK (payment_status IN ('pending','paid','failed')),
    payment_preimage TEXT,
    submitted_at    TIMESTAMPTZ DEFAULT now(),
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    timeout_seconds INTEGER DEFAULT 30
);

-- fl_rounds
CREATE TABLE fl_rounds (
    round_id        SERIAL PRIMARY KEY,
    round_number    INTEGER NOT NULL UNIQUE,
    status          TEXT NOT NULL DEFAULT 'open'
                    CHECK (status IN ('open','aggregating','complete','failed')),
    model_version   TEXT,                          -- output version
    checkpoint_hash TEXT,
    ots_proof_path  TEXT,
    btc_block       INTEGER,
    started_at      TIMESTAMPTZ DEFAULT now(),
    completed_at    TIMESTAMPTZ,
    participant_count INTEGER
);

-- fl_participants
CREATE TABLE fl_participants (
    round_id        INTEGER REFERENCES fl_rounds(round_id),
    node_id         UUID REFERENCES nodes(node_id),
    gradient_hash   TEXT,
    anomaly_flagged BOOLEAN DEFAULT FALSE,
    reward_sats     INTEGER,
    PRIMARY KEY (round_id, node_id)
);

-- model_versions
CREATE TABLE model_versions (
    version_id          TEXT PRIMARY KEY,          -- e.g. "owm-v0.4.2"
    round_number        INTEGER REFERENCES fl_rounds(round_number),
    created_at          TIMESTAMPTZ DEFAULT now(),
    sub_model_hashes    JSONB,                     -- {sub_model_id: sha256}
    ots_proof_paths     JSONB,                     -- {sub_model_id: path}
    btc_block           INTEGER,
    coordinator_sig     TEXT,
    is_current          BOOLEAN DEFAULT FALSE
);

-- slashing_events
CREATE TABLE slashing_events (
    event_id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    node_id             UUID REFERENCES nodes(node_id),
    channel_id          TEXT NOT NULL,
    tier                TEXT NOT NULL,
    reason              TEXT NOT NULL,
    evidence_hash       TEXT NOT NULL,
    signal_count        INTEGER NOT NULL,
    executed_at         TIMESTAMPTZ DEFAULT now(),
    cooldown_expires_at TIMESTAMPTZ NOT NULL,
    coordinator_sig     TEXT NOT NULL
);

-- misbehavior_signals
CREATE TABLE misbehavior_signals (
    signal_id       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    node_id         UUID REFERENCES nodes(node_id),
    signal_type     TEXT NOT NULL,
    evidence_hash   TEXT NOT NULL,
    detected_at     TIMESTAMPTZ DEFAULT now(),
    slashing_event  UUID REFERENCES slashing_events(event_id)
);

-- bounties
CREATE TABLE bounties (
    bounty_id       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    github_issue    TEXT NOT NULL UNIQUE,
    github_repo     TEXT NOT NULL,
    amount_sats     BIGINT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'open'
                    CHECK (status IN ('open','claimed','paid','expired')),
    claimed_by      TEXT,                          -- GitHub login
    merged_pr_url   TEXT,
    created_at      TIMESTAMPTZ DEFAULT now(),
    paid_at         TIMESTAMPTZ,
    payment_preimage TEXT
);

-- Indexes
CREATE INDEX idx_nodes_status         ON nodes(status);
CREATE INDEX idx_nodes_pubkey         ON nodes(public_key);
CREATE INDEX idx_tasks_assigned_node  ON tasks(assigned_node, status);
CREATE INDEX idx_tasks_submitted      ON tasks(submitted_at DESC);
CREATE INDEX idx_slashing_node        ON slashing_events(node_id);
CREATE INDEX idx_signals_node         ON misbehavior_signals(node_id, detected_at DESC);
```

---

## 5. Network Communication Matrix

| From | To | Protocol | Auth | Port |
|---|---|---|---|---|
| `owm-node` | `owm-coordinator` | gRPC + mTLS | Ed25519 node identity cert | 9000 |
| `owm-node` | `owm-pool` | Stratum v2 (Noise) | OWM Ed25519 identity key | 3333 |
| `owm-coordinator` | `Treasury LND` | gRPC | Macaroon (admin) | 10009 |
| `owm-coordinator` | `Treasury LND` (slash) | gRPC | Macaroon (restricted: ForceCloseChan only) | 10009 |
| `owm-pool` | `bitcoin-node` | JSON-RPC over HTTP | RPC username/password | 8332 |
| `owm-pool` | `Treasury LND` | gRPC | Macaroon (invoice + pay) | 10009 |
| `Public API` | `owm-coordinator` | gRPC (internal) | Service mesh mTLS | 9001 |
| `GitHub App` | `owm-coordinator` | HTTP (internal) | Service token | 9002 |
| Operators | `Public API` | HTTPS | API key / LN invoice | 443 |
| Operators | `owm-governance` | HTTPS | Node identity signature | 443 |
| `OTS Calendars` | External | HTTPS | None (public) | 443 |

---

## 6. Deployment Topology

```
                        ┌──────────────────────────────┐
                        │   Coordinator Cluster (k8s)  │
                        │                              │
                        │  ┌──────────┐ ┌──────────┐  │
                        │  │coord-api │ │coord-api │  │    ← 2 replicas
                        │  │replica-1 │ │replica-2 │  │
                        │  └────┬─────┘ └────┬─────┘  │
                        │       └──────┬──────┘        │
                        │         ┌────▼────┐          │
                        │         │postgres │          │    ← primary + 1 read replica
                        │         │ (WAL)   │          │
                        │         └─────────┘          │
                        │  ┌───────┐  ┌────────────┐   │
                        │  │ redis │  │ minio/S3   │   │    ← cache + model storage
                        │  └───────┘  └────────────┘   │
                        │  ┌──────────────────────┐    │
                        │  │  owm-pool  (Rust)    │    │    ← mining pool
                        │  └──────────────────────┘    │
                        │  ┌──────────────────────┐    │
                        │  │  bitcoin-node         │   │    ← Core or Knots
                        │  └──────────────────────┘    │
                        │  ┌──────────────────────┐    │
                        │  │  LND (treasury)      │    │    ← Lightning node
                        │  └──────────────────────┘    │
                        │  ┌──────────────────────┐    │
                        │  │  OTS calendar        │    │    ← self-hosted OTS
                        │  └──────────────────────┘    │
                        │  ┌──────────────────────┐    │
                        │  │  Public API (FastAPI) │   │    ← 2 replicas
                        │  └──────────────────────┘    │
                        │  ┌──────────────────────┐    │
                        │  │  Governance Portal   │    │
                        │  └──────────────────────┘    │
                        │  ┌──────────────────────┐    │
                        │  │  nginx (TLS, gateway) │   │
                        │  └──────────────────────┘    │
                        └──────────────────────────────┘
                                     ↕ P2P gRPC/mTLS
                ┌────────────────────────────────────────────┐
                │              Node Operators (self-hosted)  │
                │  ┌──────────┐  ┌──────────┐  ┌──────────┐ │
                │  │owm-node  │  │owm-node  │  │owm-node  │ │
                │  │T1 + mine │  │T2 + mine │  │T3        │ │
                │  └──────────┘  └──────────┘  └──────────┘ │
                └────────────────────────────────────────────┘
```

---

*End of Architecture Document v1.0.0*
