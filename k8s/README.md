# Kubernetes (coordinator stack)

This directory deploys **one self-contained stack per cluster**: PostgreSQL (StatefulSet), `owm-coordinator` (Deployment), and in-cluster Services only (no Ingress in the default overlays).

## Decentralization vs Kubernetes

**Kubernetes is not a decentralized control plane.** Each operator who runs a coordinator typically runs **their own** cluster (or a single-node `k3s`/kubeadm stack on a VPS). Adding more trusted coordinators in the OWM network means **more independent deployments**, each with its own Postgres and coordinator—not one shared multi-site Kubernetes cluster unless you deliberately build federation.

## Overlays

| Overlay | Use |
|--------|-----|
| `overlays/dev` | Auto-generated weak Postgres password, `OWM_DEV_MODE=true`, mock Lightning. For learning and integration tests on a lab cluster. |
| `overlays/prod` | `OWM_DEV_MODE=false`. You **must** create Secrets before apply (real LND settings + DSN). gRPC stays **plaintext on the pod network** unless you add TLS elsewhere. |
| `overlays/prod-mtls` | Same as prod intent plus **cert-manager** Issuers/Certificates and TLS mounts for gRPC. With `OWM_DEV_MODE=false`, the coordinator **requires mTLS**; issue client certs for every node identity from the same CA. Requires **cert-manager** installed in the cluster. |

Build manifests:

```bash
kubectl kustomize k8s/overlays/dev
kubectl kustomize k8s/overlays/prod
kubectl kustomize k8s/overlays/prod-mtls
```

Apply (example):

```bash
kubectl apply -k k8s/overlays/dev
```

For `dev`, load the image on the nodes (`docker save` / `docker load`, or push to a registry and set the `images` stanza in `overlays/dev/kustomization.yaml`).

## Production secrets (required for `prod` / `prod-mtls`)

Create namespace-scoped secrets **before** deploying the coordinator:

1. **`owm-postgres`** — key `password` (must match the password embedded in `OWM_DATABASE_DSN`).
2. **`owm-coordinator-env`** — use `env`-style keys (see [coordinator config](../owm-coordinator/internal/config/config.go)): at minimum `OWM_DATABASE_DSN`, and with `OWM_DEV_MODE=false`, all required `OWM_LIGHTNING_*` variables and macaroon/TLS **file paths** inside the container; mount those files via `Secret` volumes (extend the Deployment with a local patch or forked overlay).

Example (adjust DSN and do not commit real secrets):

```bash
kubectl create namespace owm --dry-run=client -o yaml | kubectl apply -f -
kubectl -n owm create secret generic owm-postgres --from-literal=password='YOUR_PG_PASSWORD'
kubectl -n owm create secret generic owm-coordinator-env \
  --from-literal=OWM_DATABASE_DSN='postgres://owm:YOUR_PG_PASSWORD@postgres:5432/owm?sslmode=disable' \
  --from-literal=OWM_DEV_MODE=false \
  --from-literal=OWM_LIGHTNING_LND_HOST='lnd.example.svc.cluster.local:10009' \
  --from-literal=OWM_LIGHTNING_READONLY_MACAROON_PATH=/secrets/lnd/readonly.macaroon \
  --from-literal=OWM_LIGHTNING_PAYMENT_MACAROON_PATH=/secrets/lnd/admin.macaroon \
  --from-literal=OWM_LIGHTNING_SLASHING_MACAROON_PATH=/secrets/lnd/slash.macaroon \
  --from-literal=OWM_LIGHTNING_TLS_CERT_PATH=/secrets/lnd/tls.cert
# Plus kubectl create secret generic owm-lnd-... --from-file=... and patch the Deployment to mount them.
```

## cert-manager (`prod-mtls`)

Install [cert-manager](https://cert-manager.io/docs/installation/) first. The overlay under `overlays/prod-mtls/cert-manager/` bootstraps an in-namespace CA, issues a server cert for `coordinator.owm.svc.cluster.local`, and mounts it into the coordinator. You still need **client certificates** signed by that CA for gRPC clients when `OWM_DEV_MODE=false`.

## Notes

- Migrations ship in the container image (`/migrations`); set `OWM_DATABASE_MIGRATIONS_DIR=/migrations` (already the default when unset; the Deployment sets it explicitly).
- Keep **one** coordinator replica while schema migrations run from the process on startup, or use a dedicated migrate Job if you move to multiple replicas later.
