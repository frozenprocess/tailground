// Package schema holds no code — only the test that keeps the web app's
// hand-written policy.schema.json in step with the fields the vendored
// Headscale hscontrol/policy/v2 compiler actually accepts.
package schema

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	policyv2 "github.com/juanfont/headscale/hscontrol/policy/v2"
)

// schemaPath is relative to this test file (engine/schema/).
const schemaPath = "../../web/src/schema/policy.schema.json"

// TestSchemaCoversEveryPolicyField asserts that every JSON field name reachable
// from policyv2.Policy appears somewhere as an object key in policy.schema.json.
// It is deliberately shallow: it proves coverage, not correctness of the shapes.
// When Headscale adds a policy field, this test fails until the schema learns it.
func TestSchemaCoversEveryPolicyField(t *testing.T) {
	want := map[string]bool{}
	collectJSONFields(reflect.TypeOf(policyv2.Policy{}), want, map[reflect.Type]bool{})

	raw, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read %s: %v", schemaPath, err)
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse %s: %v", schemaPath, err)
	}
	have := map[string]bool{}
	collectObjectKeys(doc, have)

	var missing []string
	for name := range want {
		if !have[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("policy.schema.json is missing field(s) the v2 compiler accepts: %s\n"+
			"add them (as a property or $def key) to %s", strings.Join(missing, ", "), schemaPath)
	}
}

// collectJSONFields walks a struct type, recording the name from every explicit
// `json:"..."` tag and recursing into nested struct types (through pointers,
// slices, arrays and map values). Fields without a json tag are skipped, so
// helper structs that unmarshal from a scalar (SSHCheckPeriod, AliasWithPorts)
// contribute nothing.
func collectJSONFields(t reflect.Type, out map[string]bool, seen map[reflect.Type]bool) {
	for t != nil && (t.Kind() == reflect.Ptr || t.Kind() == reflect.Slice || t.Kind() == reflect.Array || t.Kind() == reflect.Map) {
		if t.Kind() == reflect.Map {
			t = t.Elem()
			continue
		}
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct || seen[t] {
		return
	}
	seen[t] = true

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if tag, ok := f.Tag.Lookup("json"); ok {
			name := strings.Split(tag, ",")[0]
			if name != "" && name != "-" {
				out[name] = true
			}
		}
		collectJSONFields(f.Type, out, seen)
	}
}

// collectObjectKeys records every key of every JSON object anywhere in the doc.
func collectObjectKeys(v any, out map[string]bool) {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			out[k] = true
			collectObjectKeys(child, out)
		}
	case []any:
		for _, child := range t {
			collectObjectKeys(child, out)
		}
	}
}
