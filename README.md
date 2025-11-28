# cloudtap

cloudtap is the **trustworthy, inspectable, policy-first tunnel**. Unlike traditional tunneling tools that focus solely on connectivity, cloudtap provides visibility and governance for security-minded teams. It wraps existing tunnel solutions (starting with frp) to add observability, policy enforcement, and tamper-evident audit trails.

- [Roadmap](./ROADMAP.md)

## Why cloudtap?

Today's tunnels are black boxes. There's no unified trace, no policy, and no tamper-proof audit of who exposed what, when, and to whom. Risk acceptance is implicit and untracked.

cloudtap solves this by providing:

- **Inspectable by default**: Capture connection metadata, request timelines, and structured events. Know exactly what's happening in every tunnel session.
- **Policy-first access**: Default-deny with allow lists, role-based policies, and approval workflows. Security teams get governance without blocking developers.
- **Tamper-evident audit**: Signed audit logs with hash chains. Prove compliance and investigate incidents with confidence.
- **Works with existing tunnels**: Start with frp adapter today. No need to replace your current tunnel infrastructure to get visibility.

## Who is cloudtap for?

- **Platform/SRE teams** needing governed remote access to dev/stage/internal services
- **Regulated industries** (fintech, healthcare) requiring compliance and audit trails
- **Tool vendors** needing auditable remote support sessions for customers

## Architecture

```
┌────────────────┐                    ┌─────────────────────┐
│  Existing      │                    │  cloudtap           │
│  Tunnel        │◄── wraps ─────────►│  Adapter            │
│  (frp/ngrok)   │                    │  (capture + policy) │
└────────────────┘                    └──────────┬──────────┘
                                                 │
                    ┌────────────────────────────┼────────────────────────────┐
                    │                            │                            │
                    ▼                            ▼                            ▼
           ┌───────────────┐           ┌───────────────┐           ┌───────────────┐
           │ Timeline      │           │ Policy        │           │ Audit Log     │
           │ Capture       │           │ Enforcement   │           │ (signed)      │
           └───────────────┘           └───────────────┘           └───────────────┘
                    │                            │                            │
                    ▼                            ▼                            ▼
           ┌───────────────┐           ┌───────────────┐           ┌───────────────┐
           │ OTLP/SIEM     │           │ Allow/Deny    │           │ S3/Kafka      │
           │ Export        │           │ Decisions     │           │ Sink          │
           └───────────────┘           └───────────────┘           └───────────────┘
```

1. cloudtap wraps your existing tunnel (frp, ngrok, etc.) with an adapter
2. The adapter captures metadata, enforces policies, and logs all access
3. Events stream to your existing observability stack (OTLP) and audit sinks (S3, Kafka)

## Current Status

**Phase 0 - Observability + Policy Wrapper (In Development)**

We're building the frp adapter that adds:
- Connection metadata capture (who/what/where)
- Request timeline recording
- Default-deny policy with allow lists
- OTLP export to existing SIEM/APM

See the [Roadmap](./ROADMAP.md) for full phase details.

## Getting Started (alpha)

Prerequisites:

- Go 1.24.3 or later
- frp installed and configured (cloudtap wraps frp, doesn't replace it)
- `golangci-lint` for development

Bootstrap:

```bash
git clone https://github.com/forest6511/cloudtap.git
cd cloudtap
go mod tidy
make build           # builds cloudtapd and cloudtapctl into ./bin
make test && make lint
```

### Basic Usage (with frp adapter)

```bash
# Start cloudtapd with frp adapter mode
./bin/cloudtapd --adapter frp --frp-config /path/to/frpc.toml

# Use cloudtapctl to manage policies and view timeline
./bin/cloudtapctl policy list
./bin/cloudtapctl policy allow --target staging-api --user dev-team
./bin/cloudtapctl timeline --last 1h
```

## API Reference

- gRPC definitions: [`proto/tunnel/v1/tunnel.proto`](proto/tunnel/v1/tunnel.proto)
- HTTP endpoints: [`docs/api/openapi.yaml`](docs/api/openapi.yaml)

## What We're NOT Building (for now)

- Feature parity with frp/ngrok (dashboards, file transfer, generic TCP relays)
- Long-lived static tunnels without audit context
- Hosted tunnel marketplace

We're focused on making tunnels inspectable and governed, not replacing tunnel infrastructure.

## Contributing

Issues are tagged by phase. Focus areas:
- Adapters (frp, ngrok, cloudflared)
- Policy engine and DSL
- Observability exports (OTLP, S3, Kafka)

See [AGENTS.md](./AGENTS.md) for development guidelines.

## License

Apache 2.0 (planned). A dedicated LICENSE file will be added before the first tagged release.
