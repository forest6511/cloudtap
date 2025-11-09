package policy

import (
	"context"
	"fmt"
)

// Route captures the identifying information required to evaluate access.
type Route struct {
	Name        string
	Destination string
}

// Evaluator decides whether a given route should be allowed. The upcoming
// implementation will enforce role-based controls and token lifecycles.
type Evaluator struct{}

// Allow currently rejects every request until real policy checks arrive.
func (Evaluator) Allow(ctx context.Context, route Route) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return fmt.Errorf("route %s is denied (policy stub)", route.Name)
	}
}
