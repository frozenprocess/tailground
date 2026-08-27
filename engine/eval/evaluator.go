package eval

// Evaluator compiles a policy against a mock tailnet and returns the reachability
// Result. Headscale's policy engine is one implementation (engine/adapters/hs);
// the web app only ever sees this interface and the types in this package.
type Evaluator interface {
	Evaluate(req Request) Result
}
