# Security Policy

## Reporting a Vulnerability

**Please do not report security vulnerabilities through public GitHub issues.**

If you discover a security vulnerability in cloudtap, please report it privately:

1. **Email**: security@cloudtap.dev (or create a private security advisory on GitHub)
2. **GitHub Security Advisory**: [Create a private advisory](https://github.com/forest6511/cloudtap/security/advisories/new)

### What to Include

- Type of vulnerability (e.g., policy bypass, audit log tampering, injection)
- Affected component (adapter, policy engine, audit, API)
- Step-by-step reproduction instructions
- Proof of concept if available
- Impact assessment

### Response Timeline

- **Acknowledgment**: Within 48 hours
- **Initial Assessment**: Within 7 days
- **Resolution Target**: Within 30 days for critical issues

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 0.x.x   | :white_check_mark: |

As we're in early development, all 0.x versions receive security updates.

## Security Model

cloudtap is designed with security as a core principle:

### Default-Deny Policy

- All connections are denied unless explicitly allowed
- Policies must be configured before any traffic is permitted
- No implicit trust relationships

### Audit Log Integrity

- Hash chain prevents tampering with audit logs
- Each entry links to previous entry via cryptographic hash
- Verification endpoint to validate log integrity

### Adapter Security

- cloudtap wraps existing tunnels (frp, ngrok) without modifying their security
- Connection metadata is captured but traffic is not modified
- Policy decisions are enforced at the adapter layer

## Security Best Practices

When deploying cloudtap:

1. **Policy Configuration**
   - Start with deny-all, add explicit allow rules
   - Use specific patterns, avoid wildcards when possible
   - Regularly audit policy rules

2. **Audit Log Protection**
   - Export logs to immutable storage (S3, etc.)
   - Monitor for verification failures
   - Retain logs per compliance requirements

3. **Access Control**
   - Restrict access to cloudtapd API endpoints
   - Use TLS for all control plane communication
   - Rotate credentials regularly

4. **Monitoring**
   - Alert on policy denials
   - Monitor audit log verification status
   - Track adapter connection anomalies

## Threat Model

cloudtap addresses these security concerns:

| Threat | Mitigation |
|--------|------------|
| Unauthorized tunnel access | Default-deny policy engine |
| Untracked tunnel usage | Timeline capture and audit log |
| Audit log tampering | Hash chain verification |
| Policy bypass | Pre-connection policy evaluation |
| Compliance gaps | Exportable audit trails |

## Disclosure Policy

We follow coordinated disclosure:

1. Reporter notifies us privately
2. We acknowledge and investigate
3. We develop and test a fix
4. We release the fix with appropriate notice
5. We credit the reporter (if desired)

We appreciate responsible disclosure and will acknowledge contributors in our security advisories.
