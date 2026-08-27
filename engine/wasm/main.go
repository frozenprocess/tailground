//go:build js && wasm

// Command engine is the WASM entrypoint. It registers a single synchronous
// function on the JS side:
//
//	globalThis.tailground.evaluate(requestJSON string) -> resultJSON string
//
// The web app runs this inside a Web Worker. A panic in the
// engine is caught and returned as a Result carrying one "internal" error
// diagnostic, so a bad input can never take the worker down.
package main

import (
	"encoding/json"
	"fmt"
	"syscall/js"

	"github.com/frozenprocess/tailground/engine/adapters/hs"
	"github.com/frozenprocess/tailground/engine/eval"
)

func main() {
	js.Global().Set("tailground", js.ValueOf(map[string]any{
		"evaluate": js.FuncOf(evaluate),
	}))
	// Keep the Go runtime alive for the lifetime of the worker.
	select {}
}

func evaluate(_ js.Value, args []js.Value) (out any) {
	defer func() {
		if r := recover(); r != nil {
			out = mustJSON(eval.Result{
				Matrices: map[string]map[string]string{},
				Diagnostics: []eval.Diagnostic{{
					Severity: "error", Code: "internal",
					Message: fmt.Sprintf("engine panic: %v", r),
				}},
			})
		}
	}()

	if len(args) != 1 || args[0].Type() != js.TypeString {
		return mustJSON(eval.Result{
			Matrices: map[string]map[string]string{},
			Diagnostics: []eval.Diagnostic{{
				Severity: "error", Code: "internal",
				Message: "evaluate(requestJSON string) expects exactly one string argument",
			}},
		})
	}

	var req eval.Request
	if err := json.Unmarshal([]byte(args[0].String()), &req); err != nil {
		return mustJSON(eval.Result{
			Matrices: map[string]map[string]string{},
			Diagnostics: []eval.Diagnostic{{
				Severity: "error", Code: "internal",
				Message: "parsing request JSON: " + err.Error(),
			}},
		})
	}

	return mustJSON(hs.New().Evaluate(req))
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return `{"matrices":{},"diagnostics":[{"severity":"error","code":"internal","message":"marshaling result failed"}]}`
	}
	return string(b)
}
