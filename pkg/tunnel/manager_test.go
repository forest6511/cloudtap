package tunnel

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeSource struct {
	targets []Target
	err     error
}

func (f fakeSource) Targets(ctx context.Context) ([]Target, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make([]Target, len(f.targets))
	copy(out, f.targets)
	return out, nil
}

type fakeDialer struct {
	err error
}

func (f fakeDialer) Connect(ctx context.Context, target Target) error {
	if f.err != nil {
		return f.err
	}
	return nil
}

func TestManagerRefreshAndList(t *testing.T) {
	ctx := context.Background()
	src := fakeSource{targets: []Target{{Name: "b"}, {Name: "a"}}}
	m := NewManager(nil, WithSource(src), WithDialer(fakeDialer{}), WithRefreshInterval(time.Millisecond))

	names, err := m.ListTargets(ctx)
	if err != nil {
		t.Fatalf("ListTargets error: %v", err)
	}
	if len(names) != 2 || names[0] != "a" || names[1] != "b" {
		t.Fatalf("unexpected names %v", names)
	}
}

func TestManagerOpenUsesDialer(t *testing.T) {
	ctx := context.Background()
	expected := errors.New("boom")
	src := fakeSource{targets: []Target{{Name: "svc", Address: "tcp://localhost:1"}}}
	m := NewManager(nil, WithSource(src), WithDialer(fakeDialer{err: expected}), WithRefreshInterval(0))

	if err := m.Open(ctx, "svc"); err != expected {
		t.Fatalf("expected dialer error, got %v", err)
	}
}

func TestManagerCloseRequiresActive(t *testing.T) {
	ctx := context.Background()
	src := fakeSource{targets: []Target{{Name: "svc", Address: "tcp://localhost:1"}}}
	m := NewManager(nil, WithSource(src), WithDialer(fakeDialer{}))

	if err := m.Close(ctx, "svc"); err == nil {
		t.Fatal("expected error when closing inactive tunnel")
	}
}

func TestResolveNetworkAddr(t *testing.T) {
	cases := []struct {
		target Target
		want   string
	}{
		{Target{Name: "a", Address: "grpc://localhost:9000"}, "localhost:9000"},
		{Target{Name: "b", Address: "127.0.0.1:1234"}, "127.0.0.1:1234"},
	}

	for _, tc := range cases {
		_, got, err := ResolveNetworkAddr(tc.target)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if got != tc.want {
			t.Fatalf("want %s got %s", tc.want, got)
		}
	}
}
