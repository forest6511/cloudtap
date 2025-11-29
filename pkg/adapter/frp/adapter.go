// Package frp provides an adapter that wraps the frp client for cloudtap.
//
// The frp adapter manages the lifecycle of an frpc process and intercepts
// connection events through frp's plugin system. It implements the adapter.Adapter
// interface and provides automatic process restart with exponential backoff.
//
// # Process Management
//
// The adapter spawns frpc as a child process and monitors its health.
// If the process exits unexpectedly, it will be restarted automatically
// with exponential backoff to avoid overwhelming the system.
//
// # Configuration
//
// The adapter requires:
//   - Path to the frpc binary
//   - Path to the frpc configuration file (TOML format)
//   - Optional restart parameters (delay, max retries)
//
// # Example Usage
//
//	cfg := frp.Config{
//	    FrpcPath:     "/usr/local/bin/frpc",
//	    ConfigPath:   "/etc/cloudtap/frpc.toml",
//	    RestartDelay: time.Second,
//	    MaxRetries:   10,
//	}
//	adapter := frp.New(cfg, logger)
//	adapter.OnConnection(handler)
//	if err := adapter.Start(ctx); err != nil {
//	    log.Fatal(err)
//	}
package frp

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/forest6511/cloudtap/pkg/adapter"
)

