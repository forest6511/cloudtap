# Roadmap

The public roadmap outlines the features that will shape cloudtap's OSS release.
Targeted deliverables may shift as early adopters share feedback, but each phase
represents a shippable milestone.

## Phase 0 – Foundation (MVP)
- Bootstrap CLI and daemon binaries (`cmd/cloudtapctl`, `cmd/cloudtapd`).
- Establish mutually authenticated tunnels for a single staging target.
- Expose `/healthz` and `/proxy` endpoints plus a minimal gRPC API surface.
- Produce structured logs and placeholder events for future exporters.

## Phase 1 – Observability & Developer Ergonomics
- Implement request timeline capture and streaming log APIs.
- Add OpenTelemetry/OTLP exporter pipelines and redaction helpers.
- Ship VS Code task integration and standalone CLI visualizers.
- Harden `pkg/tunnel` multiplexing with adaptive backpressure.

## Phase 2 – Security & Policy
- Introduce role-based policies, token vending service, and short-lived creds.
- Release policy-as-code DSL with unit tests living under `pkg/policy`.
- Deliver audit logging sinks (S3, Kafka) and tamper-evident storage options.

## Phase 3 – Ecosystem & Plugins
- Publish plugin SDK with lifecycle hooks (connect, anomaly, teardown).
- Provide reference plugins for incident capture, on-call notifications, and
  trace replay.
- Offer GitHub Actions helper + Terraform module templates for self-hosting.

## How to contribute
- Pick issues tagged `help wanted` linked to the phases above.
- Discuss major roadmap adjustments via GitHub Discussions before filing PRs.
- Internal deep-dive notes belong in `docs/internal/` (git-ignored) so the
  public roadmap stays high-level and safe to share.
