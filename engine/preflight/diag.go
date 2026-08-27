package preflight

import (
	"fmt"

	"github.com/frozenprocess/tailground/engine/eval"
)

// note appends one diagnostic. Severity is "info" | "warning" | "error"; code
// is "policy" (the policy text is at fault) or "snapshot" (the mock tailnet is).
func (r *Result) note(severity, code, ptr, format string, args ...any) {
	r.Diagnostics = append(r.Diagnostics, eval.Diagnostic{
		Severity: severity, Code: code, Pointer: ptr,
		Message: fmt.Sprintf(format, args...),
	})
}

func (r *Result) warn(code, ptr, format string, args ...any) {
	r.note("warning", code, ptr, format, args...)
}

func (r *Result) info(code, ptr, format string, args ...any) {
	r.note("info", code, ptr, format, args...)
}
