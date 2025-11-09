package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	tunnelv1 "github.com/forest6511/cloudtap/proto/tunnel/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

func TestRequestTunnelFallsBackToHTTP(t *testing.T) {
	grpcAddr, cleanup := startGRPCServer(t, fakeTunnelServer{openErr: status.Error(codes.Unavailable, "unavailable")})
	t.Cleanup(cleanup)

	called := false
	httpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/proxy" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		called = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("proxy to demo-service initiated"))
	}))
	defer httpSrv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	rt := runtimeForTests(grpcAddr, httpSrv.URL)
	if err := requestTunnel(ctx, rt, "demo-service"); err != nil {
		t.Fatalf("requestTunnel failed: %v", err)
	}
	if !called {
		t.Fatal("expected HTTP fallback to run")
	}
}

func TestListTargetsGRPC(t *testing.T) {
	grpcAddr, cleanup := startGRPCServer(t, fakeTunnelServer{targets: []string{"demo", "logs"}})
	t.Cleanup(cleanup)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	rt := runtimeForTests(grpcAddr, "")
	targets, err := listTargets(ctx, rt)
	if err != nil {
		t.Fatalf("listTargets failed: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %v", targets)
	}
}

func TestCloseTunnelFallsBackToHTTP(t *testing.T) {
	grpcAddr, cleanup := startGRPCServer(t, fakeTunnelServer{closeErr: status.Error(codes.Unavailable, "force http")})
	t.Cleanup(cleanup)

	called := false
	httpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/close" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		called = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("closed tunnel to demo"))
	}))
	defer httpSrv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	rt := runtimeForTests(grpcAddr, httpSrv.URL)
	if err := closeTunnel(ctx, rt, "demo"); err != nil {
		t.Fatalf("closeTunnel failed: %v", err)
	}
	if !called {
		t.Fatal("expected HTTP fallback for close")
	}
}

func startGRPCServer(t *testing.T, impl tunnelv1.TunnelServiceServer) (string, func()) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	server := grpc.NewServer()
	tunnelv1.RegisterTunnelServiceServer(server, impl)
	go server.Serve(lis)

	cleanup := func() {
		server.Stop()
		_ = lis.Close()
	}
	return lis.Addr().String(), cleanup
}

func runtimeForTests(grpcAddr, httpURL string) *clientRuntime {
	return &clientRuntime{
		grpcAddr:   grpcAddr,
		httpBase:   httpURL,
		httpClient: http.DefaultClient,
		grpcOpts:   []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())},
	}
}

type fakeTunnelServer struct {
	tunnelv1.UnimplementedTunnelServiceServer

	openErr  error
	closeErr error
	targets  []string
}

func (f fakeTunnelServer) Open(ctx context.Context, _ *tunnelv1.OpenRequest) (*tunnelv1.OpenResponse, error) {
	if f.openErr != nil {
		return nil, f.openErr
	}
	return &tunnelv1.OpenResponse{Message: "ok"}, nil
}

func (f fakeTunnelServer) Close(ctx context.Context, _ *tunnelv1.CloseRequest) (*tunnelv1.CloseResponse, error) {
	if f.closeErr != nil {
		return nil, f.closeErr
	}
	return &tunnelv1.CloseResponse{Message: "ok"}, nil
}

func (f fakeTunnelServer) ListTargets(ctx context.Context, _ *tunnelv1.ListTargetsRequest) (*tunnelv1.ListTargetsResponse, error) {
	return &tunnelv1.ListTargetsResponse{Targets: f.targets}, nil
}
func TestAuthorizationHeaderForHTTPFallback(t *testing.T) {
	seen := ""
	httpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer httpSrv.Close()

	rt := &clientRuntime{
		httpBase:   httpSrv.URL,
		token:      "test-token",
		httpClient: http.DefaultClient,
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := requestTunnelHTTP(ctx, rt, "demo"); err != nil {
		t.Fatalf("requestTunnelHTTP failed: %v", err)
	}
	if seen != "Bearer test-token" {
		t.Fatalf("expected Authorization header, got %q", seen)
	}
}
