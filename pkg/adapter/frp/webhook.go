// Package frp provides an adapter that wraps the frp client for cloudtap.
package frp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/forest6511/cloudtap/pkg/adapter"
	"github.com/google/uuid"
)

// Plugin operation types from frp.
const (
	OpLogin       = "Login"
	OpNewProxy    = "NewProxy"
	OpCloseProxy  = "CloseProxy"
	OpPing        = "Ping"
	OpNewWorkConn = "NewWorkConn"
	OpNewUserConn = "NewUserConn"
)

// WebhookConfig holds configuration for the webhook server.
type WebhookConfig struct {
	// ListenAddr is the address to listen on for webhook requests.
	// Default: "127.0.0.1:9000"
	ListenAddr string `json:"listen_addr"`

	// Path is the URL path for the webhook handler.
	// Default: "/handler"
	Path string `json:"path"`

	// ReadTimeout is the maximum duration for reading the request.
	// Default: 10 seconds.
	ReadTimeout time.Duration `json:"read_timeout"`

	// WriteTimeout is the maximum duration for writing the response.
	// Default: 10 seconds.
	WriteTimeout time.Duration `json:"write_timeout"`
}

// DefaultWebhookConfig returns a WebhookConfig with sensible defaults.
func DefaultWebhookConfig() WebhookConfig {
	return WebhookConfig{
		ListenAddr:   "127.0.0.1:9000",
		Path:         "/handler",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
}

// Validate checks if the webhook configuration is valid.
func (c WebhookConfig) Validate() error {
	if c.ListenAddr == "" {
		return fmt.Errorf("frp: webhook ListenAddr is required")
	}
	if c.Path == "" {
		return fmt.Errorf("frp: webhook Path is required")
	}
	if !strings.HasPrefix(c.Path, "/") {
		return fmt.Errorf("frp: webhook Path must start with /")
	}
	return nil
}

// PluginRequest represents the incoming request from frp server plugin.
type PluginRequest struct {
	Version string          `json:"version"`
	Op      string          `json:"op"`
	Content json.RawMessage `json:"content"`
}

// PluginResponse represents the response to send back to frp.
type PluginResponse struct {
	Reject       bool            `json:"reject"`
	RejectReason string          `json:"reject_reason,omitempty"`
	Unchange     bool            `json:"unchange,omitempty"`
	Content      json.RawMessage `json:"content,omitempty"`
}

// UserInfo contains user information included in most plugin requests.
type UserInfo struct {
	User  string            `json:"user"`
	Metas map[string]string `json:"metas,omitempty"`
	RunID string            `json:"run_id"`
}

// LoginContent represents the content of a Login operation.
type LoginContent struct {
	Version       string            `json:"version"`
	Hostname      string            `json:"hostname"`
	OS            string            `json:"os"`
	Arch          string            `json:"arch"`
	User          string            `json:"user"`
	Timestamp     int64             `json:"timestamp"`
	PrivilegeKey  string            `json:"privilege_key"`
	RunID         string            `json:"run_id"`
	PoolCount     int               `json:"pool_count"`
	Metas         map[string]string `json:"metas,omitempty"`
	ClientAddress string            `json:"client_address"`
}

// NewProxyContent represents the content of a NewProxy operation.
type NewProxyContent struct {
	User               UserInfo          `json:"user"`
	ProxyName          string            `json:"proxy_name"`
	ProxyType          string            `json:"proxy_type"`
	UseEncryption      bool              `json:"use_encryption"`
	UseCompression     bool              `json:"use_compression"`
	BandwidthLimit     string            `json:"bandwidth_limit,omitempty"`
	BandwidthLimitMode string            `json:"bandwidth_limit_mode,omitempty"`
	Group              string            `json:"group,omitempty"`
	GroupKey           string            `json:"group_key,omitempty"`
	RemotePort         int               `json:"remote_port,omitempty"`
	CustomDomains      []string          `json:"custom_domains,omitempty"`
	Subdomain          string            `json:"subdomain,omitempty"`
	Locations          []string          `json:"locations,omitempty"`
	HTTPUser           string            `json:"http_user,omitempty"`
	HTTPPwd            string            `json:"http_pwd,omitempty"`
	HostHeaderRewrite  string            `json:"host_header_rewrite,omitempty"`
	Headers            map[string]string `json:"headers,omitempty"`
	SK                 string            `json:"sk,omitempty"`
	Multiplexer        string            `json:"multiplexer,omitempty"`
	Metas              map[string]string `json:"metas,omitempty"`
}

// CloseProxyContent represents the content of a CloseProxy operation.
type CloseProxyContent struct {
	User      UserInfo `json:"user"`
	ProxyName string   `json:"proxy_name"`
}

// PingContent represents the content of a Ping operation.
type PingContent struct {
	User         UserInfo `json:"user"`
	Timestamp    int64    `json:"timestamp"`
	PrivilegeKey string   `json:"privilege_key"`
}

// NewWorkConnContent represents the content of a NewWorkConn operation.
type NewWorkConnContent struct {
	User         UserInfo `json:"user"`
	RunID        string   `json:"run_id"`
	Timestamp    int64    `json:"timestamp"`
	PrivilegeKey string   `json:"privilege_key"`
}

// NewUserConnContent represents the content of a NewUserConn operation.
type NewUserConnContent struct {
	User       UserInfo `json:"user"`
	ProxyName  string   `json:"proxy_name"`
	ProxyType  string   `json:"proxy_type"`
	RemoteAddr string   `json:"remote_addr"`
}

// WebhookServer handles frp plugin webhook requests.
type WebhookServer struct {
	config  WebhookConfig
	logger  *slog.Logger
	handler adapter.ConnectionHandler

	server   *http.Server
	listener net.Listener

	mu      sync.RWMutex
	started atomic.Bool

	// Connection tracking
	activeConnections sync.Map // map[string]*connectionState
	connectionCount   atomic.Int32
}

// connectionState tracks the state of an active connection.
type connectionState struct {
	connectionID string
	user         string
	proxyName    string
	proxyType    string
	startedAt    time.Time
}

// NewWebhookServer creates a new webhook server.
func NewWebhookServer(cfg WebhookConfig, logger *slog.Logger) *WebhookServer {
	if logger == nil {
		logger = slog.Default()
	}

	// Apply defaults
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = "127.0.0.1:9000"
	}
	if cfg.Path == "" {
		cfg.Path = "/handler"
	}
	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = 10 * time.Second
	}
	if cfg.WriteTimeout == 0 {
		cfg.WriteTimeout = 10 * time.Second
	}

	return &WebhookServer{
		config: cfg,
		logger: logger.With("component", "webhook"),
	}
}

