package frp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/forest6511/cloudtap/pkg/adapter"
)

func TestDefaultWebhookConfig(t *testing.T) {
	cfg := DefaultWebhookConfig()

	if cfg.ListenAddr != "127.0.0.1:9000" {
		t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, "127.0.0.1:9000")
	}
	if cfg.Path != "/handler" {
		t.Errorf("Path = %q, want %q", cfg.Path, "/handler")
	}
	if cfg.ReadTimeout != 10*time.Second {
		t.Errorf("ReadTimeout = %v, want %v", cfg.ReadTimeout, 10*time.Second)
	}
	if cfg.WriteTimeout != 10*time.Second {
		t.Errorf("WriteTimeout = %v, want %v", cfg.WriteTimeout, 10*time.Second)
	}
}

func TestWebhookConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  WebhookConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: WebhookConfig{
				ListenAddr: "127.0.0.1:9000",
				Path:       "/handler",
			},
			wantErr: false,
		},
		{
			name: "missing ListenAddr",
			config: WebhookConfig{
				Path: "/handler",
			},
			wantErr: true,
		},
		{
			name: "missing Path",
			config: WebhookConfig{
				ListenAddr: "127.0.0.1:9000",
			},
			wantErr: true,
		},
		{
			name: "path without leading slash",
			config: WebhookConfig{
				ListenAddr: "127.0.0.1:9000",
				Path:       "handler",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewWebhookServer(t *testing.T) {
	cfg := DefaultWebhookConfig()
	s := NewWebhookServer(cfg, nil)

	if s == nil {
		t.Fatal("NewWebhookServer() returned nil")
	}
	if s.config.ListenAddr != cfg.ListenAddr {
		t.Errorf("ListenAddr = %q, want %q", s.config.ListenAddr, cfg.ListenAddr)
	}
}

func TestWebhookServer_HandleLogin(t *testing.T) {
	s := NewWebhookServer(DefaultWebhookConfig(), nil)

	var receivedEvent adapter.ConnectionEvent
	var handlerCalled atomic.Bool

	s.SetHandler(func(event adapter.ConnectionEvent) adapter.Decision {
		handlerCalled.Store(true)
		receivedEvent = event
		return adapter.Decision{Allowed: true, Reason: "test allowed"}
	})

	// Create login request
	loginContent := LoginContent{
		Version:       "0.52.0",
		Hostname:      "test-host",
		OS:            "linux",
		Arch:          "amd64",
		User:          "testuser",
		Timestamp:     time.Now().Unix(),
		PrivilegeKey:  "secret",
		RunID:         "run-123",
		PoolCount:     1,
		Metas:         map[string]string{"env": "test"},
		ClientAddress: "192.168.1.100:45678",
	}

	contentBytes, _ := json.Marshal(loginContent)
	req := PluginRequest{
		Version: "0.1.0",
		Op:      OpLogin,
		Content: contentBytes,
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/handler?op=Login", bytes.NewReader(body))
	httpReq.Header.Set("X-Frp-Reqid", "req-123")

	w := httptest.NewRecorder()
	s.handleRequest(w, httpReq)

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	var resp PluginResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Reject {
		t.Error("response should not reject")
	}

	// Check handler was called
	if !handlerCalled.Load() {
		t.Error("handler was not called")
	}

	// Check event fields
	if receivedEvent.Type != adapter.EventConnect {
		t.Errorf("event Type = %q, want %q", receivedEvent.Type, adapter.EventConnect)
	}
	if receivedEvent.User != "testuser" {
		t.Errorf("event User = %q, want %q", receivedEvent.User, "testuser")
	}
	if receivedEvent.SourceIP != "192.168.1.100" {
		t.Errorf("event SourceIP = %q, want %q", receivedEvent.SourceIP, "192.168.1.100")
	}
	if receivedEvent.SourcePort != 45678 {
		t.Errorf("event SourcePort = %d, want %d", receivedEvent.SourcePort, 45678)
	}
	if receivedEvent.Metadata["frp.op"] != OpLogin {
		t.Errorf("event Metadata[frp.op] = %q, want %q", receivedEvent.Metadata["frp.op"], OpLogin)
	}
}

func TestWebhookServer_HandleLogin_Reject(t *testing.T) {
	s := NewWebhookServer(DefaultWebhookConfig(), nil)

	s.SetHandler(func(event adapter.ConnectionEvent) adapter.Decision {
		return adapter.Decision{Allowed: false, Reason: "unauthorized user"}
	})

	loginContent := LoginContent{
		Version:       "0.52.0",
		User:          "baduser",
		RunID:         "run-456",
		ClientAddress: "10.0.0.1:12345",
	}

	contentBytes, _ := json.Marshal(loginContent)
	req := PluginRequest{
		Version: "0.1.0",
		Op:      OpLogin,
		Content: contentBytes,
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/handler?op=Login", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleRequest(w, httpReq)

	var resp PluginResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if !resp.Reject {
		t.Error("response should reject")
	}
	if resp.RejectReason != "unauthorized user" {
		t.Errorf("RejectReason = %q, want %q", resp.RejectReason, "unauthorized user")
	}
}

func TestWebhookServer_HandleNewProxy(t *testing.T) {
	s := NewWebhookServer(DefaultWebhookConfig(), nil)

	var receivedEvent adapter.ConnectionEvent
	s.SetHandler(func(event adapter.ConnectionEvent) adapter.Decision {
		receivedEvent = event
		return adapter.Decision{Allowed: true}
	})

	proxyContent := NewProxyContent{
		User: UserInfo{
			User:  "testuser",
			Metas: map[string]string{"team": "devops"},
			RunID: "run-123",
		},
		ProxyName:      "web-proxy",
		ProxyType:      "tcp",
		UseEncryption:  true,
		UseCompression: true,
		RemotePort:     8080,
		Metas:          map[string]string{"app": "web"},
	}

	contentBytes, _ := json.Marshal(proxyContent)
	req := PluginRequest{
		Version: "0.1.0",
		Op:      OpNewProxy,
		Content: contentBytes,
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/handler?op=NewProxy", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleRequest(w, httpReq)

	var resp PluginResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Reject {
		t.Error("response should not reject")
	}

	// Check event fields
	if receivedEvent.Type != adapter.EventConnect {
		t.Errorf("event Type = %q, want %q", receivedEvent.Type, adapter.EventConnect)
	}
	if receivedEvent.User != "testuser" {
		t.Errorf("event User = %q, want %q", receivedEvent.User, "testuser")
	}
	if receivedEvent.Protocol != "tcp" {
		t.Errorf("event Protocol = %q, want %q", receivedEvent.Protocol, "tcp")
	}
	if receivedEvent.DestPort != 8080 {
		t.Errorf("event DestPort = %d, want %d", receivedEvent.DestPort, 8080)
	}
	if receivedEvent.Metadata["frp.proxy_name"] != "web-proxy" {
		t.Errorf("event Metadata[frp.proxy_name] = %q, want %q", receivedEvent.Metadata["frp.proxy_name"], "web-proxy")
	}

	// Check connection tracking
	if s.ActiveConnections() != 1 {
		t.Errorf("ActiveConnections() = %d, want 1", s.ActiveConnections())
	}
}

func TestWebhookServer_HandleCloseProxy(t *testing.T) {
	s := NewWebhookServer(DefaultWebhookConfig(), nil)

	var disconnectEventReceived bool
	s.SetHandler(func(event adapter.ConnectionEvent) adapter.Decision {
		if event.Type == adapter.EventDisconnect {
			disconnectEventReceived = true
		}
		return adapter.Decision{Allowed: true}
	})

	// First, create a proxy
	proxyContent := NewProxyContent{
		User: UserInfo{
			User:  "testuser",
			RunID: "run-123",
		},
		ProxyName: "test-proxy",
		ProxyType: "tcp",
	}
	contentBytes, _ := json.Marshal(proxyContent)
	req := PluginRequest{Op: OpNewProxy, Content: contentBytes}
	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/handler?op=NewProxy", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleRequest(w, httpReq)

	if s.ActiveConnections() != 1 {
		t.Fatalf("ActiveConnections() = %d, want 1", s.ActiveConnections())
	}

	// Now close it
	closeContent := CloseProxyContent{
		User: UserInfo{
			User:  "testuser",
			RunID: "run-123",
		},
		ProxyName: "test-proxy",
	}
	contentBytes, _ = json.Marshal(closeContent)
	req = PluginRequest{Op: OpCloseProxy, Content: contentBytes}
	body, _ = json.Marshal(req)
	httpReq = httptest.NewRequest(http.MethodPost, "/handler?op=CloseProxy", bytes.NewReader(body))
	w = httptest.NewRecorder()
	s.handleRequest(w, httpReq)

	var resp PluginResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Reject {
		t.Error("close proxy should not reject")
	}

	if !disconnectEventReceived {
		t.Error("disconnect event was not sent")
	}

	if s.ActiveConnections() != 0 {
		t.Errorf("ActiveConnections() = %d, want 0", s.ActiveConnections())
	}
}

func TestWebhookServer_HandleNewUserConn(t *testing.T) {
	s := NewWebhookServer(DefaultWebhookConfig(), nil)

	var receivedEvent adapter.ConnectionEvent
	s.SetHandler(func(event adapter.ConnectionEvent) adapter.Decision {
		receivedEvent = event
		return adapter.Decision{Allowed: true}
	})

	userConnContent := NewUserConnContent{
		User: UserInfo{
			User:  "testuser",
			RunID: "run-123",
		},
		ProxyName:  "web-proxy",
		ProxyType:  "http",
		RemoteAddr: "203.0.113.50:54321",
	}

	contentBytes, _ := json.Marshal(userConnContent)
	req := PluginRequest{
		Version: "0.1.0",
		Op:      OpNewUserConn,
		Content: contentBytes,
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/handler?op=NewUserConn", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleRequest(w, httpReq)

	var resp PluginResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Reject {
		t.Error("response should not reject")
	}

	// Check event fields
	if receivedEvent.Type != adapter.EventRequest {
		t.Errorf("event Type = %q, want %q", receivedEvent.Type, adapter.EventRequest)
	}
	if receivedEvent.SourceIP != "203.0.113.50" {
		t.Errorf("event SourceIP = %q, want %q", receivedEvent.SourceIP, "203.0.113.50")
	}
	if receivedEvent.SourcePort != 54321 {
		t.Errorf("event SourcePort = %d, want %d", receivedEvent.SourcePort, 54321)
	}
	if receivedEvent.Protocol != "http" {
		t.Errorf("event Protocol = %q, want %q", receivedEvent.Protocol, "http")
	}
}

func TestWebhookServer_HandlePing(t *testing.T) {
	s := NewWebhookServer(DefaultWebhookConfig(), nil)

	pingContent := PingContent{
		User: UserInfo{
			User:  "testuser",
			RunID: "run-123",
		},
		Timestamp:    time.Now().Unix(),
		PrivilegeKey: "secret",
	}

	contentBytes, _ := json.Marshal(pingContent)
	req := PluginRequest{
		Version: "0.1.0",
		Op:      OpPing,
		Content: contentBytes,
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/handler?op=Ping", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleRequest(w, httpReq)

	var resp PluginResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Reject {
		t.Error("ping should not reject")
	}
	if !resp.Unchange {
		t.Error("ping response should have unchange=true")
	}
}

func TestWebhookServer_HandleNewWorkConn(t *testing.T) {
	s := NewWebhookServer(DefaultWebhookConfig(), nil)

	workConnContent := NewWorkConnContent{
		User: UserInfo{
			User:  "testuser",
			RunID: "run-123",
		},
		RunID:        "run-123",
		Timestamp:    time.Now().Unix(),
		PrivilegeKey: "secret",
	}

	contentBytes, _ := json.Marshal(workConnContent)
	req := PluginRequest{
		Version: "0.1.0",
		Op:      OpNewWorkConn,
		Content: contentBytes,
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/handler?op=NewWorkConn", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleRequest(w, httpReq)

	var resp PluginResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Reject {
		t.Error("work conn should not reject")
	}
}

func TestWebhookServer_MethodNotAllowed(t *testing.T) {
	s := NewWebhookServer(DefaultWebhookConfig(), nil)

	httpReq := httptest.NewRequest(http.MethodGet, "/handler?op=Login", nil)
	w := httptest.NewRecorder()
	s.handleRequest(w, httpReq)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestWebhookServer_InvalidJSON(t *testing.T) {
	s := NewWebhookServer(DefaultWebhookConfig(), nil)

	httpReq := httptest.NewRequest(http.MethodPost, "/handler?op=Login", strings.NewReader("invalid json"))
	w := httptest.NewRecorder()
	s.handleRequest(w, httpReq)

	var resp PluginResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if !resp.Reject {
		t.Error("invalid JSON should be rejected")
	}
}

func TestWebhookServer_UnknownOperation(t *testing.T) {
	s := NewWebhookServer(DefaultWebhookConfig(), nil)

	req := PluginRequest{
		Version: "0.1.0",
		Op:      "UnknownOp",
		Content: []byte("{}"),
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/handler?op=UnknownOp", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleRequest(w, httpReq)

	var resp PluginResponse
	json.NewDecoder(w.Body).Decode(&resp)

	// Unknown operations should pass through
	if resp.Reject {
		t.Error("unknown operation should not reject")
	}
	if !resp.Unchange {
		t.Error("unknown operation should have unchange=true")
	}
}

func TestWebhookServer_NoHandler(t *testing.T) {
	s := NewWebhookServer(DefaultWebhookConfig(), nil)
	// Don't set a handler

	loginContent := LoginContent{
		User:          "testuser",
		RunID:         "run-123",
		ClientAddress: "192.168.1.1:1234",
	}

	contentBytes, _ := json.Marshal(loginContent)
	req := PluginRequest{Op: OpLogin, Content: contentBytes}
	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/handler?op=Login", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleRequest(w, httpReq)

	var resp PluginResponse
	json.NewDecoder(w.Body).Decode(&resp)

	// Without handler, should allow but not change
	if resp.Reject {
		t.Error("without handler should not reject")
	}
	if !resp.Unchange {
		t.Error("without handler should have unchange=true")
	}
}

func TestWebhookServer_StartStop(t *testing.T) {
	cfg := WebhookConfig{
		ListenAddr:   "127.0.0.1:0", // Use port 0 for random available port
		Path:         "/handler",
		ReadTimeout:  time.Second,
		WriteTimeout: time.Second,
	}
	s := NewWebhookServer(cfg, nil)

	ctx, cancel := context.WithCancel(context.Background())

	// Start in goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- s.Start(ctx)
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Check server is running
	addr := s.Addr()
	if addr == "" {
		t.Fatal("server address is empty")
	}

	// Make a request to verify it's working
	resp, err := http.Get("http://" + addr + "/handler")
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	resp.Body.Close()

	// Stop the server
	cancel()

	// Wait for server to stop
	select {
	case err := <-errChan:
		if err != nil {
			t.Errorf("Start() returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("server did not stop in time")
	}
}

func TestWebhookServer_ConcurrentRequests(t *testing.T) {
	s := NewWebhookServer(DefaultWebhookConfig(), nil)

	var callCount atomic.Int32
	s.SetHandler(func(event adapter.ConnectionEvent) adapter.Decision {
		callCount.Add(1)
		return adapter.Decision{Allowed: true}
	})

	// Create login content
	loginContent := LoginContent{
		User:          "testuser",
		RunID:         "run-123",
		ClientAddress: "192.168.1.1:1234",
	}
	contentBytes, _ := json.Marshal(loginContent)
	req := PluginRequest{Op: OpLogin, Content: contentBytes}
	body, _ := json.Marshal(req)

	// Make concurrent requests
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			httpReq := httptest.NewRequest(http.MethodPost, "/handler?op=Login", bytes.NewReader(body))
			w := httptest.NewRecorder()
			s.handleRequest(w, httpReq)
		}()
	}
	wg.Wait()

	if callCount.Load() != 100 {
		t.Errorf("handler called %d times, want 100", callCount.Load())
	}
}

func TestBuildTarget(t *testing.T) {
	tests := []struct {
		name     string
		proxy    NewProxyContent
		expected string
	}{
		{
			name: "tcp with port",
			proxy: NewProxyContent{
				ProxyType:  "tcp",
				RemotePort: 8080,
			},
			expected: ":8080",
		},
		{
			name: "tcp without port",
			proxy: NewProxyContent{
				ProxyType: "tcp",
				ProxyName: "my-proxy",
			},
			expected: "my-proxy",
		},
		{
			name: "http with custom domains",
			proxy: NewProxyContent{
				ProxyType:     "http",
				CustomDomains: []string{"example.com", "www.example.com"},
			},
			expected: "example.com",
		},
		{
			name: "http with subdomain",
			proxy: NewProxyContent{
				ProxyType: "https",
				Subdomain: "api",
			},
			expected: "api",
		},
		{
			name: "stcp",
			proxy: NewProxyContent{
				ProxyType: "stcp",
				ProxyName: "secret-proxy",
			},
			expected: "secret-proxy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildTarget(tt.proxy)
			if result != tt.expected {
				t.Errorf("buildTarget() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestParseAddr(t *testing.T) {
	tests := []struct {
		addr     string
		wantIP   string
		wantPort int
	}{
		{"192.168.1.1:8080", "192.168.1.1", 8080},
		{"10.0.0.1:443", "10.0.0.1", 443},
		{"[::1]:9090", "::1", 9090},
		{"192.168.1.1", "192.168.1.1", 0}, // No port
		{"", "", 0},                       // Empty
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			ip, port := parseAddr(tt.addr)
			if ip != tt.wantIP {
				t.Errorf("parseAddr(%q) IP = %q, want %q", tt.addr, ip, tt.wantIP)
			}
			if port != tt.wantPort {
				t.Errorf("parseAddr(%q) port = %d, want %d", tt.addr, port, tt.wantPort)
			}
		})
	}
}
