package eval

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

// This is a serialization contract, not an evaluator golden. The web test
// compiles the same JSON as a TypeScript literal against its public interfaces.
func TestWireContract(t *testing.T) {
	raw, err := os.ReadFile("testdata/wire.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]any{
		"request": &Request{}, "result": &Result{},
		"minimalRequest": &Request{}, "minimalResult": &Result{},
	} {
		t.Run(name, func(t *testing.T) {
			decoder := json.NewDecoder(bytes.NewReader(fixtures[name]))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(value); err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			var want, got any
			if err := json.Unmarshal(fixtures[name], &want); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(encoded, &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(want, got) {
				t.Fatalf("wire shape changed:\nwant %s\ngot %s", fixtures[name], encoded)
			}
			if name == "request" || name == "result" {
				assertWireFields(t, reflect.TypeOf(value), want, name)
			}
		})
	}
	var result Result
	if err := json.Unmarshal(fixtures["result"], &result); err != nil {
		t.Fatal(err)
	}
	if result.Matrices["https"]["1->2"] != VerdictAllow || result.Matrices["https"]["2->1"] != VerdictDeny {
		t.Fatal("L4 verdict constants drifted")
	}
	for key, verdict := range map[string]string{"root@1->2": SSHAccept, "ubuntu@1->2": SSHCheck, "root@2->1": SSHReject, "ubuntu@2->1": SSHDeny} {
		if result.SSH[key] != verdict {
			t.Fatalf("SSH verdict drifted: %s", key)
		}
	}
	if result.NodeCaps[1]["example.com/cap"][0] != "{}" {
		t.Fatal("numeric ID map keys or opaque capability payload changed")
	}
}

// Require the full fixture to exercise every struct field, including optional
// ones, so adding a Go field requires consciously updating the shared contract.
func assertWireFields(t *testing.T, typ reflect.Type, value any, path string) {
	t.Helper()
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	switch typ.Kind() {
	case reflect.Struct:
		object, ok := value.(map[string]any)
		if !ok {
			t.Fatalf("%s: expected object", path)
		}
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			v, ok := object[name]
			if !ok {
				t.Errorf("%s.%s absent from full fixture", path, name)
				continue
			}
			assertWireFields(t, field.Type, v, path+"."+name)
		}
	case reflect.Slice:
		// One representative element covers optional fields; other elements may
		// intentionally demonstrate omission.
		list, ok := value.([]any)
		if ok && len(list) > 0 {
			assertWireFields(t, typ.Elem(), list[0], path+"[]")
		}
	case reflect.Map:
		for key, v := range value.(map[string]any) {
			assertWireFields(t, typ.Elem(), v, path+"["+key+"]")
		}
	}
}

func TestWireEmptyAndNull(t *testing.T) {
	for _, tc := range []struct {
		value any
		want  string
	}{
		{Request{}, `{"policy":"","snapshot":{"users":null,"devices":null},"probes":null}`},
		{Result{}, `{"matrices":null,"actors":null,"engine":{"headscaleCommit":"","tailscaleVersion":""}}`},
		{Result{Matrices: map[string]map[string]string{}, Actors: []Actor{}, Diagnostics: []Diagnostic{}}, `{"matrices":{},"actors":[],"engine":{"headscaleCommit":"","tailscaleVersion":""}}`},
	} {
		got, err := json.Marshal(tc.value)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != tc.want {
			t.Fatalf("want %s; got %s", tc.want, got)
		}
	}
}