// SetHandler sets the connection handler.
func (s *WebhookServer) SetHandler(handler adapter.ConnectionHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handler = handler
}

// Start starts the webhook server.
func (s *WebhookServer) Start(ctx context.Context) error {
	if err := s.config.Validate(); err != nil {
		return err
	}

	if !s.started.CompareAndSwap(false, true) {
		return fmt.Errorf("frp: webhook server already started")
	}

	// Create listener
	listener, err := net.Listen("tcp", s.config.ListenAddr)
	if err != nil {
		s.started.Store(false)
		return fmt.Errorf("frp: failed to listen on %s: %w", s.config.ListenAddr, err)
	}
	s.listener = listener

	// Create HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc(s.config.Path, s.handleRequest)

	s.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
	}

	s.logger.Info("starting webhook server",
		"addr", s.config.ListenAddr,
		"path", s.config.Path,
	)

	// Start server in goroutine
	go func() {
		if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			s.logger.Error("webhook server error", "error", err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()

	return s.shutdown()
}

// Stop stops the webhook server gracefully.
func (s *WebhookServer) Stop(ctx context.Context) error {
	if !s.started.Load() {
		return nil
	}

	return s.shutdown()
}

// shutdown performs the actual server shutdown.
func (s *WebhookServer) shutdown() error {
	s.logger.Info("stopping webhook server")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if s.server != nil {
		if err := s.server.Shutdown(ctx); err != nil {
			s.logger.Error("webhook server shutdown error", "error", err)
			return err
		}
	}

	s.started.Store(false)
	s.logger.Info("webhook server stopped")
	return nil
}

// ActiveConnections returns the current number of active connections.
func (s *WebhookServer) ActiveConnections() int {
	return int(s.connectionCount.Load())
}

// Addr returns the listener address, useful for testing.
func (s *WebhookServer) Addr() string {
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return ""
}

// handleRequest handles incoming webhook requests from frp.
func (s *WebhookServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse operation from query parameter
	op := r.URL.Query().Get("op")
	reqID := r.Header.Get("X-Frp-Reqid")

	s.logger.Debug("received webhook request",
		"op", op,
		"req_id", reqID,
		"remote_addr", r.RemoteAddr,
	)

	// Parse request body
	var req PluginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logger.Error("failed to decode request", "error", err)
		s.sendResponse(w, PluginResponse{Reject: true, RejectReason: "invalid request body"})
		return
	}

	// Use op from query if not in body
	if req.Op == "" {
		req.Op = op
	}

	// Process the request based on operation type
	resp := s.processRequest(req, reqID)

	s.sendResponse(w, resp)
}

