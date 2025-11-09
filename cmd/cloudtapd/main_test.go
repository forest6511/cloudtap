package main

import (
	"context"
	"errors"
	"testing"

	tunnelv1 "github.com/forest6511/cloudtap/proto/tunnel/v1"
)

func TestEnvOrDefault(t *testing.T) {
	t.Setenv("CLOUDTAP_TEST_VALUE", "from-env")
	if got := envOrDefault("CLOUDTAP_TEST_VALUE", "fallback"); got != "from-env" {
		t.Fatalf("expected env override, got %s", got)
	}
	if got := envOrDefault("CLOUDTAP_MISSING", "fallback"); got != "fallback" {
		t.Fatalf("expected fallback, got %s", got)
	}
}

func TestEnvOrBool(t *testing.T) {
	t.Setenv("CLOUDTAP_BOOL", "true")
	if !envOrBool("CLOUDTAP_BOOL", false) {
		t.Fatal("expected true from env")
	}
	t.Setenv("CLOUDTAP_INVALID", "notabool")
	if envOrBool("CLOUDTAP_INVALID", true) != true {
		t.Fatal("expected fallback when parse fails")
	}
}

func TestTunnelServerValidations(t *testing.T) {
	fake := stubController{}
	server := tunnelServer{manager: fake}

	if _, err := server.Open(context.Background(), &tunnelv1.OpenRequest{}); err == nil {
		t.Fatal("expected error for missing target")
	}

	fake.openErr = errors.New("boom")
	server = tunnelServer{manager: fake}
	if _, err := server.Open(context.Background(), &tunnelv1.OpenRequest{Target: "demo"}); err == nil {
		t.Fatal("expected propagated error from manager")
	}
}

func TestTunnelServerListTargets(t *testing.T) {
	fake := stubController{
		targets: []string{"demo"},
	}
	server := tunnelServer{manager: fake}
	resp, err := server.ListTargets(context.Background(), &tunnelv1.ListTargetsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.GetTargets()) != 1 || resp.GetTargets()[0] != "demo" {
		t.Fatalf("unexpected targets: %v", resp.GetTargets())
	}
}

type stubController struct {
	openErr  error
	closeErr error
	listErr  error
	targets  []string
}

func (s stubController) Open(context.Context, string) error {
	return s.openErr
}

func (s stubController) Close(context.Context, string) error {
	return s.closeErr
}

func (s stubController) ListTargets(context.Context) ([]string, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.targets, nil
}
