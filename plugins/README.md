# plugins/

Plugin experiments for cloudtap (Phase 3).

## Status

Plugins are **not a Phase 0 priority**. This directory is reserved for future plugin development once the core observability and policy features are validated.

## Planned Plugin Types

When Phase 3 begins, plugins will support:

1. **Observability Enrichers** - Add custom metadata to timeline events
2. **Policy Hooks** - Custom policy evaluation logic
3. **Audit Sinks** - Export audit logs to custom destinations
4. **Incident Capture** - Automated incident recording on anomalies
5. **Notification** - On-call alerts via PagerDuty, Slack, etc.

## Plugin Architecture (Planned)

Plugins will respond to lifecycle events via HTTP webhooks (similar to frp):

```
Events:
- OnConnect      - New connection established
- OnDisconnect   - Connection closed
- OnPolicyCheck  - Before policy evaluation
- OnAnomaly      - Unusual pattern detected
- OnAuditWrite   - Before audit log entry
```

## Do NOT Build Yet

Focus Phase 0 efforts on:
- `pkg/adapter/` - frp adapter
- `pkg/capture/` - Timeline capture
- `pkg/policy/` - Policy engine
- `pkg/audit/` - Audit log

Plugins come after core features prove value with design partners.
