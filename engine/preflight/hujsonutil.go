package preflight

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/tailscale/hujson"
)

func arrayMember(obj *hujson.Object, key string) *hujson.Array {
	for _, m := range obj.Members {
		if strings.EqualFold(literalString(m.Name.Value), key) {
			if a, ok := m.Value.Value.(*hujson.Array); ok {
				return a
			}
		}
	}
	return nil
}

func objectMember(obj *hujson.Object, key string) *hujson.Object {
	for _, m := range obj.Members {
		if strings.EqualFold(literalString(m.Name.Value), key) {
			if o, ok := m.Value.Value.(*hujson.Object); ok {
				return o
			}
		}
	}
	return nil
}

// parseHosts reads the policy's `hosts` object (name → IP/CIDR string) so the
// ipset resolver can follow `host:<name>` members. Returns nil if absent.
func parseHosts(obj *hujson.Object) map[string]string {
	for _, m := range obj.Members {
		if strings.EqualFold(literalString(m.Name.Value), "hosts") {
			return mapMembers(m.Value, func(v hujson.Value) (string, bool) {
				return stringLiteral(v.Value)
			})
		}
	}
	return nil
}

// parseAttrConfig reads the policy's `attrConfig` section — an object keyed by a
// custom attribute name ("custom:env"), each value an ACLAttrConfig object
// (tailscale/tailscale-client-go-v2#29, tailscale/tailscale#16167). Only `type`
// ("string" | "bool" | "number"; "boolean"/"integer" tolerated as aliases) is
// read here; a missing/blank type defaults to "string". The sibling fields
// `allowSetByNode` and `broadcastToPeers` are write/propagation controls and
// don't affect static evaluation from a snapshot, so they're ignored.
func parseAttrConfig(v hujson.Value) map[string]string {
	return mapMembers(v, func(v hujson.Value) (string, bool) {
		typ := "string"
		if cfg, ok := v.Value.(*hujson.Object); ok {
			if t := literalString(objectMemberValue(cfg, "type")); t != "" {
				typ = strings.ToLower(t)
			}
		}
		return typ, true
	})
}

// objectMemberValue returns the value of obj[key] as a ValueTrimmed, or nil.
func objectMemberValue(obj *hujson.Object, key string) hujson.ValueTrimmed {
	for _, m := range obj.Members {
		if strings.EqualFold(literalString(m.Name.Value), key) {
			return m.Value.Value
		}
	}
	return nil
}

// parseNameLists reads a JSON object whose values are arrays of strings
// (`postures`, `ipsets`) into a name → []string map.
func parseNameLists(v hujson.Value) map[string][]string {
	return mapMembers(v, func(v hujson.Value) ([]string, bool) {
		arr, ok := v.Value.(*hujson.Array)
		if !ok {
			return nil, false
		}
		elems := stringElems(arr)
		return elems, len(elems) > 0 // a name with no usable entries is absent
	})
}

// mapMembers reads a JSON object into a name → T map, keeping the members fn
// accepts. A non-object v yields an empty map.
func mapMembers[T any](v hujson.Value, fn func(hujson.Value) (T, bool)) map[string]T {
	out := map[string]T{}
	if o, ok := v.Value.(*hujson.Object); ok {
		for _, m := range o.Members {
			if val, ok := fn(m.Value); ok {
				out[literalString(m.Name.Value)] = val
			}
		}
	}
	return out
}

// stringElems returns the string-literal elements of arr, skipping the rest.
func stringElems(arr *hujson.Array) []string {
	var out []string
	for _, e := range arr.Elements {
		if s, ok := stringLiteral(e.Value); ok {
			out = append(out, s)
		}
	}
	return out
}

// literalString returns the string content of a hujson string literal, or "".
func literalString(vt hujson.ValueTrimmed) string {
	s, _ := stringLiteral(vt)
	return s
}

func stringLiteral(vt hujson.ValueTrimmed) (string, bool) {
	lit, ok := vt.(hujson.Literal)
	if !ok || lit.Kind() != '"' {
		return "", false
	}
	return lit.String(), true
}

// lineCol renders a "line:col" pointer (1-indexed) for a byte offset.
func lineCol(src string, off int) string {
	if off < 0 || off > len(src) {
		return ""
	}
	line, col := 1, 1
	for i := 0; i < off; i++ {
		if src[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return fmt.Sprintf("%d:%d", line, col)
}

var hujsonLineCol = regexp.MustCompile(`line (\d+), column (\d+)`)

// hujsonErrPointer pulls the "line:col" out of a HuJSON syntax error, so the
// error can be shown at its location instead of as a loose message.
func hujsonErrPointer(err error) string {
	if m := hujsonLineCol.FindStringSubmatch(err.Error()); m != nil {
		return m[1] + ":" + m[2]
	}
	return ""
}
