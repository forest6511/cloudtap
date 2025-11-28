package adapter

import (
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestEventTypeConstants(t *testing.T) {
	tests := []struct {
		eventType EventType
		expected  string
	}{
		{EventConnect, "connect"},
		{EventDisconnect, "disconnect"},
		{EventRequest, "request"},
	}

	for _, tt := range tests {
		t.Run(string(tt.eventType), func(t *testing.T) {
			if string(tt.eventType) != tt.expected {
				t.Errorf("EventType = %q, want %q", tt.eventType, tt.expected)
			}
		})
	}
}

func TestConnectionEvent_BasicFields(t *testing.T) {
	now := time.Now()
	event := ConnectionEvent{
		ID:           "evt-123",
		ConnectionID: "conn-456",
		Timestamp:    now,
		Type:         EventConnect,
		Target:       "staging-api:8080",
		User:         "alice@dev-team",
		SourceIP:     "10.0.1.50",
		SourcePort:   54321,
		DestIP:       "192.168.1.100",
		DestPort:     8080,
		Protocol:     "tcp",
		Metadata: map[string]string{
			"proxy_name": "web",
			"client_id":  "client-456",
		},
	}

	if event.ID != "evt-123" {
		t.Errorf("ID = %q, want %q", event.ID, "evt-123")
	}
	if event.ConnectionID != "conn-456" {
		t.Errorf("ConnectionID = %q, want %q", event.ConnectionID, "conn-456")
	}
	if event.Type != EventConnect {
		t.Errorf("Type = %q, want %q", event.Type, EventConnect)
	}
	if event.Target != "staging-api:8080" {
		t.Errorf("Target = %q, want %q", event.Target, "staging-api:8080")
	}
	if event.User != "alice@dev-team" {
		t.Errorf("User = %q, want %q", event.User, "alice@dev-team")
	}
	if event.SourceIP != "10.0.1.50" {
		t.Errorf("SourceIP = %q, want %q", event.SourceIP, "10.0.1.50")
	}
	if event.SourcePort != 54321 {
		t.Errorf("SourcePort = %d, want %d", event.SourcePort, 54321)
	}
	if event.DestIP != "192.168.1.100" {
		t.Errorf("DestIP = %q, want %q", event.DestIP, "192.168.1.100")
	}
	if event.DestPort != 8080 {
		t.Errorf("DestPort = %d, want %d", event.DestPort, 8080)
	}
	if event.Protocol != "tcp" {
		t.Errorf("Protocol = %q, want %q", event.Protocol, "tcp")
	}
	if event.Metadata["proxy_name"] != "web" {
		t.Errorf("Metadata[proxy_name] = %q, want %q", event.Metadata["proxy_name"], "web")
	}
}

func TestConnectionEvent_WithRequestInfo(t *testing.T) {
	event := ConnectionEvent{
		ID:       "evt-req-001",
		Type:     EventRequest,
		Protocol: "https",
		Request: &RequestInfo{
			Method: "POST",
			Path:   "/api/v1/users",
			Host:   "api.example.com",
			TLS:    true,
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
		},
	}

	if event.Request == nil {
		t.Fatal("Request should not be nil for EventRequest")
	}
	if event.Request.Method != "POST" {
		t.Errorf("Request.Method = %q, want %q", event.Request.Method, "POST")
	}
	if event.Request.Path != "/api/v1/users" {
		t.Errorf("Request.Path = %q, want %q", event.Request.Path, "/api/v1/users")
	}
	if event.Request.Host != "api.example.com" {
		t.Errorf("Request.Host = %q, want %q", event.Request.Host, "api.example.com")
	}
	if !event.Request.TLS {
		t.Error("Request.TLS = false, want true")
	}
	if event.Request.Headers["Content-Type"] != "application/json" {
		t.Errorf("Request.Headers[Content-Type] = %q, want %q",
			event.Request.Headers["Content-Type"], "application/json")
	}
}

func TestConnectionEvent_ConnectionCorrelation(t *testing.T) {
	// Simulate a connection lifecycle with correlated events
	connID := "conn-lifecycle-001"
	now := time.Now()

	connectEvent := ConnectionEvent{
		ID:           "evt-001",
		ConnectionID: connID,
		Timestamp:    now,
		Type:         EventConnect,
	}

	requestEvent := ConnectionEvent{
		ID:           "evt-002",
		ConnectionID: connID,
		Timestamp:    now.Add(100 * time.Millisecond),
		Type:         EventRequest,
	}

	disconnectEvent := ConnectionEvent{
		ID:           "evt-003",
		ConnectionID: connID,
		Timestamp:    now.Add(500 * time.Millisecond),
		Type:         EventDisconnect,
	}

	// All events should share the same ConnectionID
	if connectEvent.ConnectionID != requestEvent.ConnectionID {
		t.Error("connect and request events should have same ConnectionID")
	}
	if requestEvent.ConnectionID != disconnectEvent.ConnectionID {
		t.Error("request and disconnect events should have same ConnectionID")
	}

	// Events should be distinguishable by their own ID
	if connectEvent.ID == requestEvent.ID || requestEvent.ID == disconnectEvent.ID {
		t.Error("each event should have unique ID")
	}

	// Verify expected ordering by timestamp
	if !connectEvent.Timestamp.Before(requestEvent.Timestamp) {
		t.Error("connect should be before request")
	}
	if !requestEvent.Timestamp.Before(disconnectEvent.Timestamp) {
		t.Error("request should be before disconnect")
	}
}

func TestDecision(t *testing.T) {
	tests := []struct {
		name     string
		decision Decision
		allowed  bool
	}{
		{
			name: "allowed with policy",
			decision: Decision{
				Allowed: true,
				Reason:  "matched allow-dev-staging policy",
				Policy:  "allow-dev-staging",
			},
			allowed: true,
		},
		{
			name: "denied by default",
			decision: Decision{
				Allowed: false,
				Reason:  "no matching policy, default deny",
				Policy:  "",
			},
			allowed: false,
		},
		{
			name: "denied by explicit policy",
			decision: Decision{
				Allowed: false,
				Reason:  "blocked by deny-external policy",
				Policy:  "deny-external",
			},
			allowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.decision.Allowed != tt.allowed {
				t.Errorf("Allowed = %v, want %v", tt.decision.Allowed, tt.allowed)
			}
		})
	}
}

