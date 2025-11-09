BINDIR ?= bin
PKG := ./...
PROTO_FILES := $(shell find proto -name '*.proto')

.PHONY: build run dev test lint tidy clean proto

build:
	@mkdir -p $(BINDIR)
	GOOS=$${GOOS:-$(shell go env GOOS)} GOARCH=$${GOARCH:-$(shell go env GOARCH)} go build -o $(BINDIR)/cloudtapd ./cmd/cloudtapd
	GOOS=$${GOOS:-$(shell go env GOOS)} GOARCH=$${GOARCH:-$(shell go env GOARCH)} go build -o $(BINDIR)/cloudtapctl ./cmd/cloudtapctl

run:
	go run ./cmd/cloudtapd

dev:
	GOLOG_LEVEL=debug go run ./cmd/cloudtapd --dev

test:
	go test $(PKG)

lint:
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		echo "golangci-lint not installed. Install via 'go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest' or run 'make lint' inside CI."; \
		exit 1; \
	fi
	golangci-lint run ./...

tidy:
	go mod tidy

clean:
	rm -rf $(BINDIR)

proto:
	@if ! command -v protoc >/dev/null 2>&1; then \
		echo "protoc not found. Install from https://grpc.io/docs/protoc-installation/"; \
		exit 1; \
	fi
	PATH=$$HOME/go/bin:$$PATH protoc --go_out=. --go-grpc_out=. $(PROTO_FILES)
