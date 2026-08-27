// Package posture is Tailground's own evaluator for Tailscale device-posture
// conditions (`postures` / `srcPosture`).
//
// Supported:
//
//   - node:os                — string, matched case-insensitively
//   - node:tsVersion         — dotted version, compared numerically
//   - node:tsReleaseTrack    — string ("stable" | "unstable")
//   - custom:<name>          — a custom attribute declared in the policy's
//     `attrConfig` (type string / number / boolean);
//     its per-device value comes from the snapshot.
//
// Operators: == != > >= < <= , the set operator `in [ ... ]`, and && / ||
// between sub-conditions (&& binds tighter). A posture definition is a list of
// condition strings, all of which must hold (AND). A rule's srcPosture is a list
// of posture names, any of which satisfies it (OR).
package posture

import (
	"fmt"
	"strconv"
	"strings"
)

// Set is the parsed `postures` block: posture name → its condition tree.
type Set struct {
	defs map[string]node
}

// Parse turns preflight's raw definitions (name → condition strings) into a Set.
// customTypes maps a declared custom attribute ("custom:env") to its type
// ("string" | "number" | "boolean") from the policy's `attrConfig`; it may be
// nil. Unparseable conditions, and `custom:` references not declared in
// customTypes, are reported as warning strings; a posture with an unparseable
// condition is treated as never-satisfied.
func Parse(raw map[string][]string, customTypes map[string]string) (*Set, []string) {
	s := &Set{defs: make(map[string]node, len(raw))}
	var warnings []string
	for name, conds := range raw {
		var parts []node
		bad := false
		for _, c := range conds {
			n, err := parseExpr(c)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s: %v", name, err))
				bad = true
				continue
			}
			applyTypes(n, customTypes)
			for _, ref := range customRefs(n) {
				if _, declared := customTypes[ref]; !declared {
					warnings = append(warnings, fmt.Sprintf("%s: custom attribute %q is not declared in attrConfig; it is compared as a string", name, ref))
				}
			}
			parts = append(parts, n)
		}
		if bad || len(parts) == 0 {
			s.defs[name] = falseNode{reason: name + " has no evaluable conditions"}
			continue
		}
		s.defs[name] = andAll(parts)
	}
	return s, warnings
}

// applyTypes walks the tree and stamps each comparison leaf with the value type
// implied by its attribute (a declared custom type, or node:tsVersion).
func applyTypes(n node, customTypes map[string]string) {
	switch v := n.(type) {
	case andNode:
		applyTypes(v.l, customTypes)
		applyTypes(v.r, customTypes)
	case orNode:
		applyTypes(v.l, customTypes)
		applyTypes(v.r, customTypes)
	case *cmpNode:
		v.kind = kindFor(v.attr, customTypes)
	case *inNode:
		v.kind = kindFor(v.attr, customTypes)
	}
}

func kindFor(attr string, customTypes map[string]string) valKind {
	if isVersionAttr(attr) {
		return kindVersion
	}
	switch customTypes[attr] {
	case "number", "integer":
		return kindNumber
	case "boolean", "bool":
		return kindBool
	default:
		return kindString
	}
}

// customRefs returns the custom attribute names referenced anywhere in n.
func customRefs(n node) []string {
	var out []string
	var walk func(node)
	walk = func(n node) {
		switch v := n.(type) {
		case andNode:
			walk(v.l)
			walk(v.r)
		case orNode:
			walk(v.l)
			walk(v.r)
		case *cmpNode:
			if strings.HasPrefix(v.attr, "custom:") {
				out = append(out, v.attr)
			}
		case *inNode:
			if strings.HasPrefix(v.attr, "custom:") {
				out = append(out, v.attr)
			}
		}
	}
	walk(n)
	return out
}

// Has reports whether name is a defined posture.
func (s *Set) Has(name string) bool {
	_, ok := s.defs[name]
	return ok
}

// Gate is the posture requirement on one rule: an OR over posture names. The
// zero Gate (no names) always allows.
type Gate struct {
	set   *Set
	names []string
}

// Gate builds the requirement for a rule's srcPosture list.
func (s *Set) Gate(names []string) Gate { return Gate{set: s, names: names} }

// Empty reports whether the gate imposes no requirement.
func (g Gate) Empty() bool { return len(g.names) == 0 }

// Allows reports whether a device with the given attributes satisfies the gate.
// An empty gate always allows.
func (g Gate) Allows(attrs map[string]string) bool {
	if g.Empty() {
		return true
	}
	for _, name := range g.names {
		n, ok := g.set.defs[name]
		if !ok {
			continue
		}
		if ok, _ := n.eval(attrs); ok {
			return true
		}
	}
	return false
}

// Explain returns a one-line human reason for the gate's verdict against attrs.
func (g Gate) Explain(attrs map[string]string) string {
	if g.Empty() {
		return "no posture requirement"
	}
	var misses []string
	for _, name := range g.names {
		n, ok := g.set.defs[name]
		if !ok {
			misses = append(misses, fmt.Sprintf("%s is not defined", name))
			continue
		}
		ok, why := n.eval(attrs)
		if ok {
			return fmt.Sprintf("%s met (%s)", name, why)
		}
		misses = append(misses, fmt.Sprintf("%s not met — %s", name, why))
	}
	if len(misses) == 1 {
		return misses[0]
	}
	return "no posture matched: " + strings.Join(misses, "; ")
}

// --- AST ----------------------------------------------------------------------

// node evaluates against a device's posture attributes. It returns whether the
// condition holds plus a short reason (the determining leaf).
type node interface {
	eval(attrs map[string]string) (bool, string)
}

