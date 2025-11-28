package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	tunnelv1 "github.com/forest6511/cloudtap/proto/tunnel/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	grpcAddr := flag.String("server", envOrDefault("CLOUDTAP_GRPC_ADDR", "localhost:9090"), "cloudtapd gRPC address")
	httpURL := flag.String("http-url", defaultHTTPURL(), "cloudtapd HTTP base URL (http[s]://host:port)")
	token := flag.String("token", envOrDefault("CLOUDTAP_TOKEN", ""), "Bearer token for authentication")
	tlsCert := flag.String("tls-cert", envOrDefault("CLOUDTAP_TLS_CERT", ""), "Client TLS certificate path")
	tlsKey := flag.String("tls-key", envOrDefault("CLOUDTAP_TLS_KEY", ""), "Client TLS private key path")
	caFile := flag.String("ca-cert", envOrDefault("CLOUDTAP_CA_CERT", ""), "Custom CA bundle for TLS validation")
	skipVerify := flag.Bool("insecure-skip-verify", false, "Skip TLS verification (dev only)")
	flag.Parse()

	if flag.NArg() == 0 {
		usage()
		return
	}

	tlsCfg, err := buildTLSConfig(*tlsCert, *tlsKey, *caFile, *skipVerify)
	if err != nil {
		log.Fatalf("failed to load TLS config: %v", err)
	}

	runtime := &clientRuntime{
		grpcAddr:   *grpcAddr,
		httpBase:   *httpURL,
		token:      *token,
		httpClient: newHTTPClient(tlsCfg),
		grpcOpts:   buildGRPCDialOptions(tlsCfg, *token),
	}

	cmd := flag.Arg(0)
	switch cmd {
	case "proxy":
		target := flag.Arg(1)
		if target == "" {
			log.Fatal("usage: cloudtapctl proxy <service>")
		}
		if err := requestTunnel(ctx, runtime, target); err != nil {
			log.Fatalf("proxy failed: %v", err)
		}
	case "close":
		target := flag.Arg(1)
		if target == "" {
			log.Fatal("usage: cloudtapctl close <service>")
		}
		if err := closeTunnel(ctx, runtime, target); err != nil {
			log.Fatalf("close failed: %v", err)
		}
	case "list-targets":
		targets, err := listTargets(ctx, runtime)
		if err != nil {
			log.Fatalf("list-targets failed: %v", err)
		}
		for _, t := range targets {
			fmt.Println(t)
		}
	default:
		usage()
	}
}

func usage() {
	fmt.Println("Usage:")
	fmt.Println("  cloudtapctl proxy <service>")
	fmt.Println("  cloudtapctl close <service>")
	fmt.Println("  cloudtapctl list-targets")
	flag.PrintDefaults()
}

func requestTunnel(ctx context.Context, rt *clientRuntime, target string) error {
	if err := requestTunnelGRPC(ctx, rt, target); err == nil {
		return nil
	} else {
		log.Printf("grpc proxy request failed: %v; falling back to HTTP", err)
	}
	if err := requestTunnelHTTP(ctx, rt, target); err != nil {
		return fmt.Errorf("http fallback failed: %w", err)
	}
	return nil
}