// processRequest processes a plugin request and returns a response.
func (s *WebhookServer) processRequest(req PluginRequest, reqID string) PluginResponse {
	s.mu.RLock()
	handler := s.handler
	s.mu.RUnlock()

	switch req.Op {
	case OpLogin:
		return s.handleLogin(req.Content, handler)
	case OpNewProxy:
		return s.handleNewProxy(req.Content, handler)
	case OpCloseProxy:
		return s.handleCloseProxy(req.Content, handler)
	case OpPing:
		return s.handlePing(req.Content, handler)
	case OpNewWorkConn:
		return s.handleNewWorkConn(req.Content, handler)
	case OpNewUserConn:
		return s.handleNewUserConn(req.Content, handler)
	default:
		s.logger.Warn("unknown operation", "op", req.Op)
		return PluginResponse{Reject: false, Unchange: true}
	}
}

// handleLogin processes a Login operation.
func (s *WebhookServer) handleLogin(content json.RawMessage, handler adapter.ConnectionHandler) PluginResponse {
	var login LoginContent
	if err := json.Unmarshal(content, &login); err != nil {
		s.logger.Error("failed to unmarshal login content", "error", err)
		return PluginResponse{Reject: true, RejectReason: "invalid login content"}
	}

	s.logger.Info("login event",
		"user", login.User,
		"client_address", login.ClientAddress,
		"hostname", login.Hostname,
		"run_id", login.RunID,
	)

	if handler == nil {
		return PluginResponse{Reject: false, Unchange: true}
	}

	// Parse client address
	sourceIP, sourcePort := parseAddr(login.ClientAddress)

	event := adapter.ConnectionEvent{
		ID:           uuid.New().String(),
		ConnectionID: login.RunID,
		Timestamp:    time.Unix(login.Timestamp, 0),
		Type:         adapter.EventConnect,
		Target:       login.Hostname,
		User:         login.User,
		SourceIP:     sourceIP,
		SourcePort:   sourcePort,
		Protocol:     "frp",
		Metadata: map[string]string{
			"frp.op":       OpLogin,
			"frp.version":  login.Version,
			"frp.os":       login.OS,
			"frp.arch":     login.Arch,
			"frp.hostname": login.Hostname,
		},
	}

	// Merge user metas
	for k, v := range login.Metas {
		event.Metadata["meta."+k] = v
	}

	decision := handler(event)

	if !decision.Allowed {
		return PluginResponse{Reject: true, RejectReason: decision.Reason}
	}

	return PluginResponse{Reject: false, Unchange: true}
}

