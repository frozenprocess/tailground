//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tailscale/hujson"
)

// syntheticTagOwner is the placeholder principal the engine side substitutes for
// an unparseable tagOwners value (see PolicyForEngine).
const syntheticTagOwner = "tag-owner@tg-e2e.invalid"

func PolicyForEngine(raw string) (string, error) {
	m, err := parsePolicy(raw)
	if err != nil {
		return "", err
	}
	owners, ok := m["tagOwners"].(map[string]any)
	if !ok {
		return raw, nil
	}
	changed := false
	for tag, v := range owners {
		list, ok := v.([]any)
		if !ok {
			continue
		}
		hasAutogroup := false
		for _, item := range list {
			if s, ok := item.(string); ok && strings.HasPrefix(s, "autogroup:") {
				hasAutogroup = true
				break
			}
		}
		if hasAutogroup {
			owners[tag] = []any{syntheticTagOwner}
			changed = true
		}
	}
	if !changed {
		return raw, nil
	}
	out, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func ProvisioningPolicy(raw string) (string, error) {
	m, err := parsePolicy(raw)
	if err != nil {
		return "", err
	}
	prov := map[string]any{
		"grants": []any{map[string]any{
			"src": []any{"*"}, "dst": []any{"*"}, "ip": []any{"*"},
		}},
	}
	for _, k := range []string{"tagOwners", "groups", "hosts"} {
		if v, ok := m[k]; ok {
			prov[k] = v
		}
	}
	out, err := json.Marshal(prov)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func stripTopLevel(raw string, keys ...string) (string, bool, error) {
	m, err := parsePolicy(raw)
	if err != nil {
		return "", false, err
	}
	removed := false
	for _, k := range keys {
		if _, ok := m[k]; ok {
			delete(m, k)
			removed = true
		}
	}
	if !removed {
		return raw, false, nil
	}
	out, err := json.Marshal(m)
	if err != nil {
		return "", false, err
	}
	return string(out), true, nil
}

// attrConfigTypes reads the policy's `attrConfig` section into a
// custom-attribute-name → type map ("string" | "bool" | "number"; the aliases
// "boolean"/"integer" are normalised). Missing/blank type ⇒ "string".
func attrConfigTypes(raw string) (map[string]string, error) {
	m, err := parsePolicy(raw)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	ac, ok := m["attrConfig"].(map[string]any)
	if !ok {
		return out, nil
	}
	for name, v := range ac {
		typ := "string"
		if cfg, ok := v.(map[string]any); ok {
			if t, ok := cfg["type"].(string); ok && t != "" {
				switch strings.ToLower(t) {
				case "number", "integer":
					typ = "number"
				case "bool", "boolean":
					typ = "bool"
				default:
					typ = "string"
				}
			}
		}
		out[name] = typ
	}
	return out, nil
}

func parsePolicy(raw string) (map[string]any, error) {
	v, err := hujson.Parse([]byte(raw))
	if err != nil {
		return nil, fmt.Errorf("parse policy: %w", err)
	}
	v.Standardize()
	var m map[string]any
	if err := json.Unmarshal(v.Pack(), &m); err != nil {
		return nil, fmt.Errorf("decode policy: %w", err)
	}
	return m, nil
}
