# cloudtap

cloudtap is a secure, observability-first tunneling proxy that makes staging and ephemeral environments easy to reach from local tools. Built in Go, it combines high-throughput multiplexing, adaptive bandwidth management, and programmable hooks so teams can debug cloud-native systems without shipping extra sidecars.

- [Roadmap](./ROADMAP.md)

## Why cloudtap?

- **Safe remote access**: Mutual TLS, short-lived tokens, and per-route policies ensure only the intended engineer can reach a protected service.
- **Observable by default**: Mirror HTTP/gRPC traffic, redact sensitive fields, and stream structured events to CLIs, VS Code extensions, or tracing pipelines.
- **Adaptive concurrency**: Borrowing ideas from `gdl`, cloudtap auto-tunes multiplexed streams and rate limits to avoid overwhelming staging clusters.
- **Programmable hooks**: A lightweight plugin SDK emits lifecycle events (connect, request, anomaly) so teams can automate triage and incident capture workflows.

## Architecture Snapshot

```
┌────────────┐       mTLS tunnel        ┌───────────────┐
│ Local Tool │ ───────────────────────► │ cloudtap Edge │
└────────────┘                         │  (Go server)  │
       ▲                                └──────┬────────┘
       │                              mirrored │ traffic
       │                                      ▼
┌────────────┐                         ┌───────────────┐
│ VS Code    │◄──── WebSocket/GRPC ────│ Observability │
│ Extension  │                         │ backends      │
└────────────┘                         └───────────────┘
```

1. Developers run `cloudtap proxy <service>` to open a mutually authenticated tunnel into staging.
2. Requests pass through the cloudtap edge, which enforces policy, captures metadata, and multiplexes streams.
3. Structured events stream to local clients and optional sinks such as OpenTelemetry, S3, or Kafka for replay.

## Getting Started (alpha)

Prerequisites:

- Go 1.24.3 matching the local toolchain (`go env GOVERSION`).
- `golangci-lint` available on your PATH (`go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`).
- Protocol buffers compiler (`protoc`) plus the Go plugins (`go install google.golang.org/protobuf/cmd/protoc-gen-go@latest google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest`) when editing APIs.

Bootstrap:

```bash
git clone https://github.com/forest6511/cloudtap.git
cd cloudtap
go mod tidy
make build           # builds cmd/cloudtapd and cmd/cloudtapctl into ./bin
make run             # starts the HTTP+gRPC daemon with placeholders
make test && make lint

# once cloudtapd is running in another terminal
./bin/cloudtapctl proxy demo-service          # uses gRPC first, HTTP fallback via --http-url
./bin/cloudtapctl list-targets                # discover available targets
./bin/cloudtapctl close demo-service          # close the active tunnel

# regenerate Go stubs whenever proto/tunnel/v1/*.proto changes
make proto
```

`cloudtapctl` accepts `--server` for gRPC (default `localhost:9090`) and `--http-url` for HTTP fallback (default `http://localhost:8080`). Each command (proxy/close/list-targets) attempts the gRPC path first and automatically falls back to the corresponding HTTP endpoint when the RPC layer is unavailable. For secured deployments, provide `--token`, `--tls-cert/--tls-key`, `--ca-cert`, and (dev-only) `--insecure-skip-verify` to align with edge policies.

## API Reference

- gRPC definitions live in [`proto/tunnel/v1/tunnel.proto`](proto/tunnel/v1/tunnel.proto).
- HTTP parity endpoints (`/proxy`, `/close`, `/targets`) are described via [OpenAPI](docs/api/openapi.yaml) for integration with other clients.

> **Status:** Early design phase. Feedback and contributors are welcome! For phase goals, see the [Roadmap](./ROADMAP.md).

## License

Apache 2.0 (planned). A dedicated LICENSE file will be added before the first tagged release.
