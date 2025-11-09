package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/forest6511/cloudtap/pkg/tunnel"
	tunnelv1 "github.com/forest6511/cloudtap/proto/tunnel/v1"
	"google.golang.org/grpc"
)

type config struct {
	HTTPAddr string
	GRPCAddr string
	DevMode  bool
}

type tunnelController interface {
	Open(ctx context.Context, target string) error
	Close(ctx context.Context, target string) error
	ListTargets(ctx context.Context) ([]string, error)
}

func main() {
	cfg := parseConfig()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Printf("cloudtapd booting (http=%s grpc=%s dev=%t)", cfg.HTTPAddr, cfg.GRPCAddr, cfg.DevMode)

	manager := tunnel.NewDefaultManager()
	serverErrs := make(chan error, 2)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		if err := runHTTPServer(ctx, cfg.HTTPAddr, manager); err != nil {
			serverErrs <- fmt.Errorf("http server: %w", err)
		}
	}()

	go func() {
		defer wg.Done()
		if err := runGRPCServer(ctx, cfg.GRPCAddr, manager); err != nil {
			serverErrs <- fmt.Errorf("grpc server: %w", err)
		}
	}()

	go func() {
		wg.Wait()
		close(serverErrs)
	}()

	var fatalErr error

	select {
	case <-ctx.Done():
		log.Println("shutdown signal received; draining servers")
	case err := <-serverErrs:
		fatalErr = err
		if err != nil {
			log.Printf("server exited early: %v", err)
		}
	}

	stop()

	for err := range serverErrs {
		if err != nil && fatalErr == nil {
			fatalErr = err
		}
	}

	if fatalErr != nil {
		log.Fatalf("cloudtapd stopped with error: %v", fatalErr)
	}

	log.Println("cloudtapd stopped cleanly")
}

func parseConfig() config {
	defaultHTTP := envOrDefault("CLOUDTAP_HTTP_ADDR", ":8080")
	defaultGRPC := envOrDefault("CLOUDTAP_GRPC_ADDR", ":9090")
	defaultDev := envOrBool("CLOUDTAP_DEV_MODE", false)

	httpAddr := flag.String("http-addr", defaultHTTP, "HTTP listen address, e.g. :8080")
	grpcAddr := flag.String("grpc-addr", defaultGRPC, "gRPC listen address, e.g. :9090")
	devMode := flag.Bool("dev", defaultDev, "run in developer mode with verbose logging")

	flag.Parse()

	return config{
		HTTPAddr: *httpAddr,
		GRPCAddr: *grpcAddr,
		DevMode:  *devMode,
	}
}

func envOrDefault(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func envOrBool(key string, fallback bool) bool {
	if val := os.Getenv(key); val != "" {
		parsed, err := strconv.ParseBool(val)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func runHTTPServer(ctx context.Context, addr string, manager tunnelController) error {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("/proxy", func(w http.ResponseWriter, r *http.Request) {
		target := r.URL.Query().Get("target")
		if target == "" {
			http.Error(w, "missing target parameter", http.StatusBadRequest)
			return
		}

		proxyCtx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		if err := manager.Open(proxyCtx, target); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		fmt.Fprintf(w, "proxy to %s initiated", target)
	})

	mux.HandleFunc("/close", func(w http.ResponseWriter, r *http.Request) {
		target := r.URL.Query().Get("target")
		if target == "" {
			http.Error(w, "missing target parameter", http.StatusBadRequest)
			return
		}
		closeCtx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		if err := manager.Close(closeCtx, target); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		fmt.Fprintf(w, "closed tunnel to %s", target)
	})

	mux.HandleFunc("/targets", func(w http.ResponseWriter, r *http.Request) {
		listCtx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		targets, err := manager.ListTargets(listCtx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(struct {
			Targets []string `json:"targets"`
		}{Targets: targets}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil {
			errCh <- err
		} else {
			errCh <- nil
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}

		if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case err := <-errCh:
		if err == nil || errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func runGRPCServer(ctx context.Context, addr string, manager tunnelController) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	server := grpc.NewServer()
	tunnelv1.RegisterTunnelServiceServer(server, tunnelServer{manager: manager})

	go func() {
		<-ctx.Done()
		server.GracefulStop()
	}()

	if err := server.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		return err
	}

	return nil
}

type tunnelServer struct {
	tunnelv1.UnimplementedTunnelServiceServer
	manager tunnelController
}

func (s tunnelServer) Open(ctx context.Context, req *tunnelv1.OpenRequest) (*tunnelv1.OpenResponse, error) {
	if req == nil || req.GetTarget() == "" {
		return nil, fmt.Errorf("target is required")
	}
	if err := s.manager.Open(ctx, req.GetTarget()); err != nil {
		return nil, err
	}
	return &tunnelv1.OpenResponse{Message: fmt.Sprintf("proxy to %s initiated", req.GetTarget())}, nil
}

func (s tunnelServer) Close(ctx context.Context, req *tunnelv1.CloseRequest) (*tunnelv1.CloseResponse, error) {
	if req == nil || req.GetTarget() == "" {
		return nil, fmt.Errorf("target is required")
	}
	if err := s.manager.Close(ctx, req.GetTarget()); err != nil {
		return nil, err
	}
	return &tunnelv1.CloseResponse{Message: fmt.Sprintf("closed tunnel to %s", req.GetTarget())}, nil
}

func (s tunnelServer) ListTargets(ctx context.Context, _ *tunnelv1.ListTargetsRequest) (*tunnelv1.ListTargetsResponse, error) {
	targets, err := s.manager.ListTargets(ctx)
	if err != nil {
		return nil, err
	}
	return &tunnelv1.ListTargetsResponse{Targets: targets}, nil
}
