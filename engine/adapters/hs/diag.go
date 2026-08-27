package hs

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/frozenprocess/tailground/engine/eval"
	"github.com/frozenprocess/tailground/engine/preflight"
)

// postureRef matches a posture-gated rule's compile message
// ("posture-gated grants[3]: <err>").
var postureRef = regexp.MustCompile(`^posture-gated (acls|grants|ssh)\[(\d+)\]`)

// locateUnhomed gives a policy position to compile messages that came back
// without one, so they show at their token in the editor. A posture-gated
// rule is compiled on its own (as rule 0 of a one-rule policy), so a JSON
// pointer in its error is meaningless — it gets the error's token, or failing
// that the rule itself.
func locateUnhomed(raw string, ds []eval.Diagnostic) {
	for i := range ds {
		d := &ds[i]
		if d.Pointer != "" || d.Target != "" || (d.Code != "parse" && d.Code != "internal") {
			continue
		}
		if m := postureRef.FindStringSubmatch(d.Message); m != nil {
			if d.Pointer = preflight.LocateToken(raw, d.Message); d.Pointer == "" {
				d.Pointer = preflight.LocatePointer(raw, "/"+m[1]+"/"+m[2])
			}
			continue
		}
		d.Pointer = preflight.LocateCompileError(raw, d.Message)
	}
}

// cleanErr trims Headscale's internal wrapping ("parsing policy from bytes: …")
// so the editor shows the useful part.
func cleanErr(err error) string {
	s := err.Error()
	for _, p := range []string{"parsing policy from bytes: ", "parsing policy: ", "compiling policy: "} {
		s = strings.TrimPrefix(s, p)
	}
	return s
}

// perDeviceDiag matches the adapter's per-device compile messages
// ("compiling route filter for web-1: <err>").
var perDeviceDiag = regexp.MustCompile(`^(.+) for ([^\s:,]+): (.+)$`)

// collapseDeviceDiags merges per-device messages that report the same error
// into one ("… for web-1, web-2, db-1: <err>"), so one policy problem shows
// up once instead of once per device. Order is kept (first occurrence wins).
func collapseDeviceDiags(in []eval.Diagnostic) []eval.Diagnostic {
	type group struct {
		at    int
		hosts []string
	}
	groups := map[string]*group{}
	out := make([]eval.Diagnostic, 0, len(in))
	for _, d := range in {
		m := perDeviceDiag.FindStringSubmatch(d.Message)
		if m == nil || d.Pointer != "" {
			out = append(out, d)
			continue
		}
		key := d.Severity + "\x00" + d.Code + "\x00" + m[1] + "\x00" + m[3]
		if g, ok := groups[key]; ok {
			g.hosts = append(g.hosts, m[2])
			out[g.at].Message = fmt.Sprintf("%s for %s: %s", m[1], strings.Join(g.hosts, ", "), m[3])
			continue
		}
		groups[key] = &group{at: len(out), hosts: []string{m[2]}}
		out = append(out, d)
	}
	return out
}

// diag builds a diagnostic with no location.
func diag(severity, code, format string, args ...any) eval.Diagnostic {
	return eval.Diagnostic{Severity: severity, Code: code, Message: fmt.Sprintf(format, args...)}
}

// deviceWarn builds a snapshot warning attached to a device's row in the UI.
func deviceWarn(device, format string, args ...any) eval.Diagnostic {
	d := diag("warning", "snapshot", format, args...)
	d.Target = "device:" + device
	return d
}
