package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testServer(t *testing.T) (*server, *Store) {
	t.Helper()
	st, err := OpenStore(filepath.Join(t.TempDir(), "t.db"), 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return &server{
		store:  st,
		limit:  newLimiter(3, time.Hour),
		global: newLimiter(1000, time.Hour),
		misses: newLimiter(3, time.Hour),
	}, st
}

func do(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRoundTripAndDedupe(t *testing.T) {
	s, _ := testServer(t)
	h := s.routes()
	a := do(h, "POST", "/api/share", "abc_DEF-123")
	b := do(h, "POST", "/api/share", "abc_DEF-123")
	if a.Code != 200 || a.Body.String() != b.Body.String() {
		t.Fatalf("create/dedupe: %d %q vs %q", a.Code, a.Body, b.Body)
	}
	id := strings.Split(a.Body.String(), `"`)[3]
	if len(id) != minIDLen {
		t.Fatalf("id length %d", len(id))
	}
	got := do(h, "GET", "/api/share/"+id, "")
	if got.Code != 200 || got.Body.String() != "abc_DEF-123" {
		t.Fatalf("fetch: %d %q", got.Code, got.Body)
	}
	if do(h, "GET", "/api/share/nope12345", "").Code != 404 {
		t.Fatal("unknown id should 404")
	}
}

func TestRejects(t *testing.T) {
	s, _ := testServer(t)
	h := s.routes()
	if c := do(h, "POST", "/api/share", "has space").Code; c != 400 {
		t.Fatalf("bad charset: %d", c)
	}
	if c := do(h, "POST", "/api/share", "").Code; c != 400 {
		t.Fatalf("empty: %d", c)
	}
	if c := do(h, "POST", "/api/share", strings.Repeat("a", maxTokenBytes+1)).Code; c != 413 {
		t.Fatalf("too large: %d", c)
	}
	if c := do(h, "GET", "/api/share/../x", "").Code; c == 200 {
		t.Fatalf("traversal: %d", c)
	}
}

func TestRateLimit(t *testing.T) {
	s, _ := testServer(t)
	h := s.routes()
	for i := 0; i < 3; i++ {
		if c := do(h, "POST", "/api/share", "tok"+string(rune('a'+i))).Code; c != 200 {
			t.Fatalf("share %d: %d", i, c)
		}
	}
	if c := do(h, "POST", "/api/share", "tokz").Code; c != 429 {
		t.Fatalf("expected 429, got %d", c)
	}
}

func TestExpiry(t *testing.T) {
	_, st := testServer(t)
	base := time.Unix(1_700_000_000, 0)
	st.now = func() time.Time { return base }
	old, _ := st.Put("old")
	keep, _ := st.Put("keep")
	// "keep" is read 300 days in; "old" is untouched.
	st.now = func() time.Time { return base.Add(300 * 24 * time.Hour) }
	if _, ok, _ := st.Get(keep); !ok {
		t.Fatal("keep missing")
	}
	st.now = func() time.Time { return base.Add(400 * 24 * time.Hour) }
	if n, err := st.Purge(shareTTL); err != nil || n != 1 {
		t.Fatalf("purge n=%d err=%v", n, err)
	}
	if _, ok, _ := st.Get(old); ok {
		t.Fatal("old should be gone")
	}
	if _, ok, _ := st.Get(keep); !ok {
		t.Fatal("keep should survive (was read within a year)")
	}
}

func TestIDCollisionExtends(t *testing.T) {
	_, st := testServer(t)
	// Plant a different token under the 8-char ID of "x".
	st.db.Exec(`INSERT INTO shares VALUES (?, 'other', 0, 0)`, idFor("x", minIDLen))
	id, err := st.Put("x")
	if err != nil || len(id) != minIDLen+1 {
		t.Fatalf("id=%q err=%v", id, err)
	}
	if tok, ok, _ := st.Get(id); !ok || tok != "x" {
		t.Fatalf("get: %q %v", tok, ok)
	}
}

func TestBackup(t *testing.T) {
	_, st := testServer(t)
	id, _ := st.Put("snapshot-me")
	dst := filepath.Join(t.TempDir(), "b.db")
	if err := st.Backup(dst); err != nil {
		t.Fatal(err)
	}
	cp, err := OpenStore(dst, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer cp.Close()
	if tok, ok, _ := cp.Get(id); !ok || tok != "snapshot-me" {
		t.Fatalf("backup missing row: %q %v", tok, ok)
	}
	if err := st.Backup(dst); err == nil {
		t.Fatal("backup must refuse to overwrite an existing file")
	}
}

func doFrom(h http.Handler, method, path, body string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// Spoofing X-Forwarded-For must not buy extra quota when Cloudflare names the
// real client.
func TestRateLimitKeysOnCFConnectingIP(t *testing.T) {
	s, _ := testServer(t)
	h := s.routes()
	for i := 0; i < 3; i++ {
		hdr := map[string]string{"CF-Connecting-IP": "9.9.9.9", "X-Forwarded-For": "10.0.0." + string(rune('1'+i))}
		if c := doFrom(h, "POST", "/api/share", "tok"+string(rune('a'+i)), hdr).Code; c != 200 {
			t.Fatalf("share %d: %d", i, c)
		}
	}
	spoof := map[string]string{"CF-Connecting-IP": "9.9.9.9", "X-Forwarded-For": "10.9.9.9"}
	if c := doFrom(h, "POST", "/api/share", "tokz", spoof).Code; c != 429 {
		t.Fatalf("spoofed XFF got through: %d", c)
	}
	other := map[string]string{"CF-Connecting-IP": "8.8.8.8"}
	if c := doFrom(h, "POST", "/api/share", "tokz", other).Code; c != 200 {
		t.Fatalf("a different client should have its own quota: %d", c)
	}
}

func TestGlobalLimit(t *testing.T) {
	s, _ := testServer(t)
	s.global = newLimiter(2, time.Hour)
	h := s.routes()
	for i, ip := range []string{"1.1.1.1", "2.2.2.2"} {
		if c := doFrom(h, "POST", "/api/share", "g"+string(rune('a'+i)), map[string]string{"CF-Connecting-IP": ip}).Code; c != 200 {
			t.Fatalf("share %d: %d", i, c)
		}
	}
	if c := doFrom(h, "POST", "/api/share", "gz", map[string]string{"CF-Connecting-IP": "3.3.3.3"}).Code; c != 429 {
		t.Fatalf("global limit not enforced: %d", c)
	}
}

func TestStorageCap(t *testing.T) {
	s, st := testServer(t)
	s.limit = newLimiter(100, time.Hour)
	st.maxBytes = 10
	h := s.routes()
	if c := do(h, "POST", "/api/share", "123456").Code; c != 200 {
		t.Fatalf("first: %d", c)
	}
	if c := do(h, "POST", "/api/share", "abcdef").Code; c != http.StatusInsufficientStorage {
		t.Fatalf("over cap: %d", c)
	}
	// Re-sharing existing content costs nothing, so it still works when full.
	if c := do(h, "POST", "/api/share", "123456").Code; c != 200 {
		t.Fatalf("dedupe at cap: %d", c)
	}
	// Purging frees room.
	st.now = func() time.Time { return time.Now().Add(2 * shareTTL) }
	if n, _ := st.Purge(shareTTL); n != 1 {
		t.Fatalf("purge %d", n)
	}
	st.now = time.Now
	if c := do(h, "POST", "/api/share", "abcdef").Code; c != 200 {
		t.Fatalf("after purge: %d", c)
	}
}

func TestMissLimit(t *testing.T) {
	s, _ := testServer(t)
	h := s.routes()
	for i := 0; i < 3; i++ {
		if c := do(h, "GET", "/api/share/AAAAAAA"+string(rune('a'+i)), "").Code; c != 404 {
			t.Fatalf("miss %d: %d", i, c)
		}
	}
	if c := do(h, "GET", "/api/share/AAAAAAAz", "").Code; c != 429 {
		t.Fatalf("guessing not slowed: %d", c)
	}
}
