# Software Bill of Materials (SBOM)

**Project:** Open World Model (OWM)  
**SBOM Version:** 0.1.0  
**Generated:** 2026-03-05  
**Status:** Current / ongoing — updated as components and dependencies change  

This document lists the software components (dependencies) used to build the OWM codebase. It supports security review, compliance, and supply-chain transparency. The scope is the current repository; as new components (e.g. owm-node, owm-pool, owm-governance) are added, they will be included here.

## Format and conventions

- **Minimum elements** follow [NTIA SBOM guidance](https://www.ntia.gov/report/2021/minimum-elements-software-bill-materials-sbom): supplier/author, component name, version, dependency relationship (direct vs indirect).
- **Package URL (PURL)** is used where applicable: [purl-spec](https://github.com/package-url/purl-spec). For Go: `pkg:golang/<module-path>@<version>`.
- **License** uses [SPDX License Identifier](https://spdx.org/licenses/) where known; otherwise "See upstream."
- **Scope:** "Direct" = explicitly required by the project; "Indirect" = pulled in by a direct dependency.

## Regenerating / extending the SBOM

- **owm-coordinator (Go):** From repo root, run:
  ```bash
  cd owm-coordinator && go list -m all
  ```
  To produce a full transitive list with versions. CI can generate a machine-readable SBOM (e.g. CycloneDX) via `syft` or `go list -m -json all`.
- **owm-node (Python + Rust):** When present, use `pip list` / `pip freeze` and `cargo tree` (or `cargo metadata`).
- **owm-pool (Rust):** When present, use `cargo tree` or `cargo metadata`.

---

## 1. owm-coordinator (Go)

**Artifact:** Coordinator service (registry, scheduler, FL orchestration, stake verification, gRPC API).  
**Module path:** `github.com/owmnetwork/owm-coordinator`  
**Go version:** 1.22  
**Source of truth:** [owm-coordinator/go.mod](../owm-coordinator/go.mod)

### 1.1 Direct dependencies

| Component | Version | PURL | License | Purpose |
|-----------|---------|------|---------|---------|
| github.com/google/uuid | v1.6.0 | `pkg:golang/github.com/google/uuid@v1.6.0` | BSD-3-Clause | UUID generation |
| github.com/jackc/pgx/v5 | v5.5.5 | `pkg:golang/github.com/jackc/pgx/v5@v5.5.5` | MIT | PostgreSQL driver |
| github.com/prometheus/client_golang | v1.19.0 | `pkg:golang/github.com/prometheus/client_golang@v1.19.0` | Apache-2.0 | Metrics (Prometheus) |
| github.com/redis/go-redis/v9 | v9.5.1 | `pkg:golang/github.com/redis/go-redis/v9@v9.5.1` | BSD-2-Clause | Redis client |
| github.com/spf13/viper | v1.18.2 | `pkg:golang/github.com/spf13/viper@v1.18.2` | MIT | Configuration |
| go.uber.org/zap | v1.27.0 | `pkg:golang/go.uber.org/zap@v1.27.0` | MIT | Structured logging |
| golang.org/x/crypto | v0.22.0 | `pkg:golang/golang.org/x/crypto@v0.22.0` | BSD-3-Clause | Cryptography (e.g. Ed25519) |
| google.golang.org/grpc | v1.63.2 | `pkg:golang/google.golang.org/grpc@v1.63.2` | Apache-2.0 | gRPC server/client |
| google.golang.org/protobuf | v1.34.1 | `pkg:golang/google.golang.org/protobuf@v1.34.1` | BSD-3-Clause | Protocol Buffers |

### 1.2 Indirect dependencies (from go.mod require block)

| Component | Version | PURL | License | Purpose |
|-----------|---------|------|---------|---------|
| github.com/golang-migrate/migrate/v4 | v4.17.1 | `pkg:golang/github.com/golang-migrate/migrate/v4@v4.17.1` | MIT | Database migrations |

### 1.3 Full transitive tree

The full list of transitive dependencies (including versions) is produced by:

```bash
cd owm-coordinator && go list -m all
```

Run this after `go mod tidy` (or in CI) to audit the complete dependency graph. Transitive dependencies of the packages above are not duplicated in this document; they are tracked in [owm-coordinator/go.sum](../owm-coordinator/go.sum) once present.

---

## 2. owm-node (Python + Rust)

**Status:** Not yet implemented (planned per [SRS](../SRS.md)).  
When available, this section will list Python (PyPI) and Rust (crates.io) dependencies used by the node daemon.

---

## 3. owm-pool (Rust)

**Status:** Not yet implemented (planned per [SRS](../SRS.md)).  
When available, this section will list Rust (crates.io) dependencies used by the Stratum v2 mining pool.

---

## 4. owm-governance (Python / HTMX)

**Status:** Not yet implemented (planned per [SRS](../SRS.md)).  
When available, this section will list dependencies used by the governance portal.

---

## 5. Build and tooling (CI / shared)

| Component | Version | PURL / reference | License | Purpose |
|-----------|---------|------------------|---------|---------|
| Go | 1.22 | — | BSD-3-Clause | Language/runtime (owm-coordinator) |
| protoc (Protocol Buffers compiler) | — | — | BSD-3-Clause | Code generation from .proto |
| Docker (Buildx) | — | — | Apache-2.0 | Container builds in CI |

Versions for CI are defined in [.github/workflows/ci.yml](../.github/workflows/ci.yml).

---

## Changelog

| Date | SBOM version | Changes |
|------|--------------|---------|
| 2026-03-05 | 0.1.0 | Initial SBOM: owm-coordinator direct and listed indirect deps; placeholders for owm-node, owm-pool, owm-governance. |