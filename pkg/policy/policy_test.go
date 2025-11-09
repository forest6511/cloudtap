package policy

import (
	"context"
	"testing"
)

func TestEvaluatorDeniesByDefault(t *testing.T) {
	err := (Evaluator{}).Allow(context.Background(), Route{Name: "svc", Destination: "demo"})
	if err == nil {
		t.Fatal("expected deny error from placeholder evaluator")
	}
}
