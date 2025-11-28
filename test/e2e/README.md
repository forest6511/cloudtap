# test/e2e

End-to-end tests for cloudtap using real tunnel adapters.

## Test Setup

Tests use Docker Compose to spin up:
- frp server (frps)
- frp client (frpc)
- Echo service (target)
- cloudtap daemon with frp adapter

## Running Tests

```bash
# Start test infrastructure
docker-compose -f docker-compose.test.yml up -d

# Run E2E tests
make e2e

# Or run specific test
go test ./test/e2e -run TestPolicyEnforcement -v

# Cleanup
docker-compose -f docker-compose.test.yml down
```

## Test Scenarios

### Phase 0 Focus

1. **Adapter Lifecycle** - Start/stop/reconnect with frp
2. **Policy Enforcement** - Allow/deny decisions
3. **Timeline Capture** - Connection metadata extraction
4. **Audit Log Integrity** - Hash chain verification

### Test Structure

```
test/e2e/
├── docker-compose.test.yml  # Test infrastructure
├── fixtures/                # Test configs
│   ├── frps.toml           # frp server config
│   ├── frpc.toml           # frp client config
│   └── policies/           # Test policy files
├── adapter_test.go         # Adapter lifecycle tests
├── policy_test.go          # Policy enforcement tests
├── timeline_test.go        # Timeline capture tests
└── audit_test.go           # Audit log tests
```

## Writing Tests

- Use table-driven tests
- Each scenario should be hermetic (no shared state)
- Include both positive and negative cases
- Test policy allow AND deny paths