func TestStatus(t *testing.T) {
	startedAt := time.Now().Add(-1 * time.Hour)
	status := Status{
		Type:              "frp",
		Connected:         true,
		StartedAt:         startedAt,
		UptimeSeconds:     3600,
		ActiveConnections: 5,
		ReconnectAttempts: 2,
		BackendEndpoint:   "frps.example.com:7000",
		LastError:         "",
	}

	if status.Type != "frp" {
		t.Errorf("Type = %q, want %q", status.Type, "frp")
	}
	if !status.Connected {
		t.Error("Connected = false, want true")
	}
	if status.UptimeSeconds != 3600 {
		t.Errorf("UptimeSeconds = %d, want %d", status.UptimeSeconds, 3600)
	}
	if status.ActiveConnections != 5 {
		t.Errorf("ActiveConnections = %d, want %d", status.ActiveConnections, 5)
	}
	if status.ReconnectAttempts != 2 {
		t.Errorf("ReconnectAttempts = %d, want %d", status.ReconnectAttempts, 2)
	}
	if status.BackendEndpoint != "frps.example.com:7000" {
		t.Errorf("BackendEndpoint = %q, want %q", status.BackendEndpoint, "frps.example.com:7000")
	}
	if status.LastError != "" {
		t.Errorf("LastError = %q, want empty", status.LastError)
	}
}

func TestStatus_WithError(t *testing.T) {
	errTime := time.Now()
	status := Status{
		Type:        "frp",
		Connected:   false,
		LastError:   "connection refused",
		LastErrorAt: errTime,
	}

	if status.Connected {
		t.Error("Connected = true, want false when there's an error")
	}
	if status.LastError != "connection refused" {
		t.Errorf("LastError = %q, want %q", status.LastError, "connection refused")
	}
	if status.LastErrorAt.IsZero() {
		t.Error("LastErrorAt should not be zero when there's an error")
	}
}

