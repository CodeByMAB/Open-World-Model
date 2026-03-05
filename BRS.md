# Business Requirements Specification (BRS)

**Project:** Open World Model (OWM)
**Version:** 1.0.0
**Date:** 2026-03-05
**Status:** Draft — Pending Stakeholder Review

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Business Context & Problem Statement](#2-business-context--problem-statement)
3. [Project Vision](#3-project-vision)
4. [Stakeholders](#4-stakeholders)
5. [Business Objectives](#5-business-objectives)
6. [Scope](#6-scope)
7. [Business Requirements](#7-business-requirements)
   - 7.1 [AI Model Requirements](#71-ai-model-requirements)
   - 7.2 [Decentralized Network Requirements](#72-decentralized-network-requirements)
   - 7.3 [Bitcoin & Lightning Integration Requirements](#73-bitcoin--lightning-integration-requirements)
   - 7.4 [GitHub Integration Requirements](#74-github-integration-requirements)
   - 7.5 [Data Requirements](#75-data-requirements)
   - 7.6 [Governance Requirements](#76-governance-requirements)
   - 7.7 [Licensing & Commercialization Requirements](#77-licensing--commercialization-requirements)
8. [Constraints & Assumptions](#8-constraints--assumptions)
9. [Risks & Mitigation Strategies](#9-risks--mitigation-strategies)
10. [Success Criteria & KPIs](#10-success-criteria--kpis)
11. [Milestones & Roadmap](#11-milestones--roadmap)
12. [Glossary](#12-glossary)

---

## 1. Executive Summary

The **Open World Model (OWM)** is an open-source, federated ensemble of AI models that collectively builds, maintains, and continuously updates a comprehensive model of the real world — encompassing geography, science, code, language, and live data streams. The system runs on a permissionless, decentralized network of GPU-equipped nodes. Node operators are compensated in Bitcoin via the Lightning Network for their compute and data contributions. Model versioning and integrity are anchored to the Bitcoin blockchain via **OpenTimestamps**. The project is well-versed in software engineering and Git, enabling it to autonomously analyze, improve, and generate pull requests for GitHub repositories. The treasury is controlled by a Bitcoin multisig wallet; technical governance follows a rough-consensus model.

---

## 2. Business Context & Problem Statement

### 2.1 Current State of World Models

Existing large-scale AI world models (e.g., large language models, knowledge graphs) are:

- **Centralized**: Controlled by a small number of corporations with opaque governance.
- **Expensive to run**: Requiring massive proprietary data centers inaccessible to independent contributors.
- **Stale**: Updated infrequently; disconnected from real-time world data.
- **Unincentivized for contributors**: Data and compute providers receive no direct financial reward.
- **Poorly audited**: Code quality and security vulnerabilities go unaddressed due to lack of automated tooling.

### 2.2 The Opportunity

Bitcoin's Lightning Network enables programmable, near-zero-fee micropayments globally. GPU hardware is increasingly distributed across individuals, businesses, and academic institutions. Open-source data ecosystems (Wikipedia, OpenStreetMap, arXiv, GitHub) provide a rich training corpus. The convergence of these three trends makes a community-owned, Bitcoin-incentivized world AI model economically viable for the first time.

### 2.3 Problem Statement

There is no open, continuously updated, decentralized AI world model that:
1. Compensates contributors fairly and automatically with real money (Bitcoin).
2. Allows anyone with a GPU to join and earn rewards.
3. Is demonstrably trustworthy via cryptographic model provenance (OpenTimestamps).
4. Is skilled at software engineering and actively improves open-source codebases.

---

## 3. Project Vision

> **"A living, open model of our world — built by everyone, owned by no one, powered by Bitcoin."**

The Open World Model will be the world's first community-owned, continuously updated AI that understands the planet — its geography, science, culture, code, and events — funded and incentivized entirely through the Bitcoin economy.

---

## 4. Stakeholders

| Stakeholder | Role | Key Interests |
|---|---|---|
| **Node Operators** | Contribute GPU compute and earn BTC | Fair compensation, low setup friction, stable rewards |
| **Data Contributors** | Provide proprietary or curated datasets | BTC rewards, data privacy controls, attribution |
| **Core Maintainers** | Architects and lead engineers | Technical integrity, long-term sustainability |
| **Open-Source Community** | GitHub contributors, bug reporters, reviewers | BTC bounties, transparent governance, code quality |
| **Commercial API Users** | Companies consuming OWM inference via API | Reliability, SLAs, feature richness |
| **Treasury Multisig Holders** | Control spending of Bitcoin reserves | Accountability, protocol health |
| **End Users** | Individuals querying the model for knowledge | Accuracy, response quality, accessibility |
| **Security Researchers** | Auditing the codebase and network | Responsible disclosure, bug bounties |

---

## 5. Business Objectives

| ID | Objective | Priority |
|---|---|---|
| BO-01 | Launch a production network with 100+ active GPU nodes within 12 months. | Critical |
| BO-02 | Establish a Bitcoin Lightning treasury funded by commercial API usage within 6 months. | Critical |
| BO-03 | Deliver a federated ensemble AI model capable of answering world-knowledge and coding questions. | Critical |
| BO-04 | Enable automated GitHub repo analysis, security audits, and PR generation via the AI. | High |
| BO-05 | Implement OpenTimestamps-based model version provenance on the Bitcoin blockchain. | High |
| BO-06 | Attract a community of 500+ registered GitHub contributors within 12 months. | High |
| BO-07 | Generate sufficient commercial API revenue to sustain treasury operations by month 9. | Medium |
| BO-08 | Achieve recognized open-source project status (e.g., GitHub Trending, academic citations). | Medium |

---

## 6. Scope

### 6.1 In Scope

- Federated ensemble AI model architecture (multiple specialized sub-models).
- Decentralized node network with tiered GPU participation.
- Bitcoin Lightning Network payment system for node rewards.
- OpenTimestamps integration for model version anchoring.
- GitHub integration: automated code analysis, PR generation, security auditing, Bitcoin bounty system.
- Multi-source data pipeline: open public data, code/developer knowledge, live feeds, contributor data.
- Bitcoin multisig treasury management.
- Source-available licensing with commercial license enforcement.
- Developer-facing REST/WebSocket API for inference.
- Community governance portal (rough consensus voting).
- Comprehensive documentation, testing framework, and CI/CD pipeline.

### 6.2 Out of Scope

- Bitcoin ordinals or inscriptions (explicitly excluded).
- Centralized cloud-only deployment.
- Proof-of-Work or Proof-of-Stake consensus mechanisms (no new token or altcoin).
- Consumer mobile applications (Phase 1).
- Non-Bitcoin payment rails (fiat, other cryptocurrencies) in Phase 1.

---

## 7. Business Requirements

### 7.1 AI Model Requirements

| ID | Requirement | Priority |
|---|---|---|
| BR-AI-01 | The system shall consist of a federated ensemble of specialized AI sub-models (e.g., geography, science, code, current events) that collaborate to answer queries. | Critical |
| BR-AI-02 | The ensemble shall support multimodal inputs: text, structured data, geographic coordinates, and time-series data. | High |
| BR-AI-03 | The model shall be continuously updated as new data is ingested, without requiring full retraining. | Critical |
| BR-AI-04 | The AI shall have deep expertise in software engineering, including code generation, review, refactoring, and Git operations. | Critical |
| BR-AI-05 | Model inference shall be distributable across multiple nodes to reduce single-node load. | High |
| BR-AI-06 | All model weight versions shall be cryptographically signed and timestamped via OpenTimestamps before distribution. | Critical |
| BR-AI-07 | The system shall support federated learning: nodes train on local data and contribute gradient updates without sharing raw data. | High |

### 7.2 Decentralized Network Requirements

| ID | Requirement | Priority |
|---|---|---|
| BR-NET-01 | Any operator with a qualifying GPU may join the network as a node without requiring permission from a central authority. | Critical |
| BR-NET-02 | Node participation shall be tiered by hardware capability: Tier 1 (consumer GPU), Tier 2 (prosumer/workstation GPU), Tier 3 (server-grade multi-GPU). | Critical |
| BR-NET-03 | Each tier shall have defined minimum VRAM, bandwidth, and uptime requirements. | Critical |
| BR-NET-04 | The network shall support all four compute paradigms: federated learning, distributed task scheduling, decentralized compute marketplace job dispatch, and a hybrid centralized-coordinator mode for bootstrapping. | High |
| BR-NET-05 | Node health and contribution metrics (compute hours, tasks completed, uptime) shall be verifiably tracked. | Critical |
| BR-NET-06 | The network shall tolerate node dropout without loss of service; redundancy targets shall be defined per tier. | High |
| BR-NET-07 | Task routing shall optimize for latency, node capability, and cost efficiency. | High |

### 7.3 Bitcoin & Lightning Integration Requirements

| ID | Requirement | Priority |
|---|---|---|
| BR-BTC-01 | Node operators shall be compensated in Bitcoin satoshis via Lightning Network micropayments based on verified compute contributions. | Critical |
| BR-BTC-02 | The project shall maintain a Bitcoin multisig treasury (minimum 3-of-5 signers) funded by commercial API revenue. | Critical |
| BR-BTC-03 | Lightning payment channels shall be established between the treasury node and all active contributor nodes. | Critical |
| BR-BTC-04 | Payment calculations shall be transparent, deterministic, and auditable by any network participant. | Critical |
| BR-BTC-05 | Model versions shall be anchored to the Bitcoin blockchain via OpenTimestamps (OTS) using SHA-256 digests of model weight checksums. | Critical |
| BR-BTC-06 | GitHub contributors with merged PRs or accepted bounties shall receive Bitcoin Lightning payments automatically. | High |
| BR-BTC-07 | The system shall not use Bitcoin Ordinals, Inscriptions, or any protocol that writes large data to the blockchain. | Critical (Exclusion) |
| BR-BTC-08 | All Lightning payment logic shall be implemented using established open-source Lightning libraries (e.g., LDK, CLN, LND). | High |

### 7.4 GitHub Integration Requirements

| ID | Requirement | Priority |
|---|---|---|
| BR-GH-01 | The AI shall autonomously analyze GitHub repositories for code quality, performance bottlenecks, and security vulnerabilities. | Critical |
| BR-GH-02 | The AI shall generate and submit pull requests with suggested improvements to external repositories. | High |
| BR-GH-03 | The project's own GitHub repository shall support open community contributions via a documented PR and code review process. | Critical |
| BR-GH-04 | A Bitcoin bounty board shall be maintained on GitHub Issues, with bounties automatically paid via Lightning when PRs are merged. | High |
| BR-GH-05 | The AI shall conduct automated security audits and flag CVEs or vulnerability patterns in monitored repositories. | High |
| BR-GH-06 | GitHub Actions CI/CD shall enforce code quality, run tests, and validate OTS timestamps on every release. | High |

### 7.5 Data Requirements

| ID | Requirement | Priority |
|---|---|---|
| BR-DATA-01 | The system shall ingest open public datasets: Wikipedia, OpenStreetMap, arXiv, open news feeds, and permissively licensed repositories. | Critical |
| BR-DATA-02 | The system shall ingest code and developer knowledge: GitHub public repositories, Stack Overflow, official documentation. | Critical |
| BR-DATA-03 | Node operators and third parties shall be able to contribute proprietary datasets; contributors receive BTC rewards proportional to dataset quality and usage. | High |
| BR-DATA-04 | The system shall integrate live real-world data feeds: weather APIs, seismic sensors, financial market data, satellite imagery. | High |
| BR-DATA-05 | All data provenance shall be recorded; personally identifiable information (PII) shall not be ingested without explicit consent. | Critical |
| BR-DATA-06 | Federated learning shall allow nodes to train on local private data without transmitting raw data off-node. | High |

### 7.6 Governance Requirements

| ID | Requirement | Priority |
|---|---|---|
| BR-GOV-01 | The project treasury shall be controlled by a Bitcoin multisig wallet with a minimum of 3-of-5 key holders. | Critical |
| BR-GOV-02 | Treasury spending proposals shall be published publicly and subject to a rough-consensus comment period (minimum 7 days) before execution. | High |
| BR-GOV-03 | Technical decisions (protocol changes, model architecture) shall follow a documented rough-consensus process (similar to Bitcoin BIPs). | High |
| BR-GOV-04 | All governance decisions and treasury transactions shall be publicly auditable. | Critical |
| BR-GOV-05 | Key holders shall be publicly identified or pseudonymously accountable via signed Bitcoin addresses. | High |

### 7.7 Licensing & Commercialization Requirements

| ID | Requirement | Priority |
|---|---|---|
| BR-LIC-01 | The codebase and model weights shall be source-available: free for non-commercial use; commercial use requires a paid license. | Critical |
| BR-LIC-02 | Commercial license fees shall be deposited directly into the Bitcoin multisig treasury. | Critical |
| BR-LIC-03 | A developer/research license tier (free for academic and non-profit use) shall be available. | High |
| BR-LIC-04 | Commercial API access shall be gated by Lightning invoice payment per request or subscription. | High |
| BR-LIC-05 | The license shall require attribution and prohibit using the model to undermine the Bitcoin network. | Medium |

---

## 8. Constraints & Assumptions

### 8.1 Constraints

| ID | Constraint |
|---|---|
| CON-01 | All incentive payments must use Bitcoin (Lightning Network) only — no other cryptocurrencies or fiat. |
| CON-02 | Model integrity anchoring must use OpenTimestamps only — no Ordinals or Inscriptions. |
| CON-03 | No user PII may be stored on-chain or in the model. |
| CON-04 | All node communication must be encrypted end-to-end. |
| CON-05 | The project must be deployable by a single operator with a single GPU to ensure permissionless entry. |

### 8.2 Assumptions

| ID | Assumption |
|---|---|
| ASS-01 | Bitcoin Lightning Network will remain operational and continue to offer sub-cent micropayments. |
| ASS-02 | OpenTimestamps Bitcoin calendar servers will remain operational; the system will run its own OTS calendar as backup. |
| ASS-03 | Open dataset licensing (Wikipedia CC-BY-SA, OSM ODbL) permits use for AI training under the chosen project license. |
| ASS-04 | The initial core team has access to at least 5 server-grade GPU nodes for bootstrapping. |
| ASS-05 | A Lightning node (LND or CLN) will be operated by the treasury to manage payments. |

---

## 9. Risks & Mitigation Strategies

| ID | Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|---|
| RSK-01 | Lightning Network channel liquidity dries up, blocking payments | Medium | Critical | Maintain liquidity reserves; use multiple routing nodes; monitor channel health |
| RSK-02 | Insufficient node operators join the network | Medium | Critical | Aggressive community outreach; competitive BTC rewards; simple one-command node setup |
| RSK-03 | Malicious nodes submit poisoned gradients in federated learning | High | High | Byzantine fault-tolerant aggregation; node reputation scoring; gradient anomaly detection |
| RSK-04 | Commercial API revenue insufficient to sustain treasury | Medium | High | Dual-track: API revenue + direct community donations; minimize fixed costs |
| RSK-05 | Regulatory action against Bitcoin-based compute rewards | Low | High | Legal review in target jurisdictions; compute-reward framing as B2B service payment |
| RSK-06 | Model generates harmful or inaccurate outputs | Medium | High | Output filtering, RLHF, community flagging system, red-teaming program |
| RSK-07 | Multisig key holder collusion or loss of keys | Low | Critical | Documented key ceremony; geographically distributed holders; hardware wallets required |
| RSK-08 | Open-source contributors reverse-engineer and reuse commercially without license | Medium | Medium | License monitoring tools; community enforcement; DMCA process |

---

## 10. Success Criteria & KPIs

| KPI | Target (Month 6) | Target (Month 12) |
|---|---|---|
| Active GPU nodes | 25+ | 100+ |
| Cumulative compute hours contributed | 10,000 hours | 100,000 hours |
| GitHub contributors (PRs merged) | 50 | 500+ |
| Bitcoin treasury balance | 0.5 BTC | 5 BTC |
| Model accuracy (world knowledge benchmark) | Baseline established | 10% improvement over baseline |
| API requests served per day | 1,000 | 50,000 |
| GitHub repos analyzed / PRs submitted | 100 | 2,000+ |
| OpenTimestamps model versions anchored | 10 | 100+ |
| Commercial API licenses sold | 5 | 50+ |
| Lightning payments dispatched | 10,000 | 1,000,000+ |

---

## 11. Milestones & Roadmap

### Phase 1 — Foundation (Months 1–3)

- [ ] Repository structure, licensing, documentation framework
- [ ] Core node daemon (registration, heartbeat, task receipt)
- [ ] Local federated model prototype (2–3 sub-models)
- [ ] Lightning wallet integration and test payments on testnet
- [ ] OpenTimestamps integration for model checkpoints
- [ ] GitHub App: repo analysis and security audit MVP
- [ ] Treasury multisig wallet setup (5 key holders)

### Phase 2 — Beta Network (Months 4–6)

- [ ] Public testnet launch: 25-node target
- [ ] Tiered node hardware validation and reward calibration
- [ ] Mainnet Lightning payments enabled
- [ ] Data pipeline: open datasets + live feeds ingested
- [ ] GitHub bounty board live
- [ ] Commercial API v1 with Lightning invoice gating
- [ ] Community governance portal (rough consensus process)

### Phase 3 — Production Launch (Months 7–9)

- [ ] Mainnet launch: 50-node target
- [ ] Federated ensemble expanded to full sub-model suite
- [ ] Contributor private data marketplace
- [ ] Automated PR generation for external GitHub repos
- [ ] Security audit by external firm
- [ ] Source-available license v1 published

### Phase 4 — Scale & Sustainability (Months 10–12)

- [ ] 100+ node target
- [ ] Academic and research licensing program
- [ ] Model quality benchmark publication
- [ ] First treasury spending proposals and rough consensus votes
- [ ] Performance optimization and latency improvements
- [ ] Roadmap v2 community planning

---

## 12. Glossary

| Term | Definition |
|---|---|
| **OWM** | Open World Model — the project name |
| **Node** | A machine operated by a contributor that provides GPU compute to the OWM network |
| **Federated Ensemble** | A collection of specialized AI sub-models that collaborate to produce outputs, where each sub-model may be trained on different nodes |
| **Federated Learning** | A machine learning approach where model training occurs locally on nodes; only gradient updates (not raw data) are shared |
| **Lightning Network** | A Layer 2 Bitcoin payment protocol enabling instant, low-fee micropayments via payment channels |
| **OpenTimestamps (OTS)** | A protocol for anchoring cryptographic hashes to the Bitcoin blockchain as a proof-of-existence timestamp |
| **Bitcoin Multisig** | A Bitcoin wallet requiring M-of-N private key signatures to authorize a transaction (OWM uses 3-of-5) |
| **Rough Consensus** | A governance approach (used by IETF and Bitcoin) where decisions proceed when no significant objection exists, without requiring unanimity |
| **Satoshi (sat)** | The smallest unit of Bitcoin: 1 BTC = 100,000,000 satoshis |
| **OTS Calendar** | A server that aggregates hash submissions and periodically anchors them to the Bitcoin blockchain |
| **Gradient Update** | The mathematical update signal derived from training a model on data; used in federated learning to update global model weights |
| **Source-Available** | A licensing model where source code is publicly visible but commercial use requires a paid license |
| **BIP** | Bitcoin Improvement Proposal — a design document for proposed Bitcoin protocol changes; used here as governance model analogy |
| **CVE** | Common Vulnerabilities and Exposures — a public list of cybersecurity vulnerabilities |
| **VRAM** | Video RAM — memory on a GPU, a key bottleneck for running AI models |

---

*End of Business Requirements Specification v1.0.0*
