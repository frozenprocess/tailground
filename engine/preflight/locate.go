package preflight

import (
	"regexp"
	"strings"

	"github.com/tailscale/hujson"
)

var (
	// `... within "/grants/0/ip/0": ...` — Go's JSON decoder names the path.
	errJSONPointer = regexp.MustCompile(`within "(/[^"]*)"`)
	// `group not defined in policy: "group:nope"` — the compiler names the token.
	errQuotedToken = regexp.MustCompile(`"([^"/][^"]*)"`)
)

// LocateCompileError maps a Headscale compile error back to a "line:col" in
// the raw policy, so it can be shown at its location rather than as a loose
// message. It tries the JSON-pointer form first, then the quoted-token form.
// It returns "" when neither resolves.
func LocateCompileError(raw, msg string) string {
	if m := errJSONPointer.FindStringSubmatch(msg); m != nil {
		if p := LocatePointer(raw, m[1]); p != "" {
			return p
		}
	}
	return LocateToken(raw, msg)
}

// LocatePointer resolves a JSON pointer against the raw policy, walking up to
// the nearest value that exists (preflight may have rewritten array members).
func LocatePointer(raw, ptr string) string {
	root, err := hujson.Parse([]byte(raw))
	if err != nil {
		return ""
	}
	for ; ptr != ""; ptr = ptr[:strings.LastIndexByte(ptr, '/')] {
		if v := root.Find(ptr); v != nil {
			return lineCol(raw, v.StartOffset)
		}
	}
	return ""
}

// LocateToken finds the first quoted token in msg that appears in the raw
// policy as a string value — exactly, or followed by ":<ports>".
func LocateToken(raw, msg string) string {
	root, err := hujson.Parse([]byte(raw))
	if err != nil {
		return ""
	}
	for _, m := range errQuotedToken.FindAllStringSubmatch(msg, -1) {
		tok := m[1]
		for v := range root.All() {
			if s, ok := stringLiteral(v.Value); ok && (s == tok || strings.HasPrefix(s, tok+":")) {
				return lineCol(raw, v.StartOffset)
			}
		}
	}
	return ""
}
