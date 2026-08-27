package preflight

import (
	"strconv"
	"strings"

	"github.com/tailscale/hujson"
)

// sshRecorderKeys are the Tailscale SSH session-recording fields (an Enterprise
// feature). The Headscale engine's SSH type doesn't define them, so — with its
// strict unmarshal — leaving them in fails the whole compile with an
// `unknown field` error. Session recording has no bearing on reachability,
// so preflight drops them silently and the rule still evaluates.
var sshRecorderKeys = map[string]bool{"recorder": true, "enforcerecorder": true}

// stripSSHRecorder removes the session-recording keys from every ssh[] rule.
func stripSSHRecorder(obj *hujson.Object) {
	arr := arrayMember(obj, "ssh")
	if arr == nil {
		return
	}
	for i := range arr.Elements {
		el, ok := arr.Elements[i].Value.(*hujson.Object)
		if !ok {
			continue
		}
		kept := el.Members[:0]
		for _, mm := range el.Members {
			if !sshRecorderKeys[strings.ToLower(literalString(mm.Name.Value))] {
				kept = append(kept, mm)
			}
		}
		el.Members = kept
	}
}

// stripAutoApproverServices removes autoApprovers.services. Tailscale accepts
// it (auto-approval of tagged Tailscale Service hosts) but Headscale's
// AutoApproverPolicy has only routes/exitNode and rejects the unknown member,
// failing the whole policy. Approval never changes the packet filter, so it is
// dropped silently like the other un-modeled, verdict-neutral keys.
func stripAutoApproverServices(obj *hujson.Object) {
	aa := objectMember(obj, "autoApprovers")
	if aa == nil {
		return
	}
	kept := aa.Members[:0]
	for _, m := range aa.Members {
		if !strings.EqualFold(literalString(m.Name.Value), "services") {
			kept = append(kept, m)
		}
	}
	aa.Members = kept
}

// nodeAttrUnsupported lists `attr` values the Headscale engine rejects outright
// (it would advertise a feature that doesn't work). Preflight drops them with a
// note so the rest of the nodeAttrs entry still applies.
var nodeAttrUnsupported = map[string]bool{"funnel": true}

// sanitizeNodeAttrs lifts the `app` key out of each nodeAttrs entry (Headscale
// rejects it) into Result.AppNodeCaps — stamping the entry with an
// [AppTargetCapPrefix] marker attr so the adapter can resolve its targets — and
// drops any unsupported `attr` values, keeping the rest.
func sanitizeNodeAttrs(raw string, obj *hujson.Object, res *Result) {
	arr := arrayMember(obj, "nodeAttrs")
	if arr == nil {
		return
	}
	for i := range arr.Elements {
		el, ok := arr.Elements[i].Value.(*hujson.Object)
		if !ok {
			continue
		}
		kept := el.Members[:0]
		markApp := false
		for _, mm := range el.Members {
			switch strings.ToLower(literalString(mm.Name.Value)) {
			case "app":
				if caps := parseAppCaps(mm.Value); len(caps) > 0 {
					if res.AppNodeCaps == nil {
						res.AppNodeCaps = map[int]map[string][]string{}
					}
					res.AppNodeCaps[i] = caps
					markApp = true
				}
				continue
			case "attr":
				if a, ok := mm.Value.Value.(*hujson.Array); ok {
					keptAttrs := a.Elements[:0]
					for _, e := range a.Elements {
						if s, ok := stringLiteral(e.Value); ok && nodeAttrUnsupported[strings.ToLower(s)] {
							res.note("info", "limit", lineCol(raw, e.StartOffset), "nodeAttrs[%d] attr %q (public-internet exposure) isn't simulated — the graph won't show the internet reaching these devices", i, s)
							continue
						}
						keptAttrs = append(keptAttrs, e)
					}
					a.Elements = keptAttrs
				}
			}
			kept = append(kept, mm)
		}
		el.Members = kept
		if markApp {
			addAttrMarker(el, AppTargetCapPrefix+strconv.Itoa(i))
		}
	}
}

// parseAppCaps reads a nodeAttrs `app` object (capability name → list of config
// values) into a cap → JSON-value-string map.
func parseAppCaps(v hujson.Value) map[string][]string {
	o, ok := v.Value.(*hujson.Object)
	if !ok {
		return nil
	}
	out := map[string][]string{}
	for _, m := range o.Members {
		name := literalString(m.Name.Value)
		if name == "" {
			continue
		}
		if a, ok := m.Value.Value.(*hujson.Array); ok {
			for k := range a.Elements {
				out[name] = append(out[name], standardJSON(a.Elements[k]))
			}
			continue
		}
		out[name] = append(out[name], standardJSON(m.Value))
	}
	return out
}

// standardJSON renders a hujson value as compact standard JSON, dropping any
// whitespace, comments and trailing commas it carried.
func standardJSON(v hujson.Value) string {
	c := v.Clone()
	c.Minimize()
	return string(c.Pack())
}

// addAttrMarker appends marker to the entry's `attr` array, creating the array
// if the entry has none.
func addAttrMarker(el *hujson.Object, marker string) {
	mv := hujson.Value{Value: hujson.String(marker)}
	for k := range el.Members {
		if !strings.EqualFold(literalString(el.Members[k].Name.Value), "attr") {
			continue
		}
		if a, ok := el.Members[k].Value.Value.(*hujson.Array); ok {
			a.Elements = append(a.Elements, mv)
			return
		}
	}
	el.Members = append(el.Members, hujson.ObjectMember{
		Name:  hujson.Value{Value: hujson.String("attr")},
		Value: hujson.Value{Value: &hujson.Array{Elements: []hujson.Value{mv}}},
	})
}
