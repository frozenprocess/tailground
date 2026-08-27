package preflight

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/tailscale/hujson"

	"github.com/frozenprocess/tailground/engine/extshare"
)

// otherSchemeRE matches a "<scheme>://" token that isn't group:// or tag://.
var otherSchemeRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*://`)

// checkExternalRefPlacement reports, as fatal, every group:// / tag://
// reference outside a grant's src, and every other "<scheme>://" token in a
// rule's src / dst / via. It runs over the whole policy before any section is
// stripped, so references in ipsets, groups, hosts, … are caught too.
func checkExternalRefPlacement(raw string, obj *hujson.Object, res *Result) {
	for _, m := range obj.Members {
		top := literalString(m.Name.Value)
		switch strings.ToLower(top) {
		case "externaltailnets", "postures", "attrconfig", "derpmap":
			continue // config, not targets
		}
		walkStrings(m.Value, []string{top}, func(path []string, s string, v hujson.Value) {
			if msg := externalRefPlacementError(path, s); msg != "" {
				res.note("error", "policy", lineCol(raw, v.StartOffset), "%s", msg)
				res.Fatal = true
			}
		})
	}
}

// externalRefPlacementError returns the error for string s at path, or "".
func externalRefPlacementError(path []string, s string) string {
	top := strings.ToLower(path[0])
	field := ""
	if len(path) >= 3 {
		field = strings.ToLower(path[2])
	}
	isRule := top == "acls" || top == "grants" || top == "ssh"
	if isRule && len(path) >= 3 && field == "app" {
		return "" // capability parameters are free-form JSON
	}
	loc := formatPath(path)
	if isRule && len(path) == 4 {
		loc = formatPath(path[:3]) // "acls[0].src", like the grant src messages
	}

	if !extshare.IsRef(s) {
		// Tailscale accepts user://…; what it means isn't known, so it's left
		// to the engine. Any other scheme in a rule's target is rejected.
		t := strings.TrimSpace(s)
		if isRule && len(path) == 4 && (field == "src" || field == "dst" || field == "via") &&
			otherSchemeRE.MatchString(t) && !strings.HasPrefix(strings.ToLower(t), "user://") {
			return fmt.Sprintf("%s %q: unsupported target — the only cross-tailnet references are group://<tailnet>/<group> and tag://<tailnet>/<tag>", loc, s)
		}
		return ""
	}

	switch {
	case top == "grants" && len(path) == 4 && field == "src":
		return "" // the one valid place; checked further in rewriteExternalRefTokens
	case top == "acls" && (field == "src" || field == "dst"):
		return fmt.Sprintf("%s %q: unsupported target — external references are only valid in a grant's src, not in acls; write this rule as a grant", loc, s)
	case top == "grants" && field == "dst":
		return fmt.Sprintf("%s %q: unsupported target — an external reference is only valid as a grant's src; access to another tailnet's nodes is granted by that tailnet's own policy (list the local tags/groups it may name in allowExternalReferencesTo)", loc, s)
	case top == "ssh":
		return fmt.Sprintf("%s %q: external references aren't supported in ssh rules; Tailscale rejects the policy", loc, s)
	case top == "tests" || top == "sshtests":
		return fmt.Sprintf("%s %q: external references can't be used in tests; Tailscale fails the policy's tests", loc, s)
	}
	return fmt.Sprintf("%s %q: unsupported target — external references are only valid in a grant's src", loc, s)
}

