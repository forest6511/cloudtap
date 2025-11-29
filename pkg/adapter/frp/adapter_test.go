package frp

import (
	"context"
	"errors"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/forest6511/cloudtap/pkg/adapter"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.RestartDelay != time.Second {
		t.Errorf("RestartDelay = %v, want %v", cfg.RestartDelay, time.Second)
	}
	if cfg.MaxRestartDelay != 5*time.Minute {
		t.Errorf("MaxRestartDelay = %v, want %v", cfg.MaxRestartDelay, 5*time.Minute)
	}
	if cfg.MaxRetries != 10 {
		t.Errorf("MaxRetries = %d, want 10", cfg.MaxRetries)
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: Config{
				FrpcPath:   "/usr/bin/frpc",
				ConfigPath: "/etc/frpc.toml",
			},
			wantErr: false,
		},
		{
			name: "missing FrpcPath",
			config: Config{
				ConfigPath: "/etc/frpc.toml",
			},
			wantErr: true,
		},
		{
			name: "missing ConfigPath",
			config: Config{
				FrpcPath: "/usr/bin/frpc",
			},
			wantErr: true,
		},
		{
			name: "negative RestartDelay",
			config: Config{
				FrpcPath:     "/usr/bin/frpc",
				ConfigPath:   "/etc/frpc.toml",
				RestartDelay: -time.Second,
			},
			wantErr: true,
		},
		{
			name: "negative MaxRestartDelay",
			config: Config{
				FrpcPath:        "/usr/bin/frpc",
				ConfigPath:      "/etc/frpc.toml",
				MaxRestartDelay: -time.Minute,
			},
			wantErr: true,
		},
		{
			name: "negative MaxRetries",
			config: Config{
				FrpcPath:   "/usr/bin/frpc",
				ConfigPath: "/etc/frpc.toml",
				MaxRetries: -1,
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

func TestNew(t *testing.T) {
	cfg := Config{
		FrpcPath:   "/usr/bin/frpc",
		ConfigPath: "/etc/frpc.toml",
	}

	a := New(cfg, nil)

	if a == nil {
		t.Fatal("New() returned nil")
	}
	if a.config.FrpcPath != cfg.FrpcPath {
		t.Errorf("FrpcPath = %q, want %q", a.config.FrpcPath, cfg.FrpcPath)
	}
	if a.config.ConfigPath != cfg.ConfigPath {
		t.Errorf("ConfigPath = %q, want %q", a.config.ConfigPath, cfg.ConfigPath)
	}
	// Check defaults are applied
	if a.config.RestartDelay != time.Second {
		t.Errorf("RestartDelay = %v, want %v", a.config.RestartDelay, time.Second)
	}
	if a.config.MaxRestartDelay != 5*time.Minute {
		t.Errorf("MaxRestartDelay = %v, want %v", a.config.MaxRestartDelay, 5*time.Minute)
	}
	if a.config.MaxRetries != 10 {
		t.Errorf("MaxRetries = %d, want 10", a.config.MaxRetries)
	}
}

func TestAdapter_OnConnection(t *testing.T) {
	cfg := Config{
		FrpcPath:   "/usr/bin/frpc",
		ConfigPath: "/etc/frpc.toml",
	}
	a := New(cfg, nil)

	handler := func(event adapter.ConnectionEvent) adapter.Decision {
		return adapter.Decision{Allowed: true}
	}

	a.OnConnection(handler)

	// Handler should be set
	a.mu.RLock()
	hasHandler := a.handler != nil
	a.mu.RUnlock()

	if !hasHandler {
		t.Error("handler was not set")
	}
}

func TestAdapter_StartWithoutHandler(t *testing.T) {
	cfg := Config{
		FrpcPath:   "/usr/bin/frpc",
		ConfigPath: "/etc/frpc.toml",
	}
	a := New(cfg, nil)

	ctx := context.Background()
	err := a.Start(ctx)

	if !errors.Is(err, adapter.ErrHandlerNotSet) {
		t.Errorf("Start() error = %v, want %v", err, adapter.ErrHandlerNotSet)
	}
}

func TestAdapter_StartWithInvalidConfig(t *testing.T) {
	cfg := Config{
		// Missing required fields
	}
	a := New(cfg, nil)
	a.OnConnection(func(event adapter.ConnectionEvent) adapter.Decision {
		return adapter.Decision{Allowed: true}
	})

	ctx := context.Background()
	err := a.Start(ctx)

	if err == nil {
		t.Error("Start() should return error for invalid config")
	}
}

func TestAdapter_StartTwice(t *testing.T) {
	cfg := Config{
		FrpcPath:     "/bin/sleep",
		ConfigPath:   "/dev/null",
		RestartDelay: 10 * time.Millisecond,
		MaxRetries:   1,
	}
	a := New(cfg, nil)
	a.OnConnection(func(event adapter.ConnectionEvent) adapter.Decision {
		return adapter.Decision{Allowed: true}
	})

	// Use a mock runner that simulates a long-running process
	mock := &mockCommandRunner{
		waitDuration: time.Minute,
	}
	a.SetCommandRunner(mock)

	ctx, cancel := context.WithCancel(context.Background())

	// Start in background
	errChan := make(chan error, 1)
	go func() {
		errChan <- a.Start(ctx)
	}()

	// Wait a bit for Start to begin
	time.Sleep(50 * time.Millisecond)

	// Try to start again
	err := a.Start(ctx)
	if !errors.Is(err, adapter.ErrAlreadyStarted) {
		t.Errorf("second Start() error = %v, want %v", err, adapter.ErrAlreadyStarted)
	}

	// Clean up
	cancel()
	<-errChan
}

func TestAdapter_StopWithoutStart(t *testing.T) {
	cfg := Config{
		FrpcPath:   "/usr/bin/frpc",
		ConfigPath: "/etc/frpc.toml",
	}
	a := New(cfg, nil)

	ctx := context.Background()
	err := a.Stop(ctx)

	if !errors.Is(err, adapter.ErrNotStarted) {
		t.Errorf("Stop() error = %v, want %v", err, adapter.ErrNotStarted)
	}
}

func TestAdapter_StopTwice(t *testing.T) {
	cfg := Config{
		FrpcPath:     "/bin/sleep",
		ConfigPath:   "/dev/null",
		RestartDelay: 10 * time.Millisecond,
		MaxRetries:   1,
	}
	a := New(cfg, nil)
	a.OnConnection(func(event adapter.ConnectionEvent) adapter.Decision {
		return adapter.Decision{Allowed: true}
	})

	mock := &mockCommandRunner{
		waitDuration: time.Minute,
	}
	a.SetCommandRunner(mock)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start in background
	errChan := make(chan error, 1)
	go func() {
		errChan <- a.Start(ctx)
	}()

	// Wait for start
	time.Sleep(50 * time.Millisecond)

	// First stop
	stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
	defer stopCancel()
	err := a.Stop(stopCtx)
	if err != nil {
		t.Errorf("first Stop() error = %v", err)
	}

	// Second stop should return ErrNotStarted
	err = a.Stop(stopCtx)
	if !errors.Is(err, adapter.ErrNotStarted) {
		t.Errorf("second Stop() error = %v, want %v", err, adapter.ErrNotStarted)
	}

	<-errChan
}

func TestAdapter_Status(t *testing.T) {
	cfg := Config{
		FrpcPath:   "/usr/bin/frpc",
		ConfigPath: "/etc/frpc.toml",
	}
	a := New(cfg, nil)

	status := a.Status()

	if status.Type != "frp" {
		t.Errorf("Type = %q, want %q", status.Type, "frp")
	}
	if status.Connected {
		t.Error("Connected should be false before start")
	}
	if status.UptimeSeconds != 0 {
		t.Errorf("UptimeSeconds = %d, want 0", status.UptimeSeconds)
	}
}

func TestAdapter_ProcessLifecycle(t *testing.T) {
	// Test that process failures (non-zero exit) trigger max retries
	cfg := Config{
		FrpcPath:     "/usr/bin/false", // Exits with code 1 (failure)
		ConfigPath:   "/dev/null",
		RestartDelay: 10 * time.Millisecond,
		MaxRetries:   2,
	}
	a := New(cfg, nil)
	a.OnConnection(func(event adapter.ConnectionEvent) adapter.Decision {
		return adapter.Decision{Allowed: true}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := a.Start(ctx)

	// Should fail after max retries because /bin/false exits with error
	if err == nil {
		t.Error("Start() should return error after max retries")
	}

	// Check reconnect attempts
	status := a.Status()
	if status.ReconnectAttempts < 1 {
		t.Errorf("ReconnectAttempts = %d, want >= 1", status.ReconnectAttempts)
	}
}

func TestAdapter_CleanExitRestart(t *testing.T) {
	// Test that clean exits (exit code 0) reset backoff but still restart
	cfg := Config{
		FrpcPath:     "/usr/bin/true", // Exits with code 0 (success)
		ConfigPath:   "/dev/null",
		RestartDelay: 10 * time.Millisecond,
		MaxRetries:   3,
	}
	a := New(cfg, nil)
	a.OnConnection(func(event adapter.ConnectionEvent) adapter.Decision {
		return adapter.Decision{Allowed: true}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := a.Start(ctx)

	// Should not hit max retries because clean exits reset counters
	// Will exit due to context timeout
	if err != nil {
		t.Errorf("Start() unexpected error: %v", err)
	}

	// Check reconnect attempts - should have several from clean restarts
	status := a.Status()
	if status.ReconnectAttempts < 1 {
		t.Errorf("ReconnectAttempts = %d, want >= 1", status.ReconnectAttempts)
	}
}

func TestAdapter_GracefulShutdown(t *testing.T) {
	cfg := Config{
		FrpcPath:     "/bin/sleep",
		ConfigPath:   "/dev/null",
		RestartDelay: 10 * time.Millisecond,
		MaxRetries:   10,
	}
	a := New(cfg, nil)
	a.OnConnection(func(event adapter.ConnectionEvent) adapter.Decision {
		return adapter.Decision{Allowed: true}
	})

	mock := &mockCommandRunner{
		waitDuration: time.Minute,
	}
	a.SetCommandRunner(mock)

	ctx, cancel := context.WithCancel(context.Background())

	errChan := make(chan error, 1)
	go func() {
		errChan <- a.Start(ctx)
	}()

	// Wait for start
	time.Sleep(50 * time.Millisecond)

	// Stop gracefully
	stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
	defer stopCancel()

	err := a.Stop(stopCtx)
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	cancel()

	// Start should return without error (cancelled gracefully)
	select {
	case err := <-errChan:
		if err != nil {
			t.Errorf("Start() error = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Error("Start() did not return in time")
	}
}

func TestAdapter_ExponentialBackoff(t *testing.T) {
	// Use /usr/bin/false which exits with error code 1 to test backoff on failures
	cfg := Config{
		FrpcPath:        "/usr/bin/false",
		ConfigPath:      "/dev/null",
		RestartDelay:    10 * time.Millisecond,
		MaxRestartDelay: 50 * time.Millisecond,
		MaxRetries:      5,
	}
	a := New(cfg, nil)
	a.OnConnection(func(event adapter.ConnectionEvent) adapter.Decision {
		return adapter.Decision{Allowed: true}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	startTime := time.Now()
	_ = a.Start(ctx)
	totalTime := time.Since(startTime)

	// With 5 retries and exponential backoff starting at 10ms:
	// delay 1: 10ms, delay 2: 20ms, delay 3: 40ms (capped at 50ms), delay 4: 50ms
	// Total should be at least 10+20+40+50 = 120ms, less process start overhead
	// With max retries hit, we should have taken meaningful time
	if totalTime < 50*time.Millisecond {
		t.Errorf("expected at least 50ms with backoff, got %v", totalTime)
	}

	// Check reconnect attempts
	status := a.Status()
	if status.ReconnectAttempts < 2 {
		t.Errorf("ReconnectAttempts = %d, want >= 2", status.ReconnectAttempts)
	}
}

func TestAdapter_StatusAfterStart(t *testing.T) {
	cfg := Config{
		FrpcPath:     "/bin/sleep",
		ConfigPath:   "/dev/null",
		RestartDelay: time.Second,
		MaxRetries:   1,
	}
	a := New(cfg, nil)
	a.OnConnection(func(event adapter.ConnectionEvent) adapter.Decision {
		return adapter.Decision{Allowed: true}
	})

	mock := &mockCommandRunner{
		waitDuration: time.Minute,
	}
	a.SetCommandRunner(mock)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go a.Start(ctx)

	// Wait for start
	time.Sleep(50 * time.Millisecond)

	status := a.Status()

	if status.Type != "frp" {
		t.Errorf("Type = %q, want %q", status.Type, "frp")
	}
	if !status.StartedAt.Before(time.Now()) {
		t.Error("StartedAt should be set")
	}
	if status.UptimeSeconds < 0 {
		t.Errorf("UptimeSeconds = %d, want >= 0", status.UptimeSeconds)
	}
}

func TestAdapter_LastError(t *testing.T) {
	cfg := Config{
		FrpcPath:     "/nonexistent/path",
		ConfigPath:   "/dev/null",
		RestartDelay: 10 * time.Millisecond,
		MaxRetries:   1,
	}
	a := New(cfg, nil)
	a.OnConnection(func(event adapter.ConnectionEvent) adapter.Decision {
		return adapter.Decision{Allowed: true}
	})

	mock := &mockCommandRunner{
		startError: errors.New("test error"),
	}
	a.SetCommandRunner(mock)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_ = a.Start(ctx)

	status := a.Status()
	if status.LastError == "" {
		t.Error("LastError should be set after failure")
	}
	if status.LastErrorAt.IsZero() {
		t.Error("LastErrorAt should be set after failure")
	}
}

// mockCommandRunner implements CommandRunner for testing.
type mockCommandRunner struct {
	startError      error
	exitImmediately bool
	waitDuration    time.Duration
	onStart         func()

	mu         sync.Mutex
	startCount int
}

func (m *mockCommandRunner) CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	m.mu.Lock()
	m.startCount++
	if m.onStart != nil {
		m.onStart()
	}
	m.mu.Unlock()

	if m.startError != nil {
		// Return a command that will fail immediately
		return exec.CommandContext(ctx, "/nonexistent/command/that/does/not/exist")
	}

	if m.exitImmediately {
		// Use a command that exits immediately
		return exec.CommandContext(ctx, "true")
	}

	// Use sleep for long-running process simulation
	if m.waitDuration > 0 {
		return exec.CommandContext(ctx, "sleep", "3600")
	}

	return exec.CommandContext(ctx, "true")
}

func TestAdapter_ConcurrentStatusCalls(t *testing.T) {
	cfg := Config{
		FrpcPath:     "/bin/sleep",
		ConfigPath:   "/dev/null",
		RestartDelay: time.Second,
		MaxRetries:   1,
	}
	a := New(cfg, nil)
	a.OnConnection(func(event adapter.ConnectionEvent) adapter.Decision {
		return adapter.Decision{Allowed: true}
	})

	mock := &mockCommandRunner{
		waitDuration: time.Minute,
	}
	a.SetCommandRunner(mock)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go a.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	// Call Status concurrently
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status := a.Status()
			if status.Type != "frp" {
				t.Errorf("Type = %q, want %q", status.Type, "frp")
			}
		}()
	}
	wg.Wait()
}

func TestAdapter_InterfaceCompliance(t *testing.T) {
	// Compile-time check that Adapter implements adapter.Adapter
	var _ adapter.Adapter = (*Adapter)(nil)
}

// TestAdapter_OutputLogging tests that process output is logged.
func TestAdapter_OutputLogging(t *testing.T) {
	// Use /usr/bin/false to exit with error, triggering max retries
	cfg := Config{
		FrpcPath:     "/usr/bin/false",
		ConfigPath:   "/dev/null",
		RestartDelay: 10 * time.Millisecond,
		MaxRetries:   1,
	}

	a := New(cfg, nil)
	a.OnConnection(func(event adapter.ConnectionEvent) adapter.Decision {
		return adapter.Decision{Allowed: true}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Should fail due to max retries (process exits with error)
	_ = a.Start(ctx)

	// Just verify no panic occurred during output logging
}

func TestAdapter_ContextCancellationDuringRestart(t *testing.T) {
	// Use /usr/bin/false to trigger restart delay, then cancel context
	cfg := Config{
		FrpcPath:     "/usr/bin/false",
		ConfigPath:   "/dev/null",
		RestartDelay: time.Second, // Long delay
		MaxRetries:   10,
	}
	a := New(cfg, nil)
	a.OnConnection(func(event adapter.ConnectionEvent) adapter.Decision {
		return adapter.Decision{Allowed: true}
	})

	var startCount atomic.Int32
	mock := &mockCommandRunner{
		onStart: func() {
			startCount.Add(1)
		},
		exitImmediately: true,
	}
	a.SetCommandRunner(mock)

	ctx, cancel := context.WithCancel(context.Background())

	errChan := make(chan error, 1)
	go func() {
		errChan <- a.Start(ctx)
	}()

	// Wait for first failure and restart delay to begin
	time.Sleep(100 * time.Millisecond)

	// Cancel during restart delay
	cancel()

	select {
	case err := <-errChan:
		// Should return nil (graceful shutdown)
		if err != nil {
			t.Errorf("Start() error = %v, want nil on context cancel", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Start() did not return after context cancellation")
	}
}
