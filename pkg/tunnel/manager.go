package tunnel

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

// Target describes a tunnelable destination discovered by the edge.
type Target struct {
	Name        string   `json:"name" yaml:"name"`
	Address     string   `json:"address" yaml:"address"`
	Protocol    string   `json:"protocol" yaml:"protocol"`
	Labels      []string `json:"labels" yaml:"labels"`
	Description string   `json:"description" yaml:"description"`
}

type activeTunnel struct {
	target   Target
	openedAt time.Time
}

// Source returns discoverable targets (from static config, service discovery, etc.).
type Source interface {
	Targets(ctx context.Context) ([]Target, error)
}

// StaticSource returns a fixed set of targets.
type StaticSource struct {
	TargetsList []Target
}

func (s StaticSource) Targets(ctx context.Context) ([]Target, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	out := make([]Target, len(s.TargetsList))
	copy(out, s.TargetsList)
	return out, nil
}

// Dialer performs the actual connectivity check / tunnel bring-up.
type Dialer interface {
	Connect(ctx context.Context, target Target) error
}

// DialerFunc adapts a function into a Dialer implementation.
type DialerFunc func(ctx context.Context, target Target) error

func (f DialerFunc) Connect(ctx context.Context, target Target) error {
	return f(ctx, target)
}

// Manager coordinates known targets and active tunnels.
type Manager struct {
	mu              sync.RWMutex
	targets         map[string]Target
	active          map[string]activeTunnel
	source          Source
	dialer          Dialer
	refreshInterval time.Duration
	lastRefresh     time.Time
}

// Option configures a Manager.
type Option func(*Manager)

// WithSource overrides the discovery source.
func WithSource(source Source) Option {
	return func(m *Manager) {
		m.source = source
	}
}

// WithDialer overrides the tunnel dialer.
func WithDialer(d Dialer) Option {
	return func(m *Manager) {
		m.dialer = d
	}
}

// WithRefreshInterval overrides how frequently targets are re-discovered.
func WithRefreshInterval(interval time.Duration) Option {
	return func(m *Manager) {
		if interval > 0 {
			m.refreshInterval = interval
		}
	}
}

// NewManager bootstraps the manager with the provided targets and options.
func NewManager(seed []Target, opts ...Option) *Manager {
	m := &Manager{
		targets:         make(map[string]Target),
		active:          make(map[string]activeTunnel),
		source:          StaticSource{TargetsList: seed},
		dialer:          SimulatedDialer{},
		refreshInterval: 30 * time.Second,
	}
	for _, opt := range opts {
		opt(m)
	}
	// Prime cache immediately.
	for _, t := range seed {
		m.targets[t.Name] = t
	}
	return m
}

// NewDefaultManager seeds demo targets and uses a simulated dialer for local dev.
func NewDefaultManager() *Manager {
	return NewManager([]Target{
		{Name: "demo-service", Address: "tcp://127.0.0.1:9000", Protocol: "grpc", Description: "Sample echo service"},
		{Name: "logs-gateway", Address: "tcp://127.0.0.1:9001", Protocol: "https", Description: "Structured log shipper"},
		{Name: "metrics-api", Address: "tcp://127.0.0.1:9002", Protocol: "grpc", Description: "Prometheus-compatible adapter"},
	})
}

// RegisterTarget adds or updates a target immediately.
func (m *Manager) RegisterTarget(t Target) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.targets == nil {
		m.targets = make(map[string]Target)
	}
	m.targets[t.Name] = t
}

// Open establishes (simulated) tunnel state for the requested target.
func (m *Manager) Open(ctx context.Context, target string) error {
	if err := m.ensureTargets(ctx); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	info, ok := m.targets[target]
	if !ok {
		return fmt.Errorf("target %s is not registered", target)
	}

	if err := m.dialer.Connect(ctx, info); err != nil {
		return err
	}

	m.active[target] = activeTunnel{target: info, openedAt: time.Now()}
	return nil
}

// Close tears down an active tunnel if one exists.
func (m *Manager) Close(ctx context.Context, target string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.active[target]; !ok {
		return fmt.Errorf("no active tunnel for %s", target)
	}
	delete(m.active, target)
	return nil
}

// ListTargets returns discoverable targets sorted alphabetically.
func (m *Manager) ListTargets(ctx context.Context) ([]string, error) {
	if err := m.ensureTargets(ctx); err != nil {
		return nil, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.targets))
	for name := range m.targets {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func (m *Manager) ensureTargets(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.source == nil {
		return errors.New("tunnel manager source not configured")
	}

	if time.Since(m.lastRefresh) < m.refreshInterval {
		return nil
	}

	targets, err := m.source.Targets(ctx)
	if err != nil {
		return err
	}

	m.targets = make(map[string]Target, len(targets))
	for _, t := range targets {
		m.targets[t.Name] = t
	}
	m.lastRefresh = time.Now()
	return nil
}

// ResolveNetworkAddr extracts the network and host:port for a target address.
func ResolveNetworkAddr(t Target) (network, hostPort string, err error) {
	if t.Address == "" {
		return "", "", fmt.Errorf("target %s is missing address", t.Name)
	}

	addr := t.Address
	if !strings.Contains(addr, "://") {
		addr = "tcp://" + addr
	}
	u, err := url.Parse(addr)
	if err != nil {
		return "", "", err
	}
	host := u.Host
	if !strings.Contains(host, ":") && u.Port() == "" {
		return "", "", fmt.Errorf("target %s missing port in address %s", t.Name, t.Address)
	}
	network = u.Scheme
	if network == "" {
		network = "tcp"
	}
	if network == "grpcs" || network == "https" {
		network = "tcp"
	}
	if network == "grpc" || network == "http" {
		network = "tcp"
	}
	return network, host, nil
}
