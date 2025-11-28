# cloudtap

[![CI](https://github.com/forest6511/cloudtap/actions/workflows/ci.yml/badge.svg)](https://github.com/forest6511/cloudtap/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/forest6511/cloudtap)](https://goreportcard.com/report/github.com/forest6511/cloudtap)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

**The trustworthy, inspectable, policy-first tunnel.**

cloudtap wraps existing tunnel solutions (frp, ngrok) to add what they lack: visibility into who uses tunnels, policy enforcement before connections are allowed, and tamper-evident audit trails for compliance.

## Why cloudtap?

Today's tunnels are black boxes:

| Problem | Impact |
|---------|--------|
| No visibility into tunnel usage | "Who exposed our staging DB last week?" |
| No policy enforcement | Tunnels bypass your access controls |
| No audit trail | Compliance gaps, incident investigation blind spots |

**cloudtap solves this:**

- **Inspectable** - Capture connection metadata (who/what/when/where)
- **Policy-first** - Default-deny with explicit allow lists
- **Auditable** - Tamper-evident logs with hash chain verification

## How It Works

```
┌────────────────┐                    ┌─────────────────────┐
│  Existing      │                    │  cloudtap           │
│  Tunnel        │◄── wraps ─────────►│  Adapter            │
│  (frp/ngrok)   │                    │  (capture + policy) │
└────────────────┘                    └──────────┬──────────┘
                                                 │
                   ┌─────────────────────────────┼─────────────────────────────┐
                   │                             │                             │
                   ▼                             ▼                             ▼
          ┌───────────────┐           ┌───────────────┐            ┌───────────────┐
          │ Timeline      │           │ Policy        │            │ Audit Log     │
          │ Capture       │           │ Engine        │            │ (hash chain)  │
          └───────────────┘           └───────────────┘            └───────────────┘
                   │                             │                             │
                   ▼                             ▼                             ▼
          ┌───────────────┐           ┌───────────────┐            ┌───────────────┐
          │ OTLP/SIEM     │           │ Default-Deny  │            │ S3/Kafka      │
          │ Export        │           │ + Allow List  │            │ Export        │
          └───────────────┘           └───────────────┘            └───────────────┘
```

1. cloudtap wraps your existing tunnel with an adapter
2. Every connection is captured, evaluated against policy, and logged
3. Events export to your observability stack and audit sinks

## Quick Start

### Prerequisites

- Go 1.24+
- frp installed ([fatedier/frp](https://github.com/fatedier/frp))

### Install

```bash
git clone https://github.com/forest6511/cloudtap.git
cd cloudtap
make build
```

### Run

```bash
# Start cloudtapd with frp adapter
./bin/cloudtapd --adapter frp --frp-config /path/to/frpc.toml

# List policies
./bin/cloudtapctl policy list

# Add allow rule
./bin/cloudtapctl policy allow --target staging-api --user dev-team

# View connection timeline
./bin/cloudtapctl timeline --last 1h

# Verify audit log integrity
./bin/cloudtapctl audit verify
```

### Example Policy (Default-Deny)

```yaml
# cloudtap-policy.yaml
default: deny

rules:
  - name: allow-dev-team-staging
    effect: allow
    match:
      target: "staging-*"
      user: "dev-team-*"
      source_ip: "10.0.0.0/8"

  - name: allow-oncall-production
    effect: allow
    match:
      target: "prod-*"
      user: "oncall-*"
    require:
      approval: true
      expires_in: 1h
```

### Example Timeline Output

```json
{
  "events": [
    {
      "id": "evt_abc123",
      "timestamp": "2024-01-15T10:30:00Z",
      "event_type": "connection_start",
      "target": "staging-api",
      "user": "alice@dev-team",
      "source_ip": "10.0.1.50",
      "decision": "allowed",
      "matched_policy": "allow-dev-team-staging"
    }
  ]
}
```

## Who Is This For?

| Audience | Use Case |
|----------|----------|
| **Platform/SRE teams** | Governed remote access to internal services |
| **Regulated industries** | SOC2/PCI compliance with audit trails |
| **Tool vendors** | Auditable remote support sessions |

## What We're NOT Building

- Feature parity with frp/ngrok (dashboards, file transfer)
- Hosted tunnel marketplace
- Raw tunnel performance competition

We focus on **governance and observability**, not tunnel infrastructure.

## Project Status

**Phase 0 - Observability + Policy Wrapper** (In Development)

- [ ] frp adapter with connection interception
- [ ] Timeline capture (who/what/when)
- [ ] Default-deny policy engine
- [ ] Audit log with hash chain
- [ ] OTLP export

See [ROADMAP.md](ROADMAP.md) for the full plan.

## Documentation

- [ROADMAP.md](ROADMAP.md) - Development phases and milestones
- [CONTRIBUTING.md](CONTRIBUTING.md) - How to contribute
- [SECURITY.md](SECURITY.md) - Security policy and reporting
- [docs/api/openapi.yaml](docs/api/openapi.yaml) - HTTP API specification
- [proto/tunnel/v1/](proto/tunnel/v1/) - gRPC service definitions

## Contributing

We welcome contributions! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

Priority areas for Phase 0:
- `pkg/adapter/frp/` - frp adapter implementation
- `pkg/capture/` - Connection metadata capture
- `pkg/policy/` - Policy evaluation engine
- `pkg/audit/` - Tamper-evident audit log

## Security

Please report security vulnerabilities privately. See [SECURITY.md](SECURITY.md) for details.

## License

[Apache License 2.0](LICENSE)