func andAll(ns []node) node {
	acc := ns[0]
	for _, n := range ns[1:] {
		acc = andNode{acc, n}
	}
	return acc
}

type andNode struct{ l, r node }

func (n andNode) eval(a map[string]string) (bool, string) {
	lok, lw := n.l.eval(a)
	if !lok {
		return false, lw
	}
	rok, rw := n.r.eval(a)
	if !rok {
		return false, rw
	}
	return true, lw + " and " + rw
}

type orNode struct{ l, r node }

func (n orNode) eval(a map[string]string) (bool, string) {
	lok, lw := n.l.eval(a)
	if lok {
		return true, lw
	}
	rok, rw := n.r.eval(a)
	if rok {
		return true, rw
	}
	return false, lw + "; " + rw
}

type falseNode struct{ reason string }

func (n falseNode) eval(map[string]string) (bool, string) { return false, n.reason }

// valKind is how a comparison leaf's operands are compared.
type valKind int

const (
	kindString  valKind = iota // case-insensitive string compare
	kindVersion                // dotted numeric version
	kindNumber                 // numeric
	kindBool                   // boolean (true/false)
)

// attrValue looks up an attribute's per-device value, tolerating the "node:"
// prefix (which postureAttrs trims from map keys).
func attrValue(a map[string]string, attr string) (string, bool) {
	if v, ok := a[strings.TrimPrefix(attr, "node:")]; ok {
		return v, v != ""
	}
	if v, ok := a[attr]; ok {
		return v, v != ""
	}
	return "", false
}

// cmpNode is `<attr> <op> <value>`.
type cmpNode struct {
	attr string // e.g. "node:os", "custom:env"
	op   string
	val  string
	kind valKind
}

func (n *cmpNode) eval(a map[string]string) (bool, string) {
	got, ok := attrValue(a, n.attr)
	if !ok {
		return false, fmt.Sprintf("device reports no %s", n.attr)
	}

	cmp, ordered := compareValues(got, n.val, n.kind)
	var res bool
	switch n.op {
	case "==":
		res = cmp == 0
	case "!=":
		res = cmp != 0
	case ">":
		res = ordered && cmp > 0
	case ">=":
		res = ordered && cmp >= 0
	case "<":
		res = ordered && cmp < 0
	case "<=":
		res = ordered && cmp <= 0
	}
	return res, fmt.Sprintf("%s is %q, need %s %q", n.attr, got, n.op, n.val)
}

// inNode is `<attr> in [a, b, ...]`.
type inNode struct {
	attr string
	vals []string
	kind valKind
}

func (n *inNode) eval(a map[string]string) (bool, string) {
	got, ok := attrValue(a, n.attr)
	if !ok {
		return false, fmt.Sprintf("device reports no %s", n.attr)
	}
	for _, v := range n.vals {
		if cmp, _ := compareValues(got, v, n.kind); cmp == 0 {
			return true, fmt.Sprintf("%s=%s in {%s}", n.attr, got, strings.Join(n.vals, ", "))
		}
	}
	return false, fmt.Sprintf("%s=%s not in {%s}", n.attr, got, strings.Join(n.vals, ", "))
}

// compareValues compares got and want per kind. It returns the sign of the
// comparison and whether the two are meaningfully ordered (false ⇒ only == / !=
// are reliable, e.g. two non-numeric strings under kindNumber, or booleans).
func compareValues(got, want string, kind valKind) (sign int, ordered bool) {
	switch kind {
	case kindVersion:
		return compareVersions(got, want), true
	case kindNumber:
		g, gerr := strconv.ParseFloat(strings.TrimSpace(got), 64)
		w, werr := strconv.ParseFloat(strings.TrimSpace(want), 64)
		if gerr == nil && werr == nil {
			switch {
			case g < w:
				return -1, true
			case g > w:
				return 1, true
			default:
				return 0, true
			}
		}
		return strings.Compare(strings.ToLower(got), strings.ToLower(want)), false
	case kindBool:
		return strings.Compare(normalizeBool(got), normalizeBool(want)), false
	default:
		// kindString: an all-numeric pair still compares numerically so
		// `custom:score > '80'` behaves even without an attrConfig type.
		if g, gerr := strconv.ParseFloat(strings.TrimSpace(got), 64); gerr == nil {
			if w, werr := strconv.ParseFloat(strings.TrimSpace(want), 64); werr == nil {
				switch {
				case g < w:
					return -1, true
				case g > w:
					return 1, true
				default:
					return 0, true
				}
			}
		}
		return strings.Compare(strings.ToLower(got), strings.ToLower(want)), true
	}
}

func normalizeBool(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "yes":
		return "true"
	case "false", "0", "no", "":
		return "false"
	default:
		return strings.ToLower(s)
	}
}

func isVersionAttr(attr string) bool {
	return strings.EqualFold(attr, "node:tsVersion")
}

// compareVersions compares dotted numeric versions ("1.80" vs "1.80.0" → 0,
// "1.78.2" vs "1.80" → -1). Non-numeric suffixes ("-pre") are ignored.
func compareVersions(a, b string) int {
	pa, pb := parseVersion(a), parseVersion(b)
	for i := 0; i < 4; i++ {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

func parseVersion(v string) []int {
	fields := strings.FieldsFunc(v, func(r rune) bool { return r == '.' || r == '-' || r == '+' })
	out := make([]int, 0, len(fields))
	for _, f := range fields {
		num := f
		for i, r := range f {
			if r < '0' || r > '9' {
				num = f[:i]
				break
			}
		}
		if num == "" {
			break
		}
		n, err := strconv.Atoi(num)
		if err != nil {
			break
		}
		out = append(out, n)
	}
	return out
}
