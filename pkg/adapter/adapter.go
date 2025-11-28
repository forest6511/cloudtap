// Package adapter provides interfaces and types for tunnel adapters.
//
// cloudtap wraps existing tunnel solutions (frp, ngrok, cloudflared) through
// adapters that intercept connection events and pass them to the capture
// and policy layers.
//
// # Event Ordering
//
// For each connection, events are guaranteed to be delivered in order:
//
//  1. EventConnect - must be delivered first and handler decision obtained
//  2. EventRequest - zero or more, only if EventConnect was allowed
//  3. EventDisconnect - delivered last, regardless of previous decisions
//
// Events for different connections may be interleaved. The ConnectionID
// field links related events.
//
// # Immutability Contract
//
// All structs (ConnectionEvent, RequestInfo) and their nested maps are
// immutable after creation. Adapter implementations MUST deep-copy all
// maps and nested structures before dispatching events to handlers.
// Handlers MUST NOT modify any event fields, maps, or nested structures.
//
// # Example Usage
//
//	adapter := frp.New(cfg)
//	adapter.OnConnection(func(event ConnectionEvent) Decision {
//	    // Evaluate policy, capture metadata, etc.
//	    return Decision{Allowed: true, Reason: "policy matched"}
//	})
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//	if err := adapter.Start(ctx); err != nil {
//	    log.Fatal(err)
//	}
package adapter

import (
	"context"
	"errors"
	"time"
)

// Sentinel errors for adapter operations.
var (
	// ErrHandlerNotSet is returned by Start when OnConnection has not been called.
	ErrHandlerNotSet = errors.New("adapter: connection handler not set")

	// ErrAlreadyStarted is returned by Start when the adapter is already running.
	ErrAlreadyStarted = errors.New("adapter: already started")

	// ErrNotStarted is returned by Stop when the adapter has not been started.
	ErrNotStarted = errors.New("adapter: not started")
)

// Adapter defines the interface that all tunnel adapters must implement.
// An adapter wraps an existing tunnel client (frp, ngrok, etc.) and
// intercepts connection events for policy evaluation and capture.
//
// Implementations must be safe for concurrent use. The OnConnection handler
// may be called concurrently from multiple goroutines for different connections,
// but events for the same ConnectionID are delivered sequentially.
type Adapter interface {
	// Start begins the adapter and underlying tunnel client.
	// It runs until the context is cancelled or a fatal error occurs.
	// Start should be called in a goroutine; it blocks until shutdown.
	// The adapter handles reconnection internally with exponential backoff.
	//
	// Returns ErrHandlerNotSet if OnConnection has not been called.
	// Returns ErrAlreadyStarted if the adapter is already running.
	// OnConnection cannot be called after Start has been invoked.
	// Start is not idempotent; calling it twice returns an error.
	//
	// No events will be emitted after the context is cancelled.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the adapter and underlying tunnel client.
	// It waits for active connections to drain up to the provided context deadline.
	// If ctx is cancelled, Stop returns immediately with ctx.Err().
	//
	// Returns ErrNotStarted if the adapter has not been started.
	// Stop is idempotent; calling it multiple times after a successful stop
	// returns ErrNotStarted.
	//
	// After Stop returns, no more events will be emitted to the handler.
	Stop(ctx context.Context) error

	// OnConnection registers a handler that will be called for each
	// connection event. The handler's Decision determines whether
	// the connection is allowed or denied.
	//
	// The handler may be called concurrently from multiple goroutines
	// for different connections. Events for the same ConnectionID are
	// delivered sequentially in the order: connect → requests → disconnect.
	//
	// Only one handler can be registered. Calling OnConnection again
	// replaces the previous handler. Must be called before Start.
	OnConnection(handler ConnectionHandler)

	// Status returns the current status of the adapter.
	// Safe to call concurrently with other methods.
	Status() Status
}

// ConnectionHandler is a function that processes connection events
// and returns a decision on whether to allow or deny the connection.
//
// Handlers must be safe for concurrent calls and should return promptly
// to avoid blocking the adapter. The handler should not modify the event.
type ConnectionHandler func(event ConnectionEvent) Decision

// EventType represents the type of connection event.
type EventType string

const (
	// EventConnect indicates a new connection is being established.
	// This is always the first event for a connection.
	EventConnect EventType = "connect"

	// EventDisconnect indicates an existing connection has closed.
	// This is always the last event for a connection.
	EventDisconnect EventType = "disconnect"

	// EventRequest indicates a request within an existing connection.
	// Only emitted if the EventConnect was allowed.
	EventRequest EventType = "request"
)

