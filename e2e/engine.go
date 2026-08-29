//go:build e2e

package e2e

import (
	"github.com/frozenprocess/tailground/engine/adapters/hs"
	"github.com/frozenprocess/tailground/engine/eval"
)

func Evaluate(req eval.Request) eval.Result {
	return hs.New().Evaluate(req)
}

// EvaluateForEngine normalizes the policy (PolicyForEngine — tagOwners the
// Headscale compiler can't parse) and then evaluates. Returns the normalized
// policy alongside the result so callers can record exactly what was scored.
func EvaluateForEngine(req eval.Request) (eval.Result, string, error) {
	pol, err := PolicyForEngine(req.Policy)
	if err != nil {
		return eval.Result{}, "", err
	}
	req.Policy = pol
	return Evaluate(req), pol, nil
}
