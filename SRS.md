# Software Requirements Specification (SRS)

**Project:** Open World Model (OWM)
**Version:** 1.0.0
**Date:** 2026-03-05
**Status:** Draft — Pending Stakeholder Review
**BRS Reference:** BRS v1.0.0

---

## Table of Contents

1. [Introduction](#1-introduction)
2. [Overall Description](#2-overall-description)
3. [System Architecture Overview](#3-system-architecture-overview)
4. [Component Specifications](#4-component-specifications)
   - 4.1 [OWM Node Daemon](#41-owm-node-daemon)
   - 4.2 [Federated Ensemble Model Engine](#42-federated-ensemble-model-engine)
   - 4.3 [Task Scheduler & Compute Marketplace](#43-task-scheduler--compute-marketplace)
   - 4.4 [Bitcoin & Lightning Payment System](#44-bitcoin--lightning-payment-system)
   - 4.5 [OpenTimestamps Integration](#45-opentimestamps-integration)
   - 4.6 [Data Pipeline](#46-data-pipeline)
   - 4.7 [GitHub Integration Service](#47-github-integration-service)
   - 4.8 [Public REST/WebSocket API](#48-public-restwebsocket-api)
   - 4.9 [Governance Portal](#49-governance-portal)
   - 4.10 [Coordinator Service (Bootstrap Layer)](#410-coordinator-service-bootstrap-layer)
5. [Data Models](#5-data-models)
6. [API Specifications](#6-api-specifications)
7. [Security Requirements](#7-security-requirements)
8. [Performance Requirements](#8-performance-requirements)
9. [Testing Requirements](#9-testing-requirements)
10. [Deployment & Infrastructure](#10-deployment--infrastructure)
11. [Documentation Requirements](#11-documentation-requirements)
12. [Technology Stack](#12-technology-stack)
13. [Directory Structure](#13-directory-structure)

---

## 1. Introduction

### 1.1 Purpose

This Software Requirements Specification defines the technical requirements, architecture, interfaces, and constraints for implementing the Open World Model (OWM) system as described in BRS v1.0.0.

### 1.2 Scope

This document covers all software components of the OWM system: the node daemon, federated model engine, Lightning payment subsystem, OpenTimestamps anchoring, GitHub integration service, public API, and governance portal.

### 1.3 Definitions & Acronyms

| Term | Meaning |
|---|---|
| **OWM** | Open World Model |
| **LN** | Lightning Network |
| **OTS** | OpenTimestamps |
| **LDK** | Lightning Development Kit (Rust) |
| **LND** | Lightning Network Daemon (Go) |
| **gRPC** | Google Remote Procedure Call |
| **FL** | Federated Learning |
| **PBFT** | Practical Byzantine Fault Tolerance |
| **P2P** | Peer-to-Peer |
| **HMAC** | Hash-based Message Authentication Code |
| **mTLS** | Mutual TLS — bidirectional certificate authentication |
| **VRAM** | Video RAM on GPU |
| **sat** | Satoshi — 1/100,000,000th of a Bitcoin |

### 1.4 References

- BRS v1.0.0 (`BRS.md`)
- Bitcoin Lightning Network BOLT specifications: https://github.com/lightning/bolts
- OpenTimestamps specification: https://opentimestamps.org
- Federated Learning: McMahan et al., "Communication-Efficient Learning of Deep Networks from Decentralized Data" (2017)

---

## 2. Overall Description

### 2.1 Product Perspective

OWM is a standalone distributed system composed of:
- A **P2P node network** where each node runs the OWM daemon.
- A **coordinator layer** that bootstraps the network and routes tasks (decentralizes progressively).
- A **model layer** consisting of a federated ensemble of transformer-based sub-models.
- A **payment layer** using Bitcoin Lightning for reward distribution.
- A **provenance layer** using OpenTimestamps for model integrity.
- A **developer layer** exposing a public API and GitHub integration tooling.

### 2.2 User Classes

| User Class | Interface | Primary Action |
|---|---|---|
| Node Operator | CLI + Dashboard | Run node daemon, earn BTC |
| Data Contributor | CLI + API | Submit datasets, earn BTC |
| API Consumer | REST/WebSocket API | Query world model |
| GitHub User | GitHub App + Bounty Board | Contribute code, earn BTC |
| Core Maintainer | All interfaces + admin CLI | Deploy, configure, govern |
| Treasury Signer | Multisig wallet UI | Approve treasury transactions |

### 2.3 Operating Environment

- **Node OS:** Linux (Ubuntu 22.04+ primary; Debian/Arch supported). Windows/macOS via Docker.
- **GPU:** NVIDIA CUDA 12.x (primary); AMD ROCm 6.x (Tier 1/2).
- **Python:** 3.11+
- **Rust:** 1.75+ (for LDK Lightning integration and performance-critical components)
- **Container:** Docker 24+ / Podman 4+
- **Orchestration:** Kubernetes (k8s) for coordinator; standalone daemon for nodes

---

## 3. System Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│                        OWM Network                                  │
│                                                                     │
│  ┌────────────┐   ┌────────────┐   ┌────────────┐   ┌───────────┐  │
│  │  Node T3   │   │  Node T2   │   │  Node T1   │   │ Node T1   │  │
│  │ (server)   │   │(workstation│   │ (consumer) │   │(consumer) │  │
│  │ OWM Daemon │   │ OWM Daemon │   │ OWM Daemon │   │OWM Daemon │  │
│  └─────┬──────┘   └─────┬──────┘   └─────┬──────┘   └─────┬─────┘  │
│        │                │                │                │         │
│        └────────────────┴────────────────┴────────────────┘         │
│                                  │                                  │
│                    ┌─────────────▼──────────────┐                  │
│                    │    Coordinator Service      │                  │
│                    │  (Task Router + Registry)   │                  │
│                    └──────┬──────────────┬───────┘                  │
│                           │              │                          │
│             ┌─────────────▼──┐    ┌──────▼───────────┐             │
│             │  Federated     │    │   Lightning       │             │
│             │  Ensemble      │    │   Payment Hub     │             │
│             │  Model Engine  │    │   (Multisig       │             │
│             └────────────────┘    │    Treasury)      │             │
│                                   └──────────────┬────┘             │
│                                                  │                  │
│             ┌────────────────────────────────────▼────────────┐    │
│             │              Public API Layer                    │    │
│             │    REST / WebSocket / GitHub App Integration     │    │
│             └─────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────┘
```

### 3.1 Key Architectural Principles

1. **Progressive Decentralization**: The system bootstraps with a coordinator and progressively shifts routing to a DHT-based P2P protocol.
2. **Separation of Concerns**: Compute, payment, data, and governance are isolated services communicating via well-defined interfaces.
3. **Byzantine Fault Tolerance**: The federated learning aggregator uses gradient clipping and anomaly detection; node reputation affects task assignment.
4. **Zero Raw-Data Egress**: Federated learning nodes never transmit training data — only compressed gradient updates.
5. **Bitcoin-Native**: No application-level token; all incentives denominated in satoshis over Lightning.

---

## 4. Component Specifications

### 4.1 OWM Node Daemon

**Language:** Python 3.11 + Rust (via PyO3 for performance-critical paths)
**Entrypoint:** `owm-node` CLI

#### 4.1.1 Responsibilities

- Register with the coordinator (announce hardware tier, public key, LN node URI).
- Receive and execute compute tasks (inference, training, gradient aggregation).
- Report task completion proofs (hash of output + execution time + hardware attestation).
- Maintain an LN payment channel to receive reward satoshis.
- Participate in federated learning rounds.
- Self-update via signed release packages (OTS-verified).

#### 4.1.2 Functional Requirements

| ID | Requirement |
|---|---|
| SRS-NODE-01 | The daemon shall start with a single command: `owm-node start --config owm.toml` |
| SRS-NODE-02 | On first run, the daemon shall generate an Ed25519 identity keypair stored in `~/.owm/identity.key` |
| SRS-NODE-03 | The daemon shall detect available GPU resources via `nvidia-smi` / `rocm-smi` and classify the node into Tier 1, 2, or 3 automatically |
| SRS-NODE-04 | The daemon shall publish a signed capability manifest (hardware tier, VRAM, bandwidth estimate) to the coordinator every 60 seconds |
| SRS-NODE-05 | The daemon shall execute tasks in isolated Python subprocesses with GPU memory limits enforced via CUDA MPS |
| SRS-NODE-06 | The daemon shall maintain a local SQLite task log with inputs, outputs, timing, and payment status |
| SRS-NODE-07 | The daemon shall validate all incoming tasks against a schema before execution |
| SRS-NODE-08 | The daemon shall expose a local HTTP health endpoint on `localhost:8080/health` |
| SRS-NODE-09 | The daemon shall support graceful shutdown, completing in-progress tasks before stopping |
| SRS-NODE-10 | The daemon shall verify OTS proofs on all model weight updates before loading them |

#### 4.1.3 Node Tier Specifications

| Tier | Minimum VRAM | Minimum RAM | Minimum Bandwidth | Uptime SLA | Base Reward Multiplier |
|---|---|---|---|---|---|
| Tier 1 (Consumer) | 8 GB | 16 GB | 50 Mbps | 70% | 1.0x |
| Tier 2 (Prosumer) | 24 GB | 64 GB | 200 Mbps | 90% | 2.5x |
| Tier 3 (Server) | 80 GB+ | 256 GB | 1 Gbps | 99% | 8.0x |

#### 4.1.4 Configuration Schema (`owm.toml`)

```toml
[node]
alias = "my-node"           # Human-readable name
tier_override = ""          # Optional: "t1", "t2", "t3" (auto-detected if empty)
data_dir = "~/.owm"

[network]
coordinator_url = "https://coordinator.owm.network"
listen_port = 9000
max_concurrent_tasks = 4

[lightning]
lnd_rpc_host = "localhost:10009"
lnd_macaroon_path = "~/.lnd/data/chain/bitcoin/mainnet/admin.macaroon"
lnd_tls_cert_path = "~/.lnd/tls.cert"
min_payment_channel_sat = 100000

[model]
model_cache_dir = "~/.owm/models"
max_vram_percent = 80

[data]
allow_contributor_data = false
contributor_data_dir = ""
```

---

### 4.2 Federated Ensemble Model Engine

**Language:** Python 3.11 (PyTorch 2.x)
**Package:** `owm.model`

#### 4.2.1 Sub-Model Registry

The ensemble consists of specialized transformer-based models, each responsible for a knowledge domain:

| Sub-Model ID | Domain | Base Architecture | Input Modalities |
|---|---|---|---|
| `owm-geo` | Geography & Earth Science | Encoder-decoder transformer | Text, coordinates, GeoJSON |
| `owm-sci` | Science & Research | Decoder transformer | Text, structured data |
| `owm-code` | Software Engineering & Git | Code-specialized transformer | Text, code, diff patches |
| `owm-events` | Current Events & News | Streaming transformer | Text, time-series |
| `owm-data` | Structured Data & APIs | Tabular transformer | Tables, JSON, time-series |
| `owm-router` | Query Router | Small classification model | Text |

#### 4.2.2 Functional Requirements

| ID | Requirement |
|---|---|
| SRS-MODEL-01 | The `owm-router` sub-model shall classify incoming queries and route them to one or more domain sub-models |
| SRS-MODEL-02 | Multiple sub-models may be activated for a single query; results shall be merged by a learned fusion layer |
| SRS-MODEL-03 | Each sub-model shall expose a standard `predict(input: ModelInput) -> ModelOutput` interface |
| SRS-MODEL-04 | Sub-models shall support streaming token generation via an async generator interface |
| SRS-MODEL-05 | The model engine shall support quantized model variants (INT8, INT4) for Tier 1 nodes with limited VRAM |
| SRS-MODEL-06 | The federated learning coordinator shall implement FedAvg as the baseline aggregation algorithm |
| SRS-MODEL-07 | Gradient updates from nodes shall be clipped to a configurable L2 norm threshold before aggregation |
| SRS-MODEL-08 | The system shall detect and reject gradient updates that deviate more than 3 standard deviations from the current distribution (anomaly detection) |
| SRS-MODEL-09 | Each sub-model checkpoint shall be identified by a SHA-256 hash of its weights file |
| SRS-MODEL-10 | The model engine shall support incremental fine-tuning without full retraining via LoRA adapters |

#### 4.2.3 Federated Learning Round Protocol

```
Round Lifecycle:
1. Coordinator selects eligible nodes (online, above tier threshold)
2. Coordinator broadcasts current model checkpoint hash + training task config
3. Nodes download model (if not cached), train on local data for N steps
4. Nodes compute compressed gradient delta (top-K sparsification, N=10%)
5. Nodes sign gradient package with Ed25519 identity key
6. Coordinator receives gradients, runs anomaly detection, clips norms
7. Coordinator aggregates via FedAvg (weighted by node tier)
8. Coordinator produces new checkpoint, signs it, runs OTS anchoring
9. Coordinator broadcasts new checkpoint hash to all nodes
10. Nodes verify OTS proof before loading new weights
11. Coordinator dispatches Lightning payments to participating nodes
```

---

### 4.3 Task Scheduler & Compute Marketplace

**Language:** Python 3.11 + Go 1.22 (scheduler core)
**Package:** `owm.scheduler`

#### 4.3.1 Task Types

| Task Type | Description | Min Tier |
|---|---|---|
| `inference` | Run model inference for an API request | T1 |
| `fl_round` | Participate in a federated learning round | T1 |
| `gradient_agg` | Aggregate gradients from FL participants | T2 |
| `data_ingest` | Process and embed a data chunk into the model | T2 |
| `audit_repo` | Run GitHub security audit on a repository | T1 |
| `embed_data` | Generate vector embeddings for a dataset | T1 |

#### 4.3.2 Functional Requirements

| ID | Requirement |
|---|---|
| SRS-SCHED-01 | The scheduler shall assign tasks to nodes based on capability (tier), current load, and historical reliability score |
| SRS-SCHED-02 | Tasks shall have a configurable timeout; timed-out tasks shall be re-queued and re-assigned |
| SRS-SCHED-03 | The scheduler shall maintain a priority queue: FL rounds > inference > data ingest |
| SRS-SCHED-04 | Node reliability scores shall be computed as: `(successful_tasks / total_tasks) * uptime_fraction` over a rolling 7-day window |
| SRS-SCHED-05 | The scheduler shall expose a gRPC interface for the coordinator to submit and monitor tasks |
| SRS-SCHED-06 | Task dispatch records shall be written to a distributed append-only log (initially a Postgres table, migrating to a P2P log in Phase 2) |
| SRS-SCHED-07 | In Phase 2, the scheduler shall use a Kademlia DHT for decentralized node discovery and task routing |

---

### 4.4 Bitcoin & Lightning Payment System

**Language:** Rust (LDK) + Python bindings
**Package:** `owm.lightning`

#### 4.4.1 Architecture

```
┌─────────────────────────────────────┐
│         Treasury Node               │
│  ┌────────────┐  ┌───────────────┐  │
│  │ 3-of-5     │  │  LND / CLN    │  │
│  │ Multisig   │  │  Lightning    │  │
│  │ Wallet     │  │  Node         │  │
│  └────────────┘  └───────┬───────┘  │
└──────────────────────────┼──────────┘
                           │ Payment Channels
          ┌────────────────┼────────────────┐
          │                │                │
    ┌─────▼─────┐   ┌──────▼──────┐  ┌──────▼──────┐
    │  Node A   │   │   Node B    │  │   Node C    │
    │ LN Wallet │   │  LN Wallet  │  │  LN Wallet  │
    └───────────┘   └─────────────┘  └─────────────┘
```

#### 4.4.2 Reward Calculation Formula

```
reward_sats(node, task) =
    base_rate_sats
    * tier_multiplier(node.tier)
    * task_weight(task.type)
    * quality_factor(task.result_score)
    * uptime_bonus(node.uptime_7d)

Where:
  base_rate_sats    = configurable (e.g., 10 sats per compute unit)
  tier_multiplier   = {T1: 1.0, T2: 2.5, T3: 8.0}
  task_weight       = {inference: 1.0, fl_round: 3.0, gradient_agg: 5.0}
  quality_factor    = clamp(result_score, 0.0, 1.0)
  uptime_bonus      = 1.0 + max(0, (uptime_7d - 0.9) * 5.0)
```

#### 4.4.3 Functional Requirements

| ID | Requirement |
|---|---|
| SRS-LN-01 | The payment subsystem shall connect to an LND or CLN node via gRPC/REST |
| SRS-LN-02 | Node registration shall require a valid LN node URI (pubkey@host:port) for receiving payments |
| SRS-LN-03 | The system shall attempt to open a payment channel to each new node upon registration if treasury balance permits |
| SRS-LN-04 | Reward payments shall be dispatched within 60 seconds of task completion proof receipt |
| SRS-LN-05 | All payment attempts, successes, and failures shall be logged with task ID, node ID, amount, and timestamp |
| SRS-LN-06 | If a direct channel does not exist, the system shall route payment via the LN gossip graph |
| SRS-LN-07 | The commercial API shall generate a BOLT11 Lightning invoice per request; access is granted upon payment confirmation |
| SRS-LN-08 | GitHub bounty payments shall be triggered by a webhook from the GitHub App upon PR merge |
| SRS-LN-09 | The treasury multisig wallet shall require 3-of-5 signatures for any on-chain transaction exceeding 0.01 BTC |
| SRS-LN-10 | The system shall expose a `/payments/history` API endpoint for node operators to audit their earnings |

---

### 4.5 OpenTimestamps Integration

**Language:** Python (`opentimestamps` library)
**Package:** `owm.provenance`

#### 4.5.1 Functional Requirements

| ID | Requirement |
|---|---|
| SRS-OTS-01 | After each FL round, the coordinator shall compute `SHA-256(model_weights_file)` and submit it to an OTS calendar server |
| SRS-OTS-02 | The system shall submit to at least 3 OTS calendar servers simultaneously for redundancy |
| SRS-OTS-03 | The project shall operate its own OTS calendar server as a fallback (`ots.owm.network`) |
| SRS-OTS-04 | OTS `.ots` proof files shall be stored alongside model checkpoint files in the model registry |
| SRS-OTS-05 | Nodes shall verify the OTS proof of a checkpoint before loading it: `ots verify checkpoint.ots` |
| SRS-OTS-06 | The coordinator shall expose an API endpoint `GET /model/versions` listing all checkpoints with their OTS proof status |
| SRS-OTS-07 | Version metadata shall include: checkpoint hash, OTS proof file path, Bitcoin block height of confirmation, round number, timestamp |

#### 4.5.2 Model Version Manifest Schema

```json
{
  "version": "owm-v0.4.2",
  "round": 142,
  "timestamp_utc": "2026-03-05T14:30:00Z",
  "sub_models": {
    "owm-code": {
      "sha256": "a3f9...",
      "ots_proof": "models/owm-code-v0.4.2.ots",
      "btc_block": 890421,
      "size_bytes": 14823049
    }
  },
  "coordinator_signature": "ed25519:base64...",
  "release_notes": "Improved code generation accuracy on Python 3.12 syntax"
}
```

---

### 4.6 Data Pipeline

**Language:** Python 3.11
**Package:** `owm.data`
**Storage:** S3-compatible object store (MinIO for self-hosted; S3 for production)

#### 4.6.1 Data Source Adapters

| Adapter | Source | Update Frequency | Format |
|---|---|---|---|
| `WikipediaAdapter` | Wikipedia dumps + live API | Weekly full + daily delta | XML/JSON |
| `OSMAdapter` | OpenStreetMap planet file | Monthly full + minutely diffs | PBF/JSON |
| `ArxivAdapter` | arXiv bulk API | Daily | PDF + metadata JSON |
| `GitHubAdapter` | GitHub API v4 (GraphQL) | Continuous | JSON + Git objects |
| `NewsAdapter` | Open RSS/Atom feeds, GDELT | Hourly | RSS/JSON |
| `WeatherAdapter` | Open-Meteo API, NOAA | Hourly | JSON |
| `SeismicAdapter` | USGS Earthquake API | Real-time streaming | GeoJSON |
| `FinanceAdapter` | Yahoo Finance, Alpha Vantage | Daily close + real-time | JSON/CSV |
| `ContributorAdapter` | Node operator submissions | On-demand | Any (validated) |

#### 4.6.2 Functional Requirements

| ID | Requirement |
|---|---|
| SRS-DATA-01 | Each adapter shall implement the `DataAdapter` interface: `fetch() -> Iterator[DataChunk]` and `validate(chunk) -> bool` |
| SRS-DATA-02 | All ingested data chunks shall be stored with source URL, ingestion timestamp, content hash, and license metadata |
| SRS-DATA-03 | Contributor-submitted data shall be quarantined and reviewed (automated quality scoring) before ingestion |
| SRS-DATA-04 | Contributor data reward = `base_rate * quality_score * usage_frequency`, paid via Lightning monthly |
| SRS-DATA-05 | PII detection shall run on all ingested text; chunks with PII shall be rejected or anonymized |
| SRS-DATA-06 | The pipeline shall support incremental updates: only new/changed data triggers re-embedding |
| SRS-DATA-07 | A vector store (Qdrant or Weaviate) shall index all ingested data for retrieval-augmented generation |
| SRS-DATA-08 | Data ingestion jobs shall be scheduled via the task scheduler and distributed to available nodes |

---

### 4.7 GitHub Integration Service

**Language:** Python 3.11
**Package:** `owm.github`
**Auth:** GitHub App (OAuth 2.0 device flow + installation tokens)

#### 4.7.1 Capabilities

```
┌─────────────────────────────────────────────┐
│           GitHub Integration Service         │
│                                             │
│  ┌───────────────┐   ┌───────────────────┐  │
│  │  Repo Analyzer │   │  Bounty Manager   │  │
│  │  - AST parsing │   │  - Issue watcher  │  │
│  │  - Lint/SAST   │   │  - PR merge hook  │  │
│  │  - Perf hints  │   │  - LN payment     │  │
│  └───────┬────────┘   └─────────┬─────────┘  │
│          │                      │            │
│  ┌───────▼────────┐   ┌─────────▼─────────┐  │
│  │  PR Generator  │   │  Security Auditor  │  │
│  │  - Diff gen    │   │  - CVE matching    │  │
│  │  - OWM code    │   │  - SAST rules      │  │
│  │    sub-model   │   │  - Dep vuln check  │  │
│  └────────────────┘   └───────────────────┘  │
└─────────────────────────────────────────────┘
```

#### 4.7.2 Functional Requirements

| ID | Requirement |
|---|---|
| SRS-GH-01 | The GitHub App shall be installable on any public or private GitHub repository |
| SRS-GH-02 | On installation, the service shall clone the repository and run a full analysis using `owm-code` sub-model |
| SRS-GH-03 | Analysis output shall include: code quality score, identified security issues (mapped to CVE IDs where applicable), performance suggestions |
| SRS-GH-04 | For each issue identified, the system shall generate a patch using the `owm-code` sub-model and submit a PR with a clear description |
| SRS-GH-05 | The bounty manager shall parse GitHub Issues labeled `owm-bounty: <sat_amount>` and register them in the bounty database |
| SRS-GH-06 | When a PR linked to a bounty issue is merged, the system shall automatically pay the PR author's registered LN address |
| SRS-GH-07 | The service shall expose a webhook endpoint `POST /github/webhook` to receive GitHub events |
| SRS-GH-08 | All PRs generated by OWM shall be clearly labeled as AI-generated with a disclaimer and link to verification |
| SRS-GH-09 | The security auditor shall check dependencies against OSV (Open Source Vulnerabilities) database on every push event |
| SRS-GH-10 | The OWM project repository itself shall use GitHub Actions for CI/CD with required status checks before merge |

#### 4.7.3 GitHub Actions Workflows

```yaml
# .github/workflows/ci.yml (required checks)
on: [push, pull_request]
jobs:
  lint:       # ruff, mypy, clippy
  test:       # pytest unit + integration
  security:   # bandit, cargo-audit, OSV check
  ots-verify: # verify OTS proof on model artifacts
  build:      # Docker image build
```

---

### 4.8 Public REST/WebSocket API

**Language:** Python 3.11 (FastAPI)
**Package:** `owm.api`

#### 4.8.1 Authentication

- **Non-commercial (free tier):** API key (rate limited: 100 req/day)
- **Commercial tier:** Lightning invoice per request OR monthly subscription Lightning payment
- **Node operators:** Authenticated via Ed25519 node identity key (exempt from rate limiting for node-management endpoints)

#### 4.8.2 API Endpoints

```
# Query API
POST   /v1/query                    # Send a query to the world model
POST   /v1/query/stream             # Streaming token generation (WebSocket)
GET    /v1/query/{query_id}         # Retrieve cached query result

# Model & Provenance
GET    /v1/model/versions           # List all model versions with OTS status
GET    /v1/model/versions/{id}      # Get specific version metadata
GET    /v1/model/versions/{id}/ots  # Download OTS proof file

# GitHub Integration
POST   /v1/github/analyze           # Submit a repo for analysis
GET    /v1/github/analysis/{id}     # Get analysis results
GET    /v1/github/bounties          # List active bounties
POST   /v1/github/bounties          # Create a new bounty

# Node Management (authenticated)
POST   /v1/nodes/register           # Register a new node
DELETE /v1/nodes/{node_id}          # Deregister a node
GET    /v1/nodes/{node_id}/status   # Get node status
GET    /v1/nodes/{node_id}/earnings # Get earnings history

# Network
GET    /v1/network/stats            # Network-wide statistics
GET    /v1/network/nodes            # List active nodes (anonymized)

# Payments
GET    /v1/payments/invoice         # Generate Lightning invoice for API access
GET    /v1/payments/history         # Node operator earnings history

# Governance
GET    /v1/governance/proposals     # List governance proposals
POST   /v1/governance/proposals     # Submit a new proposal (authenticated)
POST   /v1/governance/proposals/{id}/comment  # Comment on a proposal
```

#### 4.8.3 Query Request/Response Schema

```json
// POST /v1/query — Request
{
  "query": "What is the current seismic activity near the Cascadia Subduction Zone?",
  "context": {},
  "max_tokens": 1024,
  "stream": false
}

// POST /v1/query — Response
{
  "query_id": "qry_01J4K...",
  "answer": "As of March 5, 2026...",
  "sub_models_used": ["owm-geo", "owm-events"],
  "data_sources": ["USGS Earthquake API", "Wikipedia"],
  "confidence": 0.87,
  "model_version": "owm-v0.4.2",
  "ots_verified": true,
  "latency_ms": 312,
  "cost_sats": 5
}
```

#### 4.8.4 Functional Requirements

| ID | Requirement |
|---|---|
| SRS-API-01 | All API responses shall include `X-OWM-Model-Version` and `X-OWM-OTS-Verified` headers |
| SRS-API-02 | Rate limiting shall be enforced per API key using a sliding window algorithm |
| SRS-API-03 | Commercial API access shall be unlocked for 24 hours upon receipt of a valid Lightning payment |
| SRS-API-04 | The API shall return structured errors following RFC 7807 (Problem Details for HTTP APIs) |
| SRS-API-05 | Streaming responses shall use Server-Sent Events (SSE) over HTTPS |
| SRS-API-06 | The API shall be versioned; breaking changes require a new version prefix (`/v2/`) |

---

### 4.9 Governance Portal

**Language:** Python 3.11 (FastAPI backend) + plain HTML/HTMX frontend (no heavy JS framework)
**Package:** `owm.governance`

#### 4.9.1 Functional Requirements

| ID | Requirement |
|---|---|
| SRS-GOV-01 | The portal shall display all open governance proposals with their comment period deadline |
| SRS-GOV-02 | Any authenticated user (node operator or GitHub contributor) may submit a proposal |
| SRS-GOV-03 | Proposals shall specify: title, description, type (technical / treasury / policy), requested amount (if treasury), implementation plan |
| SRS-GOV-04 | The comment period shall default to 7 days; core maintainers may extend to 21 days for critical changes |
| SRS-GOV-05 | Treasury proposals exceeding 0.1 BTC shall require a 14-day comment period and all 5 multisig holder acknowledgments before signing |
| SRS-GOV-06 | The portal shall link to all corresponding Bitcoin transactions for executed treasury proposals |

---

### 4.10 Coordinator Service (Bootstrap Layer)

**Language:** Go 1.22
**Package:** `owm-coordinator`

#### 4.10.1 Functional Requirements

| ID | Requirement |
|---|---|
| SRS-COORD-01 | The coordinator shall maintain a live registry of all active nodes, their tiers, and capability manifests |
| SRS-COORD-02 | The coordinator shall expose a gRPC service for node registration, heartbeat, and task dispatch |
| SRS-COORD-03 | The coordinator shall run FL round orchestration: round initiation, participant selection, gradient collection, aggregation triggering |
| SRS-COORD-04 | In Phase 2, the coordinator shall migrate node discovery to a Kademlia DHT, reducing central dependency |
| SRS-COORD-05 | The coordinator shall be horizontally scalable behind a load balancer |
| SRS-COORD-06 | The coordinator shall store state in a PostgreSQL database with WAL replication |

---

## 5. Data Models

### 5.1 Node

```python
@dataclass
class Node:
    node_id: str                    # UUID v4
    public_key: str                 # Ed25519 public key (hex)
    ln_node_uri: str                # pubkey@host:port
    tier: Literal["t1", "t2", "t3"]
    vram_gb: float
    ram_gb: float
    bandwidth_mbps: float
    registered_at: datetime
    last_heartbeat: datetime
    reliability_score: float        # 0.0 – 1.0
    total_tasks_completed: int
    total_sats_earned: int
    status: Literal["online", "offline", "suspended"]
```

### 5.2 Task

```python
@dataclass
class Task:
    task_id: str                    # UUID v4
    task_type: TaskType
    assigned_node_id: str
    submitted_at: datetime
    started_at: Optional[datetime]
    completed_at: Optional[datetime]
    timeout_seconds: int
    input_hash: str                 # SHA-256 of task input
    output_hash: Optional[str]      # SHA-256 of task output
    reward_sats: int
    payment_status: Literal["pending", "paid", "failed"]
    payment_preimage: Optional[str] # LN payment proof
```

### 5.3 ModelVersion

```python
@dataclass
class ModelVersion:
    version_id: str                 # e.g., "owm-v0.4.2"
    round_number: int
    created_at: datetime
    sub_model_hashes: Dict[str, str]  # sub_model_id -> SHA-256
    ots_proof_paths: Dict[str, str]   # sub_model_id -> .ots file path
    btc_block_confirmed: Optional[int]
    coordinator_signature: str
    is_current: bool
```

### 5.4 Bounty

```python
@dataclass
class Bounty:
    bounty_id: str
    github_issue_url: str
    github_repo: str
    amount_sats: int
    creator_node_id: str
    status: Literal["open", "claimed", "paid", "expired"]
    created_at: datetime
    claimed_by_github_login: Optional[str]
    merged_pr_url: Optional[str]
    paid_at: Optional[datetime]
    payment_preimage: Optional[str]
```

---

## 6. API Specifications

### 6.1 Node Daemon gRPC Interface

```protobuf
syntax = "proto3";
package owm.coordinator.v1;

service CoordinatorService {
  rpc RegisterNode (RegisterNodeRequest) returns (RegisterNodeResponse);
  rpc Heartbeat (HeartbeatRequest) returns (HeartbeatResponse);
  rpc StreamTasks (HeartbeatRequest) returns (stream Task);
  rpc SubmitTaskResult (TaskResult) returns (TaskResultAck);
  rpc GetModelVersion (GetModelVersionRequest) returns (ModelVersion);
}

message RegisterNodeRequest {
  string public_key = 1;
  string ln_node_uri = 2;
  NodeCapabilities capabilities = 3;
  bytes signature = 4;  // Ed25519 signature over canonical request fields
}

message NodeCapabilities {
  string tier = 1;
  float vram_gb = 2;
  float ram_gb = 3;
  float bandwidth_mbps = 4;
  repeated string supported_task_types = 5;
}

message Task {
  string task_id = 1;
  string task_type = 2;
  bytes payload = 3;           // Encrypted task input
  int32 timeout_seconds = 4;
  int64 reward_sats = 5;
}

message TaskResult {
  string task_id = 1;
  string node_id = 2;
  bytes output_hash = 3;       // SHA-256 of output
  bytes output = 4;            // Task output (e.g., gradient delta)
  int64 execution_ms = 5;
  bytes signature = 6;         // Ed25519 signature over task_id + output_hash
}
```

---

## 7. Security Requirements

| ID | Requirement |
|---|---|
| SRS-SEC-01 | All inter-node and node-coordinator communication shall use mTLS with Ed25519 certificates |
| SRS-SEC-02 | Task inputs and outputs shall be encrypted with the recipient's public key (NaCl box encryption) |
| SRS-SEC-03 | The node identity keypair shall never leave the node; all authentication is via challenge-response signatures |
| SRS-SEC-04 | The coordinator shall rate-limit registration attempts: max 10 new nodes per IP per hour |
| SRS-SEC-05 | Byzantine fault-tolerant gradient aggregation shall be applied in all FL rounds (Krum or trimmed mean) |
| SRS-SEC-06 | All API inputs shall be validated and sanitized; parameterized queries shall be used for all database operations |
| SRS-SEC-07 | Secrets (LND macaroon, multisig keys) shall never appear in logs or error messages |
| SRS-SEC-08 | The GitHub App shall use short-lived installation tokens (1-hour TTL) and store no long-lived credentials |
| SRS-SEC-09 | Security advisories shall be reported via a responsible disclosure policy documented in `SECURITY.md` |
| SRS-SEC-10 | A CVE scanning CI step shall run on every PR using `pip-audit`, `cargo-audit`, and `trivy` |
| SRS-SEC-11 | Nodes flagged for malicious behavior (poisoned gradients, fake task proofs) shall be suspended and their reputation score set to 0 |
| SRS-SEC-12 | The multisig treasury shall use hardware wallets (Coldcard or Trezor) for all signing operations |

---

## 8. Performance Requirements

| ID | Requirement | Target |
|---|---|---|
| SRS-PERF-01 | API query end-to-end latency (p95) | < 2 seconds |
| SRS-PERF-02 | API query end-to-end latency (p99) | < 5 seconds |
| SRS-PERF-03 | Lightning payment dispatch latency after task completion | < 60 seconds |
| SRS-PERF-04 | Node heartbeat processing throughput | 10,000 nodes/minute |
| SRS-PERF-05 | FL round completion time (100 nodes) | < 30 minutes |
| SRS-PERF-06 | GitHub repo analysis (1,000-file repo) | < 5 minutes |
| SRS-PERF-07 | API uptime SLA (public endpoint) | 99.5% |
| SRS-PERF-08 | Maximum concurrent API requests | 1,000 |
| SRS-PERF-09 | Task scheduler dispatch latency | < 1 second |
| SRS-PERF-10 | OTS calendar submission acknowledgment | < 30 seconds |

---

## 9. Testing Requirements

### 9.1 Testing Framework

| Layer | Tool | Coverage Target |
|---|---|---|
| Unit tests | `pytest` | 85% line coverage |
| Integration tests | `pytest` + Docker Compose test environment | All happy paths + failure modes |
| Contract tests | `pact` (consumer-driven) | All inter-service gRPC/HTTP contracts |
| Load tests | `locust` | API: 1,000 concurrent users |
| Security tests | `bandit`, `semgrep`, `trivy` | 0 high-severity findings in main |
| Fuzz tests | `atheris` (Python) | gRPC input parsing, data adapters |
| FL adversarial tests | Custom test suite | Gradient poisoning detection |
| Lightning payment tests | LND testnet (regtest) | All payment flows |
| OTS tests | OTS test calendar | Proof generation and verification |

### 9.2 Key Test Scenarios

| ID | Scenario | Type |
|---|---|---|
| TST-01 | Node registers, completes inference task, receives Lightning payment | Integration |
| TST-02 | Malicious node submits poisoned gradient; system detects and rejects it | Security/FL |
| TST-03 | 30% of nodes drop during FL round; round completes with remaining nodes | Fault tolerance |
| TST-04 | OTS proof is tampered with; node refuses to load model weights | Security |
| TST-05 | Commercial API call: invoice generated, paid, query served | Integration |
| TST-06 | GitHub PR merged with bounty label; Lightning payment dispatched to contributor | Integration |
| TST-07 | API sustains 1,000 concurrent requests with p95 < 2s | Load |
| TST-08 | Node submits fake task completion proof; coordinator rejects it | Security |
| TST-09 | Contributor submits PII-containing dataset; pipeline rejects it | Data quality |
| TST-10 | Treasury proposal created, comment period elapses, multisig executes payment | Governance |

### 9.3 CI/CD Pipeline

```
git push → GitHub Actions:
  ├── lint (ruff, mypy, clippy, golangci-lint)
  ├── test:unit (pytest, cargo test, go test)
  ├── test:integration (docker-compose up + pytest)
  ├── security (bandit, cargo-audit, pip-audit, trivy)
  ├── build:docker (multi-arch: amd64, arm64)
  └── on tag:
       ├── test:load (locust)
       ├── ots:stamp (anchor release hash to OTS)
       └── release:publish (GitHub Release + Docker Hub)
```

---

## 10. Deployment & Infrastructure

### 10.1 Coordinator (Managed Infrastructure)

```yaml
# Minimum production coordinator deployment
Services:
  - coordinator-api:    2 replicas, 4 CPU, 8 GB RAM
  - scheduler:          2 replicas, 2 CPU, 4 GB RAM
  - postgres:           Primary + 1 read replica, 100 GB SSD
  - redis:              Rate limiting + task queue cache
  - minio:              Model artifact storage (100 GB initial)
  - ots-calendar:       OWM's own OTS calendar server
  - lnd:                Lightning Network Daemon (mainnet)
  - api-gateway:        nginx + TLS termination
  - governance-portal:  1 replica, 1 CPU, 2 GB RAM
```

### 10.2 Node Deployment (Operator Self-Hosted)

```bash
# One-command install
curl -fsSL https://install.owm.network | bash

# Or via Docker
docker run -d \
  --gpus all \
  -v ~/.owm:/root/.owm \
  -v ~/.lnd:/root/.lnd:ro \
  -p 9000:9000 \
  owmnetwork/owm-node:latest \
  start --config /root/.owm/owm.toml
```

### 10.3 Environment Configuration

| Variable | Description | Required |
|---|---|---|
| `OWM_COORDINATOR_URL` | Coordinator gRPC endpoint | Yes |
| `OWM_LND_HOST` | LND gRPC host:port | Yes |
| `OWM_LND_MACAROON_HEX` | LND admin macaroon (hex) | Yes |
| `OWM_LND_TLS_CERT_PATH` | Path to LND TLS cert | Yes |
| `OWM_IDENTITY_KEY_PATH` | Ed25519 node key path | Yes |
| `OWM_MODEL_CACHE_DIR` | Local model cache directory | No (default: `~/.owm/models`) |
| `OWM_LOG_LEVEL` | Logging verbosity | No (default: `INFO`) |
| `DATABASE_URL` | PostgreSQL connection string (coordinator only) | Coordinator only |

---

## 11. Documentation Requirements

| Deliverable | Location | Audience |
|---|---|---|
| Node setup guide | `docs/node-setup.md` | Node operators |
| API reference | `docs/api-reference.md` + auto-generated from OpenAPI spec | API consumers |
| Architecture deep dive | `docs/architecture.md` | Contributors |
| Federated learning protocol | `docs/fl-protocol.md` | Researchers / contributors |
| Lightning integration guide | `docs/lightning-integration.md` | Node operators |
| GitHub App setup | `docs/github-app.md` | Repo owners |
| Governance process | `docs/governance.md` | Community |
| Contributing guide | `CONTRIBUTING.md` | All contributors |
| Security policy | `SECURITY.md` | Security researchers |
| License | `LICENSE` | All users |
| Changelog | `CHANGELOG.md` | All users |
| OTS verification guide | `docs/ots-verification.md` | All users |

All public-facing Python code shall include:
- Module-level docstrings describing purpose.
- Type annotations on all function signatures.
- Inline comments for non-obvious logic only.

---

## 12. Technology Stack

| Layer | Technology | Version | Rationale |
|---|---|---|---|
| Node daemon | Python | 3.11+ | AI/ML ecosystem maturity |
| Performance-critical components | Rust | 1.75+ | Memory safety, LDK bindings |
| Coordinator | Go | 1.22+ | gRPC performance, concurrency |
| AI Framework | PyTorch | 2.x | Federated learning support, wide model compatibility |
| Quantization | `bitsandbytes`, `llama.cpp` | Latest | Tier 1 node support |
| LoRA fine-tuning | `peft` (Hugging Face) | Latest | Incremental model updates |
| Federated Learning | `Flower (flwr)` | 1.x | Mature FL framework, extensible |
| HTTP API | FastAPI | 0.110+ | Async, OpenAPI auto-gen |
| gRPC | `grpcio` / native Go | Latest | Node-coordinator protocol |
| Lightning | LND + `lnd-grpc` Python client | Latest LND | Widely deployed, robust |
| Lightning (Rust) | LDK | Latest | For embedded node support |
| Database | PostgreSQL | 16+ | Reliability, JSONB support |
| Cache / Queue | Redis | 7+ | Task queue, rate limiting |
| Object Storage | MinIO (self-hosted) / S3 | Latest | Model artifact storage |
| Vector Store | Qdrant | Latest | RAG data retrieval |
| OTS | `opentimestamps` Python library | Latest | Bitcoin anchoring |
| Container | Docker / Podman | 24+ / 4+ | Node portability |
| CI/CD | GitHub Actions | N/A | Community familiarity |
| Frontend | HTMX + Tailwind CSS | Latest | Minimal JS, fast governance portal |
| Monitoring | Prometheus + Grafana | Latest | Node and network metrics |
| Logging | structlog (Python) / zap (Go) | Latest | Structured JSON logs |

---

## 13. Directory Structure

```
Open-World-Model/
├── BRS.md                          # Business Requirements Specification
├── SRS.md                          # Software Requirements Specification
├── LICENSE                         # Source-available license
├── CONTRIBUTING.md
├── SECURITY.md
├── CHANGELOG.md
├── README.md
├── owm-node/                       # Node daemon (Python + Rust)
│   ├── src/
│   │   ├── owm/
│   │   │   ├── __init__.py
│   │   │   ├── node/               # Node daemon core
│   │   │   ├── model/              # Federated ensemble model engine
│   │   │   ├── lightning/          # LN payment integration
│   │   │   ├── provenance/         # OpenTimestamps integration
│   │   │   ├── data/               # Data pipeline & adapters
│   │   │   ├── github/             # GitHub integration service
│   │   │   └── api/                # Public REST/WebSocket API
│   ├── rust/                       # Rust extensions (LDK, perf-critical)
│   ├── tests/
│   │   ├── unit/
│   │   ├── integration/
│   │   └── adversarial/            # FL poisoning tests
│   ├── Dockerfile
│   ├── pyproject.toml
│   └── Cargo.toml
├── owm-coordinator/                # Coordinator service (Go)
│   ├── cmd/coordinator/
│   ├── internal/
│   │   ├── registry/               # Node registry
│   │   ├── scheduler/              # Task scheduler
│   │   ├── fl/                     # FL round orchestration
│   │   └── rpc/                    # gRPC server
│   ├── proto/                      # Protobuf definitions
│   ├── tests/
│   ├── Dockerfile
│   └── go.mod
├── owm-governance/                 # Governance portal (Python/HTMX)
│   ├── src/
│   ├── templates/
│   ├── static/
│   └── Dockerfile
├── deploy/                         # Deployment configs
│   ├── docker-compose.yml          # Local dev environment
│   ├── docker-compose.test.yml     # Integration test environment
│   ├── k8s/                        # Kubernetes manifests (coordinator)
│   └── scripts/
│       ├── install-node.sh         # One-command node installer
│       └── setup-treasury.sh       # Multisig treasury setup guide
├── docs/
│   ├── architecture.md
│   ├── node-setup.md
│   ├── api-reference.md
│   ├── fl-protocol.md
│   ├── lightning-integration.md
│   ├── github-app.md
│   ├── governance.md
│   └── ots-verification.md
└── .github/
    ├── workflows/
    │   ├── ci.yml
    │   └── release.yml
    └── ISSUE_TEMPLATE/
        ├── bug_report.md
        └── owm-bounty.md           # Bounty issue template
```

---

*End of Software Requirements Specification v1.0.0*
