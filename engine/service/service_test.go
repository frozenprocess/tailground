package service

import (
	"testing"

	"github.com/frozenprocess/tailground/engine/eval"
)

func TestBuild_ResolvesAndNormalizes(t *testing.T) {
	reg, diags := Build([]eval.Service{
		{Name: "svc:web", VIP: "100.96.0.1", Backends: []uint64{1, 2}},
		{Name: "  db ", VIP: "fd7a:115c:a1e0::99"},
	})
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	web, ok := reg["web"]
	if !ok {
		t.Fatalf("web not resolved: %v", reg.Names())
	}
	if web.VIP.String() != "100.96.0.1" || len(web.Backends) != 2 {
		t.Errorf("web = %+v", web)
	}
	if web.Column() != "svc:web" {
		t.Errorf("column = %q", web.Column())
	}
	if _, ok := reg["db"]; !ok {
		t.Errorf("db not resolved: %v", reg.Names())
	}
}

func TestBuild_BadVIPDropped(t *testing.T) {
	reg, diags := Build([]eval.Service{
		{Name: "svc:web", VIP: "not-an-ip", Backends: []uint64{1}},
		{Name: "svc:ok", VIP: "100.96.0.2"},
	})
	if _, ok := reg["web"]; ok {
		t.Errorf("web with bad VIP should be dropped")
	}
	if _, ok := reg["ok"]; !ok {
		t.Errorf("ok should still resolve")
	}
	if len(diags) != 1 || diags[0].Severity != "warning" {
		t.Fatalf("want one warning, got %v", diags)
	}
}

func TestBuild_DuplicateNameKeepsFirst(t *testing.T) {
	reg, diags := Build([]eval.Service{
		{Name: "svc:web", VIP: "100.96.0.1"},
		{Name: "web", VIP: "100.96.0.9"},
	})
	if reg["web"].VIP.String() != "100.96.0.1" {
		t.Errorf("want first entry kept, got %v", reg["web"].VIP)
	}
	if len(diags) != 1 {
		t.Fatalf("want one dup warning, got %v", diags)
	}
}

func TestSplit(t *testing.T) {
	for _, tc := range []struct{ in, name, ports string }{
		{"svc:web", "web", ""},
		{"svc:web:443", "web", "443"},
		{"svc:web:80,443", "web", "80,443"},
		{"svc:web:*", "web", "*"},
	} {
		n, p := Split(tc.in)
		if n != tc.name || p != tc.ports {
			t.Errorf("Split(%q) = %q,%q want %q,%q", tc.in, n, p, tc.name, tc.ports)
		}
	}
}

func TestIsToken(t *testing.T) {
	if !IsToken("svc:web") || IsToken("tag:web") || IsToken("group:eng") {
		t.Fatal("IsToken misclassified")
	}
}