// ConnectionEvent represents a connection event from the tunnel adapter.
// It contains metadata about the connection for policy evaluation and capture.
//
// All fields are immutable after creation. Handlers must not modify the event
// or any of its nested structures (Request, Headers, Metadata).
type ConnectionEvent struct {
	// ID is a unique identifier for this event.
	ID string `json:"id"`

	// ConnectionID links related events (connect, request, disconnect).
	// All events for the same connection share the same ConnectionID.
	ConnectionID string `json:"connection_id"`

	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`

	// Type indicates what kind of event this is.
	Type EventType `json:"type"`

	// Target is the destination service or resource being accessed.
	// Format depends on the adapter (e.g., "web.example.com:8080").
	Target string `json:"target"`

	// User is the identity of the user making the connection.
	// May be empty if authentication is not configured.
	User string `json:"user,omitempty"`

	// SourceIP is the source IP address (without port).
	SourceIP string `json:"source_ip"`

	// SourcePort is the source port number.
	SourcePort int `json:"source_port"`

	// DestIP is the destination IP address (without port).
	DestIP string `json:"dest_ip"`

	// DestPort is the destination port number.
	DestPort int `json:"dest_port"`

	// Protocol is the connection protocol (e.g., "tcp", "http", "https").
	Protocol string `json:"protocol"`

	// Request contains details for EventRequest type events.
	// Nil for EventConnect and EventDisconnect.
	// The Request struct and its Headers map are immutable.
	Request *RequestInfo `json:"request,omitempty"`

	// Metadata contains adapter-specific additional information.
	// Keys and values depend on the adapter implementation.
	// This map is immutable; handlers must not modify it.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// RequestInfo contains details about a request within a connection.
// Used only for EventRequest type events.
//
// All fields are immutable after creation. The Headers map is a copy
// made by the adapter; handlers must not modify it.
//
// Note: Headers uses map[string]string for simplicity and serialization.
// Multi-value headers are joined with ", " per HTTP specification.
// Header keys are canonicalized (e.g., "Content-Type", not "content-type").
type RequestInfo struct {
	// Method is the request method (e.g., "GET", "POST" for HTTP).
	Method string `json:"method"`

	// Path is the request path (e.g., "/api/v1/users").
	Path string `json:"path"`

	// Host is the request host header.
	Host string `json:"host"`

	// TLS indicates whether the request uses TLS.
	TLS bool `json:"tls"`

	// Headers contains request headers.
	// This is a copy made by the adapter; sensitive headers may be filtered.
	// Multi-value headers are joined with ", " per HTTP specification.
	// Keys are canonicalized (e.g., "Content-Type", not "content-type").
	Headers map[string]string `json:"headers,omitempty"`
}

// Decision represents the result of evaluating a connection event
// against the policy engine.
type Decision struct {
	// Allowed indicates whether the connection should be permitted.
	Allowed bool `json:"allowed"`

	// Reason explains why the decision was made.
	// For allowed connections, this may describe the matching policy.
	// For denied connections, this explains the denial reason.
	Reason string `json:"reason"`

	// Policy is the name of the policy rule that matched, if any.
	// Empty if no specific policy matched (e.g., default deny).
	Policy string `json:"policy,omitempty"`
}

// Status represents the current state of an adapter.
// Adapters should compute UptimeSeconds from StartedAt at read time
// to ensure consistency.
type Status struct {
	// Type is the adapter type (e.g., "frp", "ngrok").
	Type string `json:"type"`

	// Connected indicates whether the adapter is connected to its backend.
	Connected bool `json:"connected"`

	// StartedAt is when the adapter was started.
	StartedAt time.Time `json:"started_at"`

	// UptimeSeconds is how long the adapter has been running.
	// Adapters should compute this from StartedAt at read time.
	UptimeSeconds int64 `json:"uptime_seconds"`

	// ActiveConnections is the current number of active connections.
	ActiveConnections int `json:"active_connections"`

	// ReconnectAttempts is the number of reconnection attempts since start.
	ReconnectAttempts int `json:"reconnect_attempts"`

	// BackendEndpoint is the backend server endpoint (e.g., "frps.example.com:7000").
	BackendEndpoint string `json:"backend_endpoint"`

	// LastError is the most recent error message, if any.
	// Adapters should convert errors to strings for serialization.
	LastError string `json:"last_error,omitempty"`

	// LastErrorAt is when the last error occurred.
	// Zero value if no error has occurred.
	LastErrorAt time.Time `json:"last_error_at,omitempty"`
}

// Compile-time interface satisfaction checks.
// Implementations should include similar checks:
//
//	var _ adapter.Adapter = (*FrpAdapter)(nil)
