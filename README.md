# Open World Model (OWM)

> *"A living, open model of our world — built by everyone, owned by no one, powered by Bitcoin."*

## What is OWM?

The **Open World Model** is an open-source, federated ensemble of AI models that build and maintain a comprehensive, continuously updated model of the real world — geography, science, code, language, and live data. The system runs on a permissionless network of GPU nodes. Node operators are compensated in **Bitcoin** via the **Lightning Network**. Model versions are anchored to the Bitcoin blockchain via **OpenTimestamps**. The project integrates with **GitHub** for automated code analysis, security audits, and PR generation. Participation is secured by Bitcoin-native **Proof-of-Work** (optional mining pool contribution) and **Proof-of-Stake** (Lightning channel stake to the treasury); there is no new token.

For full business and technical requirements, see [BRS.md](BRS.md) and [SRS.md](SRS.md).

## Current status

The repository is in **early development**. The only implemented component is **owm-coordinator** (Go): the central registry, task scheduler, federated-learning orchestration, stake verification, and gRPC API. The following are specified in the SRS and planned:

- **owm-node** — Python + Rust node daemon (inference, training, optional mining client)
- **owm-pool** — Stratum v2 Bitcoin mining pool (Rust)
- **owm-governance** — Rough-consensus governance portal (Python/HTMX)
- Public REST/WebSocket API, deploy configs, and additional docs

Design and architecture are documented in [docs/architecture.md](docs/architecture.md) and the [docs/adr/](docs/adr/) decision records (ADR-001–ADR-005).

## Repository structure

```
Open-World-Model/
├── BRS.md              # Business Requirements Specification
├── SRS.md              # Software Requirements Specification
├── README.md           # This file
├── owm-coordinator/    # Coordinator service (Go) — registry, scheduler, FL, stake, gRPC
│   ├── cmd/coordinator/
│   ├── internal/       # config, registry, scheduler, fl, rpc, stake, lightning
│   ├── proto/coordinator/v1/
│   ├── Dockerfile
│   └── go.mod
├── docs/
│   ├── architecture.md
│   └── adr/            # ADR-001 through ADR-005
└── .github/workflows/  # CI (coordinator build, lint, Docker)
```

A full target layout (including node, pool, governance, and deploy) is described in [SRS.md §13](SRS.md#13-directory-structure).

## Development

### Coordinator (owm-coordinator)

- **Requirements:** Go 1.22+, PostgreSQL 16+, `protoc` for generating gRPC stubs.
- **Build:** From repo root, `cd owm-coordinator && go build ./cmd/coordinator`
- **Tests:** `go test -v ./...` (set `OWM_TEST_DSN` for integration tests, e.g. `postgres://owm:owm@localhost:5432/owm_test`).
- **Proto:** Generate stubs with `protoc` as in [.github/workflows/ci.yml](.github/workflows/ci.yml).

Configuration is file-based (see `internal/config`); a config file path can be set via `OWM_CONFIG_FILE` or the first CLI argument.

## Documentation

| Document | Description |
|----------|-------------|
| [BRS.md](BRS.md) | Business requirements, scope, roadmap |
| [SRS.md](SRS.md) | Software requirements, architecture, directory structure |
| [docs/architecture.md](docs/architecture.md) | System topology, sequences, deployment |
| [docs/adr/](docs/adr/) | Architecture decision records |
| [docs/SBOM.md](docs/SBOM.md) | Software Bill of Materials (dependencies) |

Node setup, API reference, FL protocol, Lightning integration, GitHub App, governance, OTS verification, mining pool, and proof-of-stake guides are planned per the SRS and will be added as those components are implemented.

## Contributing and license

CONTRIBUTING.md and LICENSE are planned; the BRS and SRS reference them for the full project. Until they are added, open an issue or pull request for contribution and licensing questions.
