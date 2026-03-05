# ── Build stage ──────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

# Install build dependencies.
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /src

# Cache dependencies before copying source so layer is reused on code-only changes.
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build a statically linked binary.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" \
    -o /out/coordinator ./cmd/coordinator

# ── Runtime stage ─────────────────────────────────────────────────────────────
FROM scratch

# Import CA certificates and timezone data from builder.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /out/coordinator /coordinator

# gRPC port
EXPOSE 9000
# Internal HTTP (health / Prometheus metrics)
EXPOSE 9001

ENTRYPOINT ["/coordinator"]
