# mTLS Setup for OWM Coordinator

This guide describes how to configure mutual TLS (mTLS) so that the OWM coordinator verifies node client certificates and only accepts connections from authenticated nodes.

## Overview

- The coordinator runs a gRPC server with TLS and **requires** client certificates (mTLS).
- Each **owm-node** presents a unique client certificate signed by the same CA the coordinator trusts.
- Config keys: `server.tls_cert_file`, `server.tls_key_file`, `server.ca_cert_file`.

## 1. Generate a CA

Create a private key and self-signed CA certificate (use a strong passphrase in production):

```bash
# CA key
openssl genrsa -out ca-key.pem 4096
# CA cert (valid 10 years)
openssl req -new -x509 -days 3650 -key ca-key.pem -out ca-cert.pem -subj "/CN=OWM-CA"
```

## 2. Issue the coordinator server certificate

The coordinator needs a server cert signed by the CA:

```bash
# Server key and CSR
openssl genrsa -out server-key.pem 4096
openssl req -new -key server-key.pem -out server.csr -subj "/CN=owm-coordinator.example.com"

# Sign with CA (adjust -extfile if you need SAN for your hostname)
openssl x509 -req -in server.csr -CA ca-cert.pem -CAkey ca-key.pem -CAcreateserial \
  -out server-cert.pem -days 825
```

Configure the coordinator:

- `server.tls_cert_file` = path to `server-cert.pem`
- `server.tls_key_file` = path to `server-key.pem`
- `server.ca_cert_file` = path to `ca-cert.pem`

## 3. Issue a client certificate per node

Each **owm-node** must have its own client certificate signed by the same CA:

```bash
# Node key and CSR (use a unique CN or SAN per node, e.g. node ID or hostname)
openssl genrsa -out node-key.pem 4096
openssl req -new -key node-key.pem -out node.csr -subj "/CN=owm-node-001"

# Sign with CA
openssl x509 -req -in node.csr -CA ca-cert.pem -CAkey ca-key.pem -CAcreateserial \
  -out node-cert.pem -days 825
```

The node uses `node-cert.pem` and `node-key.pem` when connecting to the coordinator's gRPC endpoint.

## 4. How owm-node presents its client cert

When **owm-node** connects to the coordinator:

1. It loads its client certificate and private key (`node-cert.pem`, `node-key.pem`).
2. It configures the gRPC client to use TLS with these credentials.
3. The coordinator's TLS layer verifies the client cert against `server.ca_cert_file` (the CA pool).
4. Only connections that present a certificate signed by that CA are accepted.

## 5. Dev mode

When `dev_mode` is true, the coordinator does **not** require client certificates even if TLS is enabled; it uses server-only TLS. Use dev mode only for local testing.

## 6. Summary of config keys

| Key | Description |
|-----|-------------|
| `server.tls_cert_file` | Path to the coordinator's server certificate |
| `server.tls_key_file` | Path to the coordinator's server private key |
| `server.ca_cert_file` | Path to the CA certificate used to verify node client certs (required when TLS is on and dev_mode is false) |