// Config holds the configuration for the frp adapter.
type Config struct {
	// FrpcPath is the path to the frpc binary.
	// Required.
	FrpcPath string `json:"frpc_path"`

	// ConfigPath is the path to the frpc configuration file (TOML format).
	// Required.
	ConfigPath string `json:"config_path"`

	// RestartDelay is the initial delay before restarting a crashed process.
	// Subsequent restarts use exponential backoff.
	// Default: 1 second.
	RestartDelay time.Duration `json:"restart_delay"`

	// MaxRestartDelay is the maximum delay between restart attempts.
	// Default: 5 minutes.
	MaxRestartDelay time.Duration `json:"max_restart_delay"`

	// MaxRetries is the maximum number of consecutive restart attempts.
	// After this many failures, Start returns an error.
	// Set to 0 for unlimited retries.
	// Default: 10.
	MaxRetries int `json:"max_retries"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		RestartDelay:    time.Second,
		MaxRestartDelay: 5 * time.Minute,
		MaxRetries:      10,
	}
}

// Validate checks if the configuration is valid.
func (c Config) Validate() error {
	if c.FrpcPath == "" {
		return fmt.Errorf("frp: FrpcPath is required")
	}
	if c.ConfigPath == "" {
		return fmt.Errorf("frp: ConfigPath is required")
	}
	if c.RestartDelay < 0 {
		return fmt.Errorf("frp: RestartDelay must be non-negative")
	}
	if c.MaxRestartDelay < 0 {
		return fmt.Errorf("frp: MaxRestartDelay must be non-negative")
	}
	if c.MaxRetries < 0 {
		return fmt.Errorf("frp: MaxRetries must be non-negative")
	}
	return nil
}

// CommandRunner abstracts command execution for testing.
type CommandRunner interface {
	// CommandContext creates an exec.Cmd with the given context.
	CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd
}

// defaultCommandRunner implements CommandRunner using os/exec.
type defaultCommandRunner struct{}

func (r *defaultCommandRunner) CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}

// Adapter wraps an frpc process and implements adapter.Adapter.
type Adapter struct {
	config  Config
	logger  *slog.Logger
	runner  CommandRunner
	handler adapter.ConnectionHandler

	// mu protects state transitions
	mu      sync.RWMutex
	started atomic.Bool
	stopped atomic.Bool

	// Process state
	cmd       *exec.Cmd
	startedAt time.Time

	// Reconnection tracking
	reconnectAttempts atomic.Int32
	lastError         atomic.Value // stores string
	lastErrorAt       atomic.Value // stores time.Time

	// Shutdown coordination
	cancel   context.CancelFunc
	doneChan chan struct{}
}

// New creates a new frp adapter with the given configuration.
// The logger is used for structured logging of lifecycle events.
// If logger is nil, a default logger writing to os.Stderr is used.
func New(cfg Config, logger *slog.Logger) *Adapter {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}

	// Apply defaults
	if cfg.RestartDelay == 0 {
		cfg.RestartDelay = time.Second
	}
	if cfg.MaxRestartDelay == 0 {
		cfg.MaxRestartDelay = 5 * time.Minute
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 10
	}

	return &Adapter{
		config:   cfg,
		logger:   logger.With("adapter", "frp"),
		runner:   &defaultCommandRunner{},
		doneChan: make(chan struct{}),
	}
}

// SetCommandRunner sets a custom command runner for testing.
// Must be called before Start.
func (a *Adapter) SetCommandRunner(runner CommandRunner) {
	a.runner = runner
}

// OnConnection registers a handler for connection events.
// Must be called before Start.
func (a *Adapter) OnConnection(handler adapter.ConnectionHandler) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.handler = handler
}

// Start begins the frp adapter and underlying frpc process.
// It blocks until the context is cancelled or a fatal error occurs.
func (a *Adapter) Start(ctx context.Context) error {
	// Validate configuration
	if err := a.config.Validate(); err != nil {
		return err
	}

	// Check handler is set
	a.mu.RLock()
	handler := a.handler
	a.mu.RUnlock()
	if handler == nil {
		return adapter.ErrHandlerNotSet
	}

	// Ensure we haven't already started
	if !a.started.CompareAndSwap(false, true) {
		return adapter.ErrAlreadyStarted
	}

	// Create cancellable context for shutdown
	ctx, a.cancel = context.WithCancel(ctx)

	a.mu.Lock()
	a.startedAt = time.Now()
	a.mu.Unlock()

	a.logger.Info("starting frp adapter",
		"frpc_path", a.config.FrpcPath,
		"config_path", a.config.ConfigPath,
	)

	// Run the process management loop
	err := a.runLoop(ctx)

	// Signal that we're done
	close(a.doneChan)

	return err
}

// Stop gracefully shuts down the adapter and frpc process.
func (a *Adapter) Stop(ctx context.Context) error {
	if !a.started.Load() {
		return adapter.ErrNotStarted
	}

	if !a.stopped.CompareAndSwap(false, true) {
		return adapter.ErrNotStarted
	}

	a.logger.Info("stopping frp adapter")

	// Cancel the run loop
	if a.cancel != nil {
		a.cancel()
	}

	// Wait for the run loop to finish or context to expire
	select {
	case <-a.doneChan:
		a.logger.Info("frp adapter stopped gracefully")
		return nil
	case <-ctx.Done():
		a.logger.Warn("frp adapter stop timed out", "error", ctx.Err())
		return ctx.Err()
	}
}

// Status returns the current status of the adapter.
func (a *Adapter) Status() adapter.Status {
	a.mu.RLock()
	startedAt := a.startedAt
	a.mu.RUnlock()

	var uptimeSeconds int64
	if !startedAt.IsZero() {
		uptimeSeconds = int64(time.Since(startedAt).Seconds())
	}

	var lastError string
	if v := a.lastError.Load(); v != nil {
		lastError = v.(string)
	}

	var lastErrorAt time.Time
	if v := a.lastErrorAt.Load(); v != nil {
		lastErrorAt = v.(time.Time)
	}

	return adapter.Status{
		Type:              "frp",
		Connected:         a.started.Load() && !a.stopped.Load() && a.isProcessRunning(),
		StartedAt:         startedAt,
		UptimeSeconds:     uptimeSeconds,
		ActiveConnections: 0, // TODO: Track active connections via webhook
		ReconnectAttempts: int(a.reconnectAttempts.Load()),
		BackendEndpoint:   "", // TODO: Parse from config
		LastError:         lastError,
		LastErrorAt:       lastErrorAt,
	}
}

// stableRunThreshold is the minimum duration a process must run
// before being considered stable (resets backoff counters).
const stableRunThreshold = 30 * time.Second

// runLoop manages the frpc process lifecycle with restart handling.
func (a *Adapter) runLoop(ctx context.Context) error {
	delay := a.config.RestartDelay
	consecutiveFailures := 0

	for {
		select {
		case <-ctx.Done():
			// Context cancelled, stop the process if running
			a.stopProcess()
			return nil
		default:
		}

		// Start the process
		processStartTime := time.Now()
		err := a.startProcess(ctx)
		if err != nil {
			a.logger.Error("failed to start frpc process", "error", err)
			a.setLastError(err)

			consecutiveFailures++
			if a.config.MaxRetries > 0 && consecutiveFailures >= a.config.MaxRetries {
				return fmt.Errorf("frp: max retries (%d) exceeded: %w", a.config.MaxRetries, err)
			}

			// Wait before retry with exponential backoff
			a.logger.Info("waiting before restart",
				"delay", delay,
				"attempt", consecutiveFailures,
				"max_retries", a.config.MaxRetries,
			)

			select {
			case <-ctx.Done():
				return nil
			case <-time.After(delay):
			}

			// Exponential backoff
			delay = delay * 2
			if delay > a.config.MaxRestartDelay {
				delay = a.config.MaxRestartDelay
			}

			a.reconnectAttempts.Add(1)
			continue
		}

		// Wait for process to exit
		err = a.waitProcess(ctx)
		runDuration := time.Since(processStartTime)

		// Check if we should stop
		if ctx.Err() != nil {
			return nil
		}

		// Determine if this was a stable run (ran long enough to reset counters)
		wasStableRun := runDuration >= stableRunThreshold

		if err != nil {
			a.logger.Error("frpc process exited with error",
				"error", err,
				"run_duration", runDuration,
			)
			a.setLastError(err)

			// Only count as failure if it wasn't a stable run
			if !wasStableRun {
				consecutiveFailures++
			} else {
				// Reset counters after stable run
				consecutiveFailures = 0
				delay = a.config.RestartDelay
			}
		} else {
			// Clean exit (no error) - reset backoff but still restart
			a.logger.Info("frpc process exited normally",
				"run_duration", runDuration,
			)
			consecutiveFailures = 0
			delay = a.config.RestartDelay
		}

		// Check max retries
		if a.config.MaxRetries > 0 && consecutiveFailures >= a.config.MaxRetries {
			return fmt.Errorf("frp: max retries (%d) exceeded after process exit", a.config.MaxRetries)
		}

		a.logger.Info("restarting frpc process",
			"delay", delay,
			"attempt", consecutiveFailures,
		)

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(delay):
		}

		// Apply exponential backoff only for failures
		if consecutiveFailures > 0 {
			delay = delay * 2
			if delay > a.config.MaxRestartDelay {
				delay = a.config.MaxRestartDelay
			}
		}

		a.reconnectAttempts.Add(1)
	}
}

// startProcess starts the frpc process.
func (a *Adapter) startProcess(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	cmd := a.runner.CommandContext(ctx, a.config.FrpcPath, "-c", a.config.ConfigPath)

	// Capture stdout/stderr for logging
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start frpc: %w", err)
	}

	a.cmd = cmd
	a.logger.Info("frpc process started", "pid", cmd.Process.Pid)

	// Start goroutines to log output
	go a.logOutput("stdout", stdout)
	go a.logOutput("stderr", stderr)

	return nil
}

// waitProcess waits for the frpc process to exit.
func (a *Adapter) waitProcess(ctx context.Context) error {
	a.mu.RLock()
	cmd := a.cmd
	a.mu.RUnlock()

	if cmd == nil || cmd.Process == nil {
		return nil
	}

	// Wait for process in a goroutine so we can respect context cancellation
	errChan := make(chan error, 1)
	go func() {
		errChan <- cmd.Wait()
	}()

	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		// Context cancelled, kill the process
		a.stopProcess()
		return ctx.Err()
	}
}

// stopProcess stops the frpc process gracefully.
func (a *Adapter) stopProcess() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.cmd == nil || a.cmd.Process == nil {
		return
	}

	a.logger.Info("stopping frpc process", "pid", a.cmd.Process.Pid)

	// Send SIGTERM first for graceful shutdown
	if err := a.cmd.Process.Signal(os.Interrupt); err != nil {
		a.logger.Warn("failed to send interrupt signal", "error", err)
		// Fall back to kill
		if err := a.cmd.Process.Kill(); err != nil {
			a.logger.Error("failed to kill process", "error", err)
		}
	}

	a.cmd = nil
}

// isProcessRunning checks if the frpc process is currently running.
func (a *Adapter) isProcessRunning() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.cmd == nil || a.cmd.Process == nil {
		return false
	}

	// Check if process is still running by sending signal 0.
	// syscall.Signal(0) probes the process without affecting it.
	err := a.cmd.Process.Signal(syscall.Signal(0))
	return err == nil
}

// setLastError records the last error.
func (a *Adapter) setLastError(err error) {
	a.lastError.Store(err.Error())
	a.lastErrorAt.Store(time.Now())
}

// logOutput logs output from the frpc process.
func (a *Adapter) logOutput(name string, r io.Reader) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			a.logger.Debug("frpc output",
				"stream", name,
				"output", string(buf[:n]),
			)
		}
		if err != nil {
			if err != io.EOF {
				a.logger.Debug("frpc output stream closed", "stream", name, "error", err)
			}
			return
		}
	}
}

// Compile-time interface check
var _ adapter.Adapter = (*Adapter)(nil)