// checkGrantSrcRef applies Tailscale's checks to a group:// / tag:// reference
// in a grant's src, in Tailscale's order: well-formed, then the alias is in
// externalTailnets, then that tailnet allows incoming connections. It returns
// the parsed reference and the canonical alias, or an error message.
func checkGrantSrcRef(loc, s string, cfg map[string]extshare.TailnetConfig) (extshare.Ref, string, string) {
	ref, ok := extshare.ParseRef(s)
	if !ok {
		return ref, "", fmt.Sprintf("%s %q: unsupported target — expected tag://<tailnet>/<tag> or group://<tailnet>/<group>%s", loc, s, nonASCIIHint(s))
	}
	alias, c, ok := findConfig(cfg, ref.Alias)
	if !ok {
		return ref, "", fmt.Sprintf("%s %q: external tailnet not found: %q — externalTailnets has no entry with that name%s", loc, s, ref.Alias, nonASCIIHint(ref.Alias))
	}
	if !c.AllowIncomingConnections {
		return ref, "", fmt.Sprintf("%s %q: external tailnet not allowed in local policy grants: %q — set \"allowIncomingConnections\": true in its externalTailnets entry to accept connections from it", loc, s, alias)
	}
	return ref, alias, ""
}

// findConfig looks an alias up in the externalTailnets config. Tailscale
// matches aliases case-insensitively.
func findConfig(cfg map[string]extshare.TailnetConfig, alias string) (string, extshare.TailnetConfig, bool) {
	if c, ok := cfg[alias]; ok {
		return alias, c, true
	}
	for a, c := range cfg {
		if strings.EqualFold(a, alias) {
			return a, c, true
		}
	}
	return "", extshare.TailnetConfig{}, false
}

// checkExternalTailnetIDs reports, as fatal, every externalTailnets entry with
// no externalID: Tailscale rejects it with "missing externalTailnet ID".
func checkExternalTailnetIDs(raw string, v hujson.Value, res *Result) {
	o, ok := v.Value.(*hujson.Object)
	if !ok {
		return
	}
	for _, m := range o.Members {
		alias := literalString(m.Name.Value)
		id := ""
		if cfg, ok := m.Value.Value.(*hujson.Object); ok {
			for _, cm := range cfg.Members {
				switch strings.ToLower(literalString(cm.Name.Value)) {
				case "externalid", "id":
					id = strings.TrimSpace(literalString(cm.Value.Value))
				}
			}
		}
		if id == "" {
			res.note("error", "policy", lineCol(raw, m.Name.StartOffset), "externalTailnets[%q]: missing externalTailnet ID — set \"externalID\" to the other tailnet's ID", alias)
			res.Fatal = true
		}
	}
}

// nonASCIIHint names the first non-ASCII character in s. A lookalike slash
// (∕ U+2215, ／ U+FF0F) or a zero-width space renders like the real thing in
// an error message, so without this the reference looks correct.
func nonASCIIHint(s string) string {
	for _, r := range s {
		if r >= utf8.RuneSelf {
			return fmt.Sprintf(" (it contains %U %q, which isn't ASCII — retype it by hand)", r, r)
		}
	}
	return ""
}

// walkStrings calls fn for every string value under v, with its path.
func walkStrings(v hujson.Value, path []string, fn func(path []string, s string, v hujson.Value)) {
	switch t := v.Value.(type) {
	case *hujson.Object:
		for _, m := range t.Members {
			walkStrings(m.Value, append(path[:len(path):len(path)], literalString(m.Name.Value)), fn)
		}
	case *hujson.Array:
		for i, e := range t.Elements {
			walkStrings(e, append(path[:len(path):len(path)], strconv.Itoa(i)), fn)
		}
	default:
		if s, ok := stringLiteral(v.Value); ok {
			fn(path, s, v)
		}
	}
}

// formatPath renders a walkStrings path as "grants[0].src", or
// `groups["group:ext"][0]` for a key that isn't a plain identifier.
func formatPath(path []string) string {
	var b strings.Builder
	b.WriteString(path[0])
	for _, p := range path[1:] {
		switch {
		case isIndex(p):
			b.WriteString("[" + p + "]")
		case isIdent(p):
			b.WriteString("." + p)
		default:
			b.WriteString(fmt.Sprintf("[%q]", p))
		}
	}
	return b.String()
}

func isIndex(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil && s != ""
}

func isIdent(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}