func TestConnectionHandler_ConcurrentCalls(t *testing.T) {
	var mu sync.Mutex
	callCount := 0

	handler := ConnectionHandler(func(event ConnectionEvent) Decision {
		mu.Lock()
		callCount++
		mu.Unlock()

		if event.User == "admin" {
			return Decision{Allowed: true, Reason: "admin access", Policy: "admin-allow"}
		}
		return Decision{Allowed: false, Reason: "not admin", Policy: ""}
	})

	// Simulate concurrent calls
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			user := "user"
			if idx%10 == 0 {
				user = "admin"
			}
			event := ConnectionEvent{
				ID:   "evt-" + strconv.Itoa(idx),
				User: user,
			}
			_ = handler(event)
		}(i)
	}
	wg.Wait()

	mu.Lock()
	if callCount != 100 {
		t.Errorf("callCount = %d, want 100", callCount)
	}
	mu.Unlock()
}

func TestConnectionHandler_DecisionLogic(t *testing.T) {
	handler := ConnectionHandler(func(event ConnectionEvent) Decision {
		// Check user
		if event.User == "admin" {
			return Decision{Allowed: true, Reason: "admin access", Policy: "admin-allow"}
		}
		// Check source IP for internal network (10.x.x.x)
		if len(event.SourceIP) >= 3 && event.SourceIP[:3] == "10." {
			return Decision{Allowed: true, Reason: "internal network", Policy: "internal-allow"}
		}
		return Decision{Allowed: false, Reason: "default deny", Policy: ""}
	})

	tests := []struct {
		name    string
		event   ConnectionEvent
		allowed bool
	}{
		{
			name:    "admin user allowed",
			event:   ConnectionEvent{User: "admin"},
			allowed: true,
		},
		{
			name: "internal IP allowed",
			event: ConnectionEvent{
				User:     "regular-user",
				SourceIP: "10.0.1.50",
			},
			allowed: true,
		},
		{
			name: "external IP denied",
			event: ConnectionEvent{
				User:     "regular-user",
				SourceIP: "203.0.113.50",
			},
			allowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := handler(tt.event)
			if decision.Allowed != tt.allowed {
				t.Errorf("Allowed = %v, want %v (reason: %s)", decision.Allowed, tt.allowed, decision.Reason)
			}
		})
	}
}