// handleNewProxy processes a NewProxy operation.
func (s *WebhookServer) handleNewProxy(content json.RawMessage, handler adapter.ConnectionHandler) PluginResponse {
	var proxy NewProxyContent
	if err := json.Unmarshal(content, &proxy); err != nil {
		s.logger.Error("failed to unmarshal new proxy content", "error", err)
		return PluginResponse{Reject: true, RejectReason: "invalid proxy content"}
	}

	s.logger.Info("new proxy event",
		"user", proxy.User.User,
		"proxy_name", proxy.ProxyName,
		"proxy_type", proxy.ProxyType,
		"remote_port", proxy.RemotePort,
	)

	if handler == nil {
		return PluginResponse{Reject: false, Unchange: true}
	}

	// Build target based on proxy type
	target := buildTarget(proxy)

	event := adapter.ConnectionEvent{
		ID:           uuid.New().String(),
		ConnectionID: proxy.User.RunID,
		Timestamp:    time.Now(),
		Type:         adapter.EventConnect,
		Target:       target,
		User:         proxy.User.User,
		Protocol:     proxy.ProxyType,
		DestPort:     proxy.RemotePort,
		Metadata: map[string]string{
			"frp.op":              OpNewProxy,
			"frp.proxy_name":      proxy.ProxyName,
			"frp.proxy_type":      proxy.ProxyType,
			"frp.use_encryption":  fmt.Sprintf("%t", proxy.UseEncryption),
			"frp.use_compression": fmt.Sprintf("%t", proxy.UseCompression),
		},
	}

	// Add domains for HTTP/HTTPS proxies
	if len(proxy.CustomDomains) > 0 {
		event.Metadata["frp.custom_domains"] = strings.Join(proxy.CustomDomains, ",")
	}
	if proxy.Subdomain != "" {
		event.Metadata["frp.subdomain"] = proxy.Subdomain
	}

	// Merge user and proxy metas
	for k, v := range proxy.User.Metas {
		event.Metadata["user_meta."+k] = v
	}
	for k, v := range proxy.Metas {
		event.Metadata["proxy_meta."+k] = v
	}

	decision := handler(event)

	if !decision.Allowed {
		return PluginResponse{Reject: true, RejectReason: decision.Reason}
	}

	// Track the proxy connection
	state := &connectionState{
		connectionID: proxy.User.RunID + "/" + proxy.ProxyName,
		user:         proxy.User.User,
		proxyName:    proxy.ProxyName,
		proxyType:    proxy.ProxyType,
		startedAt:    time.Now(),
	}
	s.activeConnections.Store(state.connectionID, state)
	s.connectionCount.Add(1)

	return PluginResponse{Reject: false, Unchange: true}
}

// handleCloseProxy processes a CloseProxy operation.
func (s *WebhookServer) handleCloseProxy(content json.RawMessage, handler adapter.ConnectionHandler) PluginResponse {
	var closeProxy CloseProxyContent
	if err := json.Unmarshal(content, &closeProxy); err != nil {
		s.logger.Error("failed to unmarshal close proxy content", "error", err)
		return PluginResponse{Reject: false, Unchange: true}
	}

	s.logger.Info("close proxy event",
		"user", closeProxy.User.User,
		"proxy_name", closeProxy.ProxyName,
	)

	connID := closeProxy.User.RunID + "/" + closeProxy.ProxyName

	// Remove from tracking
	if _, loaded := s.activeConnections.LoadAndDelete(connID); loaded {
		s.connectionCount.Add(-1)
	}

	if handler == nil {
		return PluginResponse{Reject: false, Unchange: true}
	}

	event := adapter.ConnectionEvent{
		ID:           uuid.New().String(),
		ConnectionID: closeProxy.User.RunID,
		Timestamp:    time.Now(),
		Type:         adapter.EventDisconnect,
		User:         closeProxy.User.User,
		Metadata: map[string]string{
			"frp.op":         OpCloseProxy,
			"frp.proxy_name": closeProxy.ProxyName,
		},
	}

	// Call handler but ignore decision for close events
	_ = handler(event)

	return PluginResponse{Reject: false, Unchange: true}
}

