# Business Requirements Specification (BRS)

**Project:** Open World Model (OWM)
**Version:** 1.1.0
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
   - 7.8 [Proof-of-Work: Bitcoin Mining Pool Requirements](#78-proof-of-work-bitcoin-mining-pool-requirements)
   - 7.9 [Proof-of-Stake: Lightning Channel Stake Requirements](#79-proof-of-stake-lightning-channel-stake-requirements)
8. [Constraints & Assumptions](#8-constraints--assumptions)
9. [Risks & Mitigation Strategies](#9-risks--mitigation-strategies)
10. [Success Criteria & KPIs](#10-success-criteria--kpis)
11. [Milestones & Roadmap](#11-milestones--roadmap)
12. [Glossary](#12-glossary)

---

## 1. Executive Summary

The **Open World Model (OWM)** is an open-source, federated ensemble of AI models that collectively builds, maintains, and continuously updates a comprehensive model of the real world — encompassing geography, science, code, language, and live data streams. The system runs on a permissionless, decentralized network of GPU-equipped nodes. Node operators are compensated in Bitcoin via the Lightning Network for their compute and data contributions. Model versioning and integrity are anchored to the Bitcoin blockchain via **OpenTimestamps**. The project is well-versed in software engineering and Git, enabling it to autonomously analyze, improve, and generate pull requests for GitHub repositories. The treasury is controlled by a Bitcoin multisig wallet; technical governance follows a rough-consensus model.

Node participation is secured by two complementary Bitcoin-native mechanisms: **Proof-of-Work (PoW)** — nodes may optionally contribute hashrate to the OWM-operated Stratum v2 Bitcoin mining pool, with mining revenue split between the miner and the treasury; and **Proof-of-Stake (PoS)** — all joining nodes must open a Lightning payment channel to the treasury of a minimum size determined by their hardware tier, locking in a financial commitment that can be force-closed upon verified misbehavior.

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
5. Uses Bitcoin-native PoW (mining) and PoS (Lightning channel stake) to align node incentives without introducing any new token.

---

## 3. Project Vision

> **"A living, open model of our world — built by everyone, owned by no one, powered by Bitcoin."**

The Open World Model will be the world's first community-owned, continuously updated AI that understands the planet — its geography, science, culture, code, and events — funded and incentivized entirely through the Bitcoin economy.

---

## 4. Stakeholders

| Stakeholder | Role | Key Interests |
|---|---|---|
| **Node Operators** | Contribute GPU compute and earn BTC | Fair compensation, low setup friction, stable rewards |
| **Mining Node Operators** | Contribute hashrate to OWM mining pool and earn BTC | Competitive pool rewards, transparent payout splits, low pool fees |
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
| BO-09 | Launch the OWM Stratum v2 Bitcoin mining pool and onboard at least 50 mining nodes within 12 months. | High |
| BO-10 | Establish Lightning channel stake requirements for all node tiers, ensuring every active node has skin-in-the-game by month 6. | Critical |

---

## 6. Scope

### 6.1 In Scope

- Federated ensemble AI model architecture (multiple specialized sub-models).
- Decentralized node network with tiered GPU participation.
- Bitcoin Lightning Network payment system for node rewards.
- OWM-operated Stratum v2 Bitcoin mining pool with PoW contribution rewards.
- Lightning channel stake (PoS) requirement for all joining nodes, with force-close slashing on misbehavior.
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
- Any new token or altcoin for PoW/PoS — all mechanisms use native Bitcoin only.
- Consumer mobile applications (Phase 1).
- Non-Bitcoin payment rails (fiat, other cryptocurrencies) in Phase 1.
- Solo mining without pool coordination (nodes mine via the OWM pool only).

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
| BR-NET-08 | Every joining node shall open a Lightning payment channel to the treasury node as a mandatory Proof-of-Stake commitment before receiving tasks. | Critical |
| BR-NET-09 | Nodes may optionally contribute hashrate to the OWM Bitcoin mining pool as a Proof-of-Work contribution, earning additional BTC rewards. | High |
| BR-NET-10 | A node's mining hashrate contribution shall be recorded as a positive reputation signal alongside compute metrics. | Medium |

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
| BR-BTC-09 | Node registration shall be blocked until the registering node's Lightning channel stake to the treasury meets the minimum for their declared tier. | Critical |
| BR-BTC-10 | Upon verified misbehavior (poisoned gradients, falsified task proofs), the treasury shall initiate a force-close of the offending node's channel, and the node shall be suspended with a mandatory re-stake cooldown period before re-admission. | Critical |
| BR-BTC-11 | The OWM mining pool shall operate on the Stratum v2 protocol; pool block rewards shall be split 80% to the contributing miner and 20% to the treasury via Lightning. | High |
| BR-BTC-12 | Mining pool payouts shall be dispatched via Lightning within 1 hour of block confirmation. | High |
| BR-BTC-13 | The mining pool shall publish real-time hashrate, share difficulty, and payout history on a public dashboard. | Medium |

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

### 7.8 Proof-of-Work: Bitcoin Mining Pool Requirements

The OWM mining pool gives nodes an additional Bitcoin income stream while funding the treasury, anchoring the project's economic value directly to Bitcoin's proof-of-work security.

| ID | Requirement | Priority |
|---|---|---|
| BR-POW-01 | OWM shall operate a Bitcoin mining pool using the Stratum v2 protocol with end-to-end encryption between pool and miners. | Critical |
| BR-POW-02 | Any OWM node (Tier 1, 2, or 3) with a GPU or connected ASIC may point hashrate at the OWM pool. | High |
| BR-POW-03 | Pool rewards shall be split: 80% to the contributing mining node (paid via Lightning), 20% to the treasury. | Critical |
| BR-POW-04 | The pool shall support FPPS (Full Pay Per Share) payout method to give miners predictable income regardless of variance. | High |
| BR-POW-05 | Mining pool payouts shall be dispatched via Lightning within 1 hour of each Bitcoin block confirmation. | High |
| BR-POW-06 | The pool shall publish a live public dashboard showing: total hashrate, connected miners, pool luck, recent blocks found, and per-node share history. | Medium |
| BR-POW-07 | Mining nodes shall identify themselves to the pool using their OWM node identity key, linking mining contribution to their node reputation score. | High |
| BR-POW-08 | The pool coordinator shall be operated by core maintainers initially; decentralization via a federated pool model is a Phase 4 goal. | Medium |

### 7.9 Proof-of-Stake: Lightning Channel Stake Requirements

The Lightning channel stake acts as OWM's native Proof-of-Stake mechanism — entirely Bitcoin-native, requiring no new token. By locking sats in a channel to the treasury, nodes demonstrate financial commitment and face a credible economic penalty for misbehavior (force-close), creating a powerful Sybil-resistance and alignment mechanism.

| ID | Requirement | Priority |
|---|---|---|
| BR-POS-01 | Every node must open a Lightning payment channel **to the treasury node** with a minimum capacity before they are admitted to the network. | Critical |
| BR-POS-02 | Minimum channel stake shall be tiered by hardware: Tier 1 = 100,000 sats; Tier 2 = 500,000 sats; Tier 3 = 2,000,000 sats. | Critical |
| BR-POS-03 | Nodes with higher stake than the tier minimum shall receive a proportional stake bonus multiplier on compute rewards (capped at 2x). | High |
| BR-POS-04 | The channel opened by the node counts as the node's stake; the treasury does not push funds back on opening — the node bears the full channel cost. | Critical |
| BR-POS-05 | Upon detection and confirmation of malicious behavior, the treasury shall force-close the offending node's channel, recovering time-locked funds per Lightning protocol rules. | Critical |
| BR-POS-06 | A suspended node must wait a minimum cooldown period (30 days) before re-registering and opening a new stake channel. | High |
| BR-POS-07 | Nodes that voluntarily exit (cooperative channel close) shall have their funds returned normally; no penalty applies for graceful departure. | Critical |
| BR-POS-08 | The stake requirement shall be enforced at the protocol level: the coordinator shall verify the channel exists and meets capacity minimums via the LND/CLN API before granting active status. | Critical |
| BR-POS-09 | The treasury shall not use staked channel funds for treasury spending — they remain locked in channels as economic security. | Critical |

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
| CON-06 | PoW and PoS mechanisms must use native Bitcoin only — no new token, no altcoin, no wrapped asset. |
| CON-07 | Mining pool must use Stratum v2; legacy Stratum v1 is not supported due to security and efficiency concerns. |
| CON-08 | Force-close of a node's stake channel requires coordinator confirmation and is irreversible; it must only be triggered after verified misbehavior. |

### 8.2 Assumptions

| ID | Assumption |
|---|---|
| ASMP-01 | Bitcoin Lightning Network will remain operational and continue to offer sub-cent micropayments. |
| ASMP-02 | OpenTimestamps Bitcoin calendar servers will remain operational; the system will run its own OTS calendar as backup. |
| ASMP-03 | Open dataset licensing (Wikipedia CC-BY-SA, OSM ODbL) permits use for AI training under the chosen project license. |
| ASMP-04 | The initial core team has access to at least 5 server-grade GPU nodes for bootstrapping. |
| ASMP-05 | A Lightning node (LND or CLN) will be operated by the treasury to manage payments. |
| ASMP-06 | Stratum v2 pool software (e.g., SRI — Stratum Reference Implementation) is mature enough for production use by Phase 2. |
| ASMP-07 | Node operators joining at Tier 1 have access to at least 100,000 sats (≈ $50–$100 at time of writing) to open their stake channel. |
| ASMP-08 | Force-close mechanics in Lightning are well-understood and reliably punish the counterparty who broadcasts an outdated state. |
| ASMP-09 | Operators may run either Bitcoin Core or Bitcoin Knots as their full node; both expose the same getblocktemplate RPC interface required by the OWM mining pool. |

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
| RSK-09 | Mining pool finds no blocks for extended periods, discouraging miner participation | Medium | Medium | FPPS payout model eliminates variance for miners; treasury absorbs luck risk |
| RSK-10 | Stake channel force-close triggered incorrectly (false positive on misbehavior detection) | Low | High | Multi-signal detection required before force-close; human review step for Tier 2/3 nodes; appeals process |
| RSK-11 | Node operators unable to afford minimum stake, creating a financial barrier to entry | Medium | Medium | Tier 1 minimum set conservatively (100k sats); consider a "staking loan" from treasury for trusted early adopters |
| RSK-12 | Mining pool becomes a centralization point for Bitcoin hashrate | Low | Medium | Pool is open-source; federated pool architecture in Phase 4; no minimum hashrate enforced |

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
| Mining pool hashrate (PH/s) | 0.01 | 1.0+ |
| Active mining nodes | 10 | 50+ |
| Nodes with active stake channels | 25 | 100+ |
| Total sats staked in treasury channels | 5,000,000 | 50,000,000+ |
| Treasury revenue from mining pool (20% cut) | 0.05 BTC | 0.5 BTC |
| Successful slashing events (false positives: 0) | — | Tracked |

---

## 11. Milestones & Roadmap

### Phase 1 — Foundation (Months 1–3)

- [ ] Repository structure, licensing, documentation framework
- [ ] Core node daemon (registration, heartbeat, task receipt)
- [ ] Local federated model prototype (2–3 sub-models)
- [ ] Lightning wallet integration and test payments on testnet
- [ ] Lightning channel stake enforcement: coordinator validates channel before node activation (testnet)
- [ ] OpenTimestamps integration for model checkpoints
- [ ] GitHub App: repo analysis and security audit MVP
- [ ] Treasury multisig wallet setup (5 key holders)

### Phase 2 — Beta Network (Months 4–6)

- [ ] Public testnet launch: 25-node target
- [ ] Tiered node hardware validation and reward calibration
- [ ] Mainnet Lightning payments enabled
- [ ] Lightning channel stake enforced on mainnet for all joining nodes
- [ ] OWM Stratum v2 mining pool beta launch (testnet Bitcoin)
- [ ] Data pipeline: open datasets + live feeds ingested
- [ ] GitHub bounty board live
- [ ] Commercial API v1 with Lightning invoice gating
- [ ] Community governance portal (rough consensus process)

### Phase 3 — Production Launch (Months 7–9)

- [ ] Mainnet launch: 50-node target (all with active stake channels)
- [ ] OWM mining pool mainnet launch with FPPS payouts via Lightning
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
| **Proof-of-Work (PoW)** | In OWM context: contributing hashrate to the OWM Bitcoin mining pool as a measurable, costly commitment to the network |
| **Proof-of-Stake (PoS)** | In OWM context: opening a Lightning payment channel to the treasury with a minimum sat capacity, locking Bitcoin as a financial commitment and Sybil-resistance mechanism — no new token involved |
| **Stratum v2** | The modern Bitcoin mining pool protocol; encrypted, more efficient, and more miner-privacy-preserving than Stratum v1 |
| **FPPS** | Full Pay Per Share — a mining pool payout method where miners earn a fixed share per submitted share regardless of whether the pool finds a block; the pool absorbs variance |
| **Force-Close** | A unilateral Lightning channel closure initiated by one party, broadcasting the latest commitment transaction to the blockchain; used as OWM's slashing mechanism |
| **Slashing** | The act of force-closing a misbehaving node's stake channel, causing the node to lose time-locked funds and face a re-stake cooldown |
| **Re-stake Cooldown** | A mandatory 30-day waiting period before a slashed node may re-register and open a new stake channel |
| **SRI** | Stratum Reference Implementation — the open-source Stratum v2 pool software used by OWM |

---

*End of Business Requirements Specification v1.1.1*
