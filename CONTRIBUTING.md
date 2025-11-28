# Contributing to cloudtap

Thank you for your interest in contributing to cloudtap! This document provides guidelines and instructions for contributing.

## Code of Conduct

By participating in this project, you agree to abide by our [Code of Conduct](CODE_OF_CONDUCT.md).

## How to Contribute

### Reporting Bugs

Before submitting a bug report:
1. Check the [existing issues](https://github.com/forest6511/cloudtap/issues) to avoid duplicates
2. Collect relevant information (OS, Go version, tunnel adapter type, logs)

When reporting bugs, please include:
- A clear, descriptive title
- Steps to reproduce the issue
- Expected vs actual behavior
- Environment details (OS, Go version, adapter type)
- Relevant log output

### Suggesting Features

Feature requests are welcome! Please:
1. Check existing issues and the [ROADMAP](ROADMAP.md) first
2. Describe the use case and problem you're solving
3. Explain how the feature fits cloudtap's focus on observability and policy

### Pull Requests

1. Fork the repository
2. Create a feature branch from `main`:
   ```bash
   git checkout -b feature/your-feature-name
   ```
3. Make your changes following our coding guidelines
4. Run tests and linting:
   ```bash
   make test
   make lint
   ```
5. Commit with conventional commit messages:
   ```bash
   git commit -m "feat(adapter): add ngrok connection interception"
   ```
6. Push and create a Pull Request

## Development Setup

### Prerequisites

- Go 1.24+
- Make
- golangci-lint

### Getting Started

```bash
# Clone your fork
git clone https://github.com/YOUR_USERNAME/cloudtap.git
cd cloudtap

# Install dependencies
go mod download

# Run tests
make test

# Run linter
make lint

# Build
make build

# Run in dev mode
make dev
```

## Coding Guidelines

### Style

- Format code with `gofmt -s` and `goimports`
- Follow [Effective Go](https://golang.org/doc/effective_go) guidelines
- Use structured logging with snake_case field keys
- Keep functions under 40 lines when possible

### Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

- `feat:` New feature
- `fix:` Bug fix
- `docs:` Documentation changes
- `test:` Test additions or changes
- `refactor:` Code refactoring
- `chore:` Maintenance tasks

Include issue numbers when applicable: `feat(policy): add CIDR matching (#42)`

### Testing

- Write table-driven tests
- Target 80%+ coverage for new code
- Include both positive and negative test cases
- Run race detection: `go test ./... -race`

### Security

- Never commit secrets or credentials
- Test both allow and deny paths for policy changes
- Consider security implications in PR descriptions
- Report security vulnerabilities privately (see [SECURITY.md](SECURITY.md))

## Project Structure

```
cloudtap/
├── cmd/
│   ├── cloudtapd/      # Daemon
│   └── cloudtapctl/    # CLI
├── pkg/
│   ├── adapter/        # Tunnel adapters (frp, ngrok)
│   ├── capture/        # Connection metadata capture
│   ├── policy/         # Policy evaluation engine
│   ├── audit/          # Audit log with hash chain
│   └── export/         # OTLP, S3, Kafka exporters
├── proto/              # gRPC definitions
└── test/e2e/           # End-to-end tests
```

## Phase 0 Priority Areas

We're currently focused on:
1. **frp adapter** (`pkg/adapter/frp/`) - Wrapping frp connections
2. **Capture layer** (`pkg/capture/`) - Connection metadata extraction
3. **Policy engine** (`pkg/policy/`) - Default-deny evaluation
4. **Audit log** (`pkg/audit/`) - Tamper-evident logging

See [ROADMAP.md](ROADMAP.md) for the complete roadmap.

## Getting Help

- Open a [Discussion](https://github.com/forest6511/cloudtap/discussions) for questions
- Check existing issues and documentation
- Be patient and respectful in all interactions

## License

By contributing, you agree that your contributions will be licensed under the [Apache License 2.0](LICENSE).