func requestTunnelGRPC(ctx context.Context, rt *clientRuntime, target string) error {
	conn, err := rt.dialGRPC(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := tunnelv1.NewTunnelServiceClient(conn)
	resp, err := client.Open(rt.grpcContext(ctx), &tunnelv1.OpenRequest{Target: target})
	if err != nil {
		return err
	}
	fmt.Println(resp.GetMessage())
	return nil
}

func requestTunnelHTTP(ctx context.Context, rt *clientRuntime, target string) error {
	resp, err := rt.httpRequest(ctx, "/proxy", url.Values{"target": []string{target}})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	fmt.Println(strings.TrimSpace(string(body)))
	return nil
}

func closeTunnel(ctx context.Context, rt *clientRuntime, target string) error {
	if err := closeTunnelGRPC(ctx, rt, target); err == nil {
		return nil
	} else {
		log.Printf("grpc close request failed: %v; falling back to HTTP", err)
	}
	if err := closeTunnelHTTP(ctx, rt, target); err != nil {
		return fmt.Errorf("http fallback failed: %w", err)
	}
	return nil
}

func closeTunnelGRPC(ctx context.Context, rt *clientRuntime, target string) error {
	conn, err := rt.dialGRPC(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := tunnelv1.NewTunnelServiceClient(conn)
	resp, err := client.Close(rt.grpcContext(ctx), &tunnelv1.CloseRequest{Target: target})
	if err != nil {
		return err
	}
	fmt.Println(resp.GetMessage())
	return nil
}

func closeTunnelHTTP(ctx context.Context, rt *clientRuntime, target string) error {
	resp, err := rt.httpRequest(ctx, "/close", url.Values{"target": []string{target}})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	fmt.Println(strings.TrimSpace(string(body)))
	return nil
}

func listTargets(ctx context.Context, rt *clientRuntime) ([]string, error) {
	if targets, err := listTargetsGRPC(ctx, rt); err == nil {
		return targets, nil
	} else {
		log.Printf("grpc list-targets failed: %v; falling back to HTTP", err)
	}
	return listTargetsHTTP(ctx, rt)
}

func listTargetsGRPC(ctx context.Context, rt *clientRuntime) ([]string, error) {
	conn, err := rt.dialGRPC(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	client := tunnelv1.NewTunnelServiceClient(conn)
	resp, err := client.ListTargets(rt.grpcContext(ctx), &tunnelv1.ListTargetsRequest{})
	if err != nil {
		return nil, err
	}
	return resp.GetTargets(), nil
}

func listTargetsHTTP(ctx context.Context, rt *clientRuntime) ([]string, error) {
	resp, err := rt.httpRequest(ctx, "/targets", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload struct {
		Targets []string `json:"targets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload.Targets, nil
}

// clientRuntime aggregates connection material for gRPC and HTTP.
type clientRuntime struct {
	grpcAddr   string
	httpBase   string
	httpClient *http.Client
	grpcOpts   []grpc.DialOption
	token      string
}

func (rt *clientRuntime) dialGRPC(ctx context.Context) (*grpc.ClientConn, error) {
	opts := rt.grpcOpts
	if len(opts) == 0 {
		opts = []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	}
	return grpc.DialContext(ctx, rt.grpcAddr, opts...)
}

func (rt *clientRuntime) grpcContext(ctx context.Context) context.Context {
	if rt.token == "" {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+rt.token)
}

func (rt *clientRuntime) httpRequest(ctx context.Context, path string, query url.Values) (*http.Response, error) {
	base := sanitizeHTTPBase(rt.httpBase)
	endpoint := base + path
	if query != nil {
		q := query.Encode()
		if q != "" {
			endpoint += "?" + q
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if rt.token != "" {
		req.Header.Set("Authorization", "Bearer "+rt.token)
	}
	client := rt.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	return client.Do(req)
}

func envOrDefault(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func defaultHTTPURL() string {
	if val := os.Getenv("CLOUDTAP_HTTP_URL"); val != "" {
		return val
	}
	if addr := os.Getenv("CLOUDTAP_HTTP_ADDR"); addr != "" {
		return sanitizeHTTPBase(addr)
	}
	return "http://localhost:8080"
}

func sanitizeHTTPBase(addr string) string {
	trimmed := strings.TrimSpace(addr)
	if trimmed == "" {
		return "http://localhost:8080"
	}
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		return strings.TrimRight(trimmed, "/")
	}
	if strings.HasPrefix(trimmed, ":") {
		return "http://localhost" + trimmed
	}
	return "http://" + strings.TrimRight(trimmed, "/")
}

func buildTLSConfig(certFile, keyFile, caFile string, skipVerify bool) (*tls.Config, error) {
	if certFile == "" && keyFile == "" && caFile == "" && !skipVerify {
		return nil, nil
	}
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: skipVerify}

	if certFile != "" || keyFile != "" {
		if certFile == "" || keyFile == "" {
			return nil, errors.New("both --tls-cert and --tls-key are required")
		}
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, err
		}
		cfg.Certificates = []tls.Certificate{cert}
	}

	if caFile != "" {
		data, err := os.ReadFile(caFile)
		if err != nil {
			return nil, err
		}
		pool := x509.NewCertPool()
		if ok := pool.AppendCertsFromPEM(data); !ok {
			return nil, errors.New("failed to parse CA cert")
		}
		cfg.RootCAs = pool
	}

	return cfg, nil
}

func newHTTPClient(tlsCfg *tls.Config) *http.Client {
	if tlsCfg == nil {
		return http.DefaultClient
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsCfg
	return &http.Client{Transport: transport}
}

func buildGRPCDialOptions(tlsCfg *tls.Config, token string) []grpc.DialOption {
	var creds credentials.TransportCredentials
	if tlsCfg == nil {
		creds = insecure.NewCredentials()
	} else {
		creds = credentials.NewTLS(tlsCfg)
	}
	opts := []grpc.DialOption{grpc.WithTransportCredentials(creds)}
	if token != "" {
		opts = append(opts, grpc.WithPerRPCCredentials(tokenCredentials{token: token}))
	}
	return opts
}

type tokenCredentials struct {
	token string
}

func (t tokenCredentials) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{"authorization": "Bearer " + t.token}, nil
}

func (t tokenCredentials) RequireTransportSecurity() bool {
	return false
}