func TestConnectionEvent_JSONSerialization(t *testing.T) {
	now := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	event := ConnectionEvent{
		ID:           "evt-serialize-test",
		ConnectionID: "conn-serialize-test",
		Timestamp:    now,
		Type:         EventRequest,
		Target:       "api.example.com:443",
		User:         "testuser",
		SourceIP:     "192.168.1.100",
		SourcePort:   12345,
		DestIP:       "10.0.0.1",
		DestPort:     443,
		Protocol:     "https",
		Request: &RequestInfo{
			Method: "GET",
			Path:   "/health",
			Host:   "api.example.com",
			TLS:    true,
			Headers: map[string]string{
				"Accept": "application/json",
			},
		},
		Metadata: map[string]string{
			"trace_id": "abc123",
		},
	}

	// Test JSON marshaling
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal ConnectionEvent: %v", err)
	}

	// Test JSON unmarshaling
	var decoded ConnectionEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal ConnectionEvent: %v", err)
	}

	// Verify round-trip
	if decoded.ID != event.ID {
		t.Errorf("ID = %q, want %q", decoded.ID, event.ID)
	}
	if decoded.ConnectionID != event.ConnectionID {
		t.Errorf("ConnectionID = %q, want %q", decoded.ConnectionID, event.ConnectionID)
	}
	if decoded.Type != event.Type {
		t.Errorf("Type = %q, want %q", decoded.Type, event.Type)
	}
	if decoded.Request == nil {
		t.Fatal("Request should not be nil")
	}
	if decoded.Request.Method != "GET" {
		t.Errorf("Request.Method = %q, want %q", decoded.Request.Method, "GET")
	}
	if decoded.Metadata["trace_id"] != "abc123" {
		t.Errorf("Metadata[trace_id] = %q, want %q", decoded.Metadata["trace_id"], "abc123")
	}

	// Verify snake_case field names in JSON
	jsonStr := string(data)
	if !contains(jsonStr, `"connection_id"`) {
		t.Error("JSON should use snake_case: connection_id")
	}
	if !contains(jsonStr, `"source_ip"`) {
		t.Error("JSON should use snake_case: source_ip")
	}
	if !contains(jsonStr, `"source_port"`) {
		t.Error("JSON should use snake_case: source_port")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestSentinelErrors(t *testing.T) {
	// Verify sentinel errors are properly defined
	if ErrHandlerNotSet == nil {
		t.Error("ErrHandlerNotSet should not be nil")
	}
	if ErrAlreadyStarted == nil {
		t.Error("ErrAlreadyStarted should not be nil")
	}
	if ErrNotStarted == nil {
		t.Error("ErrNotStarted should not be nil")
	}

	// Verify errors are distinguishable
	if errors.Is(ErrHandlerNotSet, ErrAlreadyStarted) {
		t.Error("ErrHandlerNotSet should not match ErrAlreadyStarted")
	}
	if errors.Is(ErrAlreadyStarted, ErrNotStarted) {
		t.Error("ErrAlreadyStarted should not match ErrNotStarted")
	}

	// Verify error messages contain meaningful text
	if ErrHandlerNotSet.Error() == "" {
		t.Error("ErrHandlerNotSet should have a message")
	}
	if ErrAlreadyStarted.Error() == "" {
		t.Error("ErrAlreadyStarted should have a message")
	}
	if ErrNotStarted.Error() == "" {
		t.Error("ErrNotStarted should have a message")
	}
}

func TestDecision_JSONSerialization(t *testing.T) {
	decision := Decision{
		Allowed: true,
		Reason:  "matched allow-dev-staging policy",
		Policy:  "allow-dev-staging",
	}

	data, err := json.Marshal(decision)
	if err != nil {
		t.Fatalf("failed to marshal Decision: %v", err)
	}

	var decoded Decision
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal Decision: %v", err)
	}

	if decoded.Allowed != decision.Allowed {
		t.Errorf("Allowed = %v, want %v", decoded.Allowed, decision.Allowed)
	}
	if decoded.Reason != decision.Reason {
		t.Errorf("Reason = %q, want %q", decoded.Reason, decision.Reason)
	}
	if decoded.Policy != decision.Policy {
		t.Errorf("Policy = %q, want %q", decoded.Policy, decision.Policy)
	}
}

func TestStatus_JSONSerialization(t *testing.T) {
	startedAt := time.Date(2025, 1, 15, 9, 0, 0, 0, time.UTC)
	errTime := time.Date(2025, 1, 15, 9, 30, 0, 0, time.UTC)
	status := Status{
		Type:              "frp",
		Connected:         true,
		StartedAt:         startedAt,
		UptimeSeconds:     3600,
		ActiveConnections: 5,
		ReconnectAttempts: 2,
		BackendEndpoint:   "frps.example.com:7000",
		LastError:         "connection timeout",
		LastErrorAt:       errTime,
	}

	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("failed to marshal Status: %v", err)
	}

	var decoded Status
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal Status: %v", err)
	}

	if decoded.Type != status.Type {
		t.Errorf("Type = %q, want %q", decoded.Type, status.Type)
	}
	if decoded.Connected != status.Connected {
		t.Errorf("Connected = %v, want %v", decoded.Connected, status.Connected)
	}
	if decoded.UptimeSeconds != status.UptimeSeconds {
		t.Errorf("UptimeSeconds = %d, want %d", decoded.UptimeSeconds, status.UptimeSeconds)
	}
	if decoded.ActiveConnections != status.ActiveConnections {
		t.Errorf("ActiveConnections = %d, want %d", decoded.ActiveConnections, status.ActiveConnections)
	}

	// Verify snake_case field names in JSON
	jsonStr := string(data)
	if !contains(jsonStr, `"started_at"`) {
		t.Error("JSON should use snake_case: started_at")
	}
	if !contains(jsonStr, `"uptime_seconds"`) {
		t.Error("JSON should use snake_case: uptime_seconds")
	}
	if !contains(jsonStr, `"active_connections"`) {
		t.Error("JSON should use snake_case: active_connections")
	}
	if !contains(jsonStr, `"backend_endpoint"`) {
		t.Error("JSON should use snake_case: backend_endpoint")
	}
}
