# Roadmap

cloudtap’s direction has shifted to deliver the observability and policy layer first, wrapping existing tunnels before deciding whether to build our own core. Each phase is independently shippable, validated with real users, and paired with go-to-market milestones.

## Value Proposition
- The trustworthy, inspectable, policy-first tunnel.
- Visibility and governance for security-minded platform/SRE teams and regulated industries.
- Works with existing tunnels (starting with frp) to prove value quickly.

## Phase 0 – Observability + Policy Wrapper (Pilot)
- Ship frp adapter: terminate/connect via frp while adding cloudtap’s sidecar for capture and policy enforcement.
- Capture connection metadata (who/what/where), request timelines, and structured events; export via OTLP to existing SIEM/APM.
- Default-deny policy gate with allow lists, redaction filters, and audit trails; CLI/daemon minimal verbs to configure adapters and policies.
- Go-to-market: recruit 3–5 design partners in security/reg fintech; publish trust posture doc and integration quickstart; measure success via deploy-to-first-signal time and policy-block events.

## Phase 1 – Compliance-Grade Visibility
- Deep traces for tunnel sessions (latency budget, retries, anomalies) with live streaming views and retention controls.
- Pluggable sinks (S3, Kafka) with tamper-evident hashes; SOC-friendly reports and webhook alerts.
- Redaction/testing harness in `pkg/policy` with table-driven cases for PII/PHI masking.
- Go-to-market: case-study pilots with two regulated customers; marketing site that centers “inspectable tunnel”; add compliance one-pager and demo assets.

## Phase 2 – Governed Access & Automation
- Role/tenant-aware policies, token vending for short-lived credentials, and approval workflows (chat+CLI) with full audit logs.
- Policy-as-code DSL and conformance tests; drift detection plus “explain why allowed/denied.”
- Integration hooks for incident capture and on-call notifications; GitHub Actions helper + Terraform module for self-hosted installs.
- Go-to-market: early-adopter program with success metrics (MTTD on policy violations, coverage of audited tunnels); publish reference architectures.

## Phase 3 – Ecosystem & Plugins
- Plugin SDK with lifecycle hooks (connect, anomaly, teardown) so vendors can add observability enrichers and compliance checks.
- Reference plugins for SIEM enrichment, trace replay, and remediation suggestions; curated catalog with signing/verification guidance.
- Partner enablement kit: co-marketing playbook, sample pricing, and support SLAs for vendors offering auditable remote support.

## Phase 4 – Native Tunnel Core (Conditional)
- Build our own core only if design-partner metrics show sustained value and gaps in third-party tunnels.
- Goals: deterministic reconnect/backpressure, policy-first session broker, embedded eBPF instrumentation for zero-dup capture.
- If not pursued, continue investing in adapters (ngrok, cloud provider offerings) and tighten policy/observability differentiators.

## Not Building (for now)
- Feature parity with frp/ngrok (UI dashboards, file transfer portals, generic TCP relays) unless needed for policy/observability.
- Long-lived static tunnels without audit or policy context.
- Self-hosted management plane before governance features prove value with design partners.

## Contributing
- Issues are tagged by phase; focus first on adapters, policy, and observability exports.
- Discuss roadmap shifts in GitHub Discussions; keep deep dives in `docs/internal/` so the public roadmap remains safe to share.
