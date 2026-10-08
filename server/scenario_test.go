package main

import (
	"bytes"
	"compress/flate"
	"encoding/base64"
	"encoding/json"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"testing"
)

// clientTokens are real tokens issued by the web app (including ones built on
// its preset dictionary); the fixtures are shared with the client's own tests.
func clientTokens(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile("../web/src/app/testdata/shares.json")
	if err != nil {
		t.Skipf("client fixtures not available: %v", err)
	}
	var fx []struct{ Token string }
	if err := json.Unmarshal(b, &fx); err != nil || len(fx) == 0 {
		t.Fatalf("fixtures: %v", err)
	}
	out := make([]string, len(fx))
	for i, f := range fx {
		out[i] = f.Token
	}
	return out
}

func deflated(t *testing.T, payload []byte) string {
	t.Helper()
	var buf bytes.Buffer
	w, _ := flate.NewWriter(&buf, 9)
	w.Write(payload)
	w.Close()
	return base64.RawURLEncoding.EncodeToString(buf.Bytes())
}

func TestScenarioCheckAcceptsClientTokens(t *testing.T) {
	for i, tok := range clientTokens(t) {
		if err := looksLikeScenario(tok); err != nil {
			t.Errorf("fixture %d rejected: %v", i, err)
		}
	}
	if err := looksLikeScenario(deflated(t, []byte(`["p",{"users":[],"devices":[]}]`))); err != nil {
		t.Errorf("plain deflate rejected: %v", err)
	}
}

func TestScenarioCheckRejectsJunk(t *testing.T) {
	tok := clientTokens(t)[0]
	rng := rand.New(rand.NewSource(1))
	junk := []string{"abc_DEF-123", strings.Repeat("a", 4000), "A", tok[:len(tok)-8], tok + "AAAA"}
	for i := 0; i < 200; i++ {
		b := make([]byte, 50+rng.Intn(2000))
		rng.Read(b)
		junk = append(junk, base64.RawURLEncoding.EncodeToString(b))
	}
	for _, j := range junk {
		if looksLikeScenario(j) == nil {
			t.Errorf("junk accepted: %.40q", j)
		}
	}
}

func TestScenarioCheckRejectsBomb(t *testing.T) {
	bomb := deflated(t, make([]byte, maxInflatedBytes+1))
	if len(bomb) > maxTokenBytes {
		t.Fatalf("test bomb too big for the API: %d", len(bomb))
	}
	if looksLikeScenario(bomb) == nil {
		t.Fatal("decompression bomb accepted")
	}
}

func TestCreateRunsScenarioCheck(t *testing.T) {
	s, _ := testServer(t)
	s.check = looksLikeScenario
	h := s.routes()
	if c := do(h, "POST", "/api/share", "abc_DEF-123").Code; c != http.StatusBadRequest {
		t.Fatalf("junk token: %d", c)
	}
	if c := do(h, "POST", "/api/share", clientTokens(t)[0]).Code; c != http.StatusOK {
		t.Fatalf("real token: %d", c)
	}
}