// handlePing processes a Ping operation.
func (s *WebhookServer) handlePing(content json.RawMessage, handler adapter.ConnectionHandler) PluginResponse {
	var ping PingContent
	if err := json.Unmarshal(content, &ping); err != nil {
		s.logger.Error("failed to unmarshal ping content", "error", err)
		return PluginResponse{Reject: false, Unchange: true}
	}

	s.logger.Debug("ping event",
		"user", ping.User.User,
		"run_id", ping.User.RunID,
	)

	// Ping is just a heartbeat, always allow
	return PluginResponse{Reject: false, Unchange: true}
}

// handleNewWorkConn processes a NewWorkConn operation.
func (s *WebhookServer) handleNewWorkConn(content json.RawMessage, handler adapter.ConnectionHandler) PluginResponse {
	var workConn NewWorkConnContent
	if err := json.Unmarshal(content, &workConn); err != nil {
		s.logger.Error("failed to unmarshal new work conn content", "error", err)
		return PluginResponse{Reject: true, RejectReason: "invalid work conn content"}
	}

	s.logger.Debug("new work conn event",
		"user", workConn.User.User,
		"run_id", workConn.RunID,
	)

	// Work connections are internal frp connections, always allow
	return PluginResponse{Reject: false, Unchange: true}
}

// handleNewUserConn processes a NewUserConn operation.
func (s *WebhookServer) handleNewUserConn(content json.RawMessage, handler adapter.ConnectionHandler) PluginResponse {
	var userConn NewUserConnContent
	if err := json.Unmarshal(content, &userConn); err != nil {
		s.logger.Error("failed to unmarshal new user conn content", "error", err)
		return PluginResponse{Reject: true, RejectReason: "invalid user conn content"}
	}

	s.logger.Info("new user conn event",
		"user", userConn.User.User,
		"proxy_name", userConn.ProxyName,
		"proxy_type", userConn.ProxyType,
		"remote_addr", userConn.RemoteAddr,
	)

	if handler == nil {
		return PluginResponse{Reject: false, Unchange: true}
	}

	// Parse remote address
	sourceIP, sourcePort := parseAddr(userConn.RemoteAddr)

	event := adapter.ConnectionEvent{
		ID:           uuid.New().String(),
		ConnectionID: userConn.User.RunID + "/" + userConn.ProxyName + "/" + uuid.New().String()[:8],
		Timestamp:    time.Now(),
		Type:         adapter.EventRequest,
		Target:       userConn.ProxyName,
		User:         userConn.User.User,
		SourceIP:     sourceIP,
		SourcePort:   sourcePort,
		Protocol:     userConn.ProxyType,
		Metadata: map[string]string{
			"frp.op":         OpNewUserConn,
			"frp.proxy_name": userConn.ProxyName,
			"frp.proxy_type": userConn.ProxyType,
		},
	}

	// Merge user metas
	for k, v := range userConn.User.Metas {
		event.Metadata["user_meta."+k] = v
	}

	decision := handler(event)

	if !decision.Allowed {
		return PluginResponse{Reject: true, RejectReason: decision.Reason}
	}

	return PluginResponse{Reject: false, Unchange: true}
}

// sendResponse writes a JSON response to the client.
func (s *WebhookServer) sendResponse(w http.ResponseWriter, resp PluginResponse) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		s.logger.Error("failed to encode response", "error", err)
	}
}

// parseAddr parses an address string into IP and port.
func parseAddr(addr string) (string, int) {
	if addr == "" {
		return "", 0
	}

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		// Maybe just an IP without port
		return addr, 0
	}

	var port int
	fmt.Sscanf(portStr, "%d", &port)
	return host, port
}

// buildTarget constructs a target string based on proxy configuration.
func buildTarget(proxy NewProxyContent) string {
	switch proxy.ProxyType {
	case "tcp", "udp":
		if proxy.RemotePort > 0 {
			return fmt.Sprintf(":%d", proxy.RemotePort)
		}
		return proxy.ProxyName
	case "http", "https":
		if len(proxy.CustomDomains) > 0 {
			return proxy.CustomDomains[0]
		}
		if proxy.Subdomain != "" {
			return proxy.Subdomain
		}
		return proxy.ProxyName
	case "stcp", "xtcp", "sudp":
		return proxy.ProxyName
	default:
		return proxy.ProxyName
	}
}
