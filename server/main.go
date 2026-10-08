// tailground-api: the tiny backend behind short share links. It stores the
// client's compressed scenario token in SQLite under a content-derived ID and
// serves it back. It never reads the scenario itself, only checks the token is
// a well-formed, size-bounded deflate stream (see scenario.go).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	maxTokenBytes = 64 << 10
	shareTTL      = 365 * 24 * time.Hour
)

type server struct {
	store  *Store
	limit  *limiter // creates, per client
	global *limiter // creates, all clients together: bounds a distributed flood
	misses *limiter // lookups of unknown IDs, per client: slows ID guessing

	// check vets a token's content before it is stored; nil accepts any
	// well-formed token (tests).
	check func(token string) error
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/share", s.create)
	mux.HandleFunc("GET /api/share/{id}", s.fetch)
	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "ok\n") })
	return mux
}

func (s *server) create(w http.ResponseWriter, r *http.Request) {
	if !s.limit.allow(clientIP(r)) || !s.global.allow("*") {
		http.Error(w, "too many shares, slow down", http.StatusTooManyRequests)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxTokenBytes))
	if err != nil {
		http.Error(w, "token too large", http.StatusRequestEntityTooLarge)
		return
	}
	token := strings.TrimSpace(string(body))
	if !validToken(token) {
		http.Error(w, "bad token", http.StatusBadRequest)
		return
	}
	if s.check != nil && s.check(token) != nil {
		http.Error(w, "bad token", http.StatusBadRequest)
		return
	}
	id, err := s.store.Put(token)
	if errors.Is(err, ErrFull) {
		http.Error(w, "share storage is full, try the long link", http.StatusInsufficientStorage)
		return
	}
	if err != nil {
		log.Printf("put: %v", err)
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": id})
}

func (s *server) fetch(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID(id) {
		http.NotFound(w, r)
		return
	}
	ip := clientIP(r)
	if !s.misses.has(ip) {
		http.Error(w, "too many unknown links, slow down", http.StatusTooManyRequests)
		return
	}
	token, ok, err := s.store.Get(id)
	if err != nil {
		log.Printf("get: %v", err)
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	if !ok {
		s.misses.allow(ip) // only unknown IDs are charged
		w.Header().Set("Cache-Control", "no-store")
		http.NotFound(w, r)
		return
	}
	// Content-addressed, so a given ID never changes.
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	io.WriteString(w, token)
}

// validToken accepts only the base64url alphabet the client emits.
func validToken(t string) bool {
	if t == "" {
		return false
	}
	for i := 0; i < len(t); i++ {
		c := t[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func validID(id string) bool {
	if len(id) < minIDLen || len(id) > maxIDLen {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

// clientIP is the real client address. The site sits behind Cloudflare, which
// sets CF-Connecting-IP (overwriting anything the client sent); failing that
// the rightmost X-Forwarded-For entry (what the gateway appended), else the
// socket peer. A request that reaches the origin directly can forge the first
// two, so the per-client limits are best-effort — the global limit and the
// storage cap are what bound a determined flood.
func clientIP(r *http.Request) string {
	if ip := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); ip != "" {
		return ip
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[len(parts)-1])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// limiter is a per-key token bucket for share creation.
type limiter struct {
	mu      sync.Mutex
	burst   float64
	refill  float64 // tokens per second
	buckets map[string]*bucket
	now     func() time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

func newLimiter(burst int, every time.Duration) *limiter {
	return &limiter{burst: float64(burst), refill: 1 / every.Seconds(), buckets: map[string]*bucket{}, now: time.Now}
}

// refilled returns key's bucket brought up to date. Caller holds l.mu.
func (l *limiter) refilled(key string) *bucket {
	now := l.now()
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	b.tokens += now.Sub(b.last).Seconds() * l.refill
	if b.tokens > l.burst {
		b.tokens = l.burst
	}
	b.last = now
	return b
}

// allow takes one token, reporting whether there was one.
func (l *limiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	b := l.refilled(key)
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// has reports whether a token is available without taking it.
func (l *limiter) has(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.refilled(key).tokens >= 1
}

// sweep drops idle full buckets so the map can't grow without bound.
func (l *limiter) sweep() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for k, b := range l.buckets {
		if b.tokens+l.now().Sub(b.last).Seconds()*l.refill >= l.burst {
			delete(l.buckets, k)
		}
	}
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8081", "listen address")
	db := flag.String("db", "/data/shares.db", "SQLite database path")
	maxBytes := flag.Int64("max-bytes", 256<<20, "refuse new shares once stored tokens total this many bytes (0 = unlimited)")
	backup := flag.String("backup", "", "write a consistent snapshot of the database to this path and exit")
	flag.Parse()

	store, err := OpenStore(*db, *maxBytes)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer store.Close()

	if *backup != "" {
		if err := store.Backup(*backup); err != nil {
			log.Fatalf("backup: %v", err)
		}
		log.Printf("backup written to %s", *backup)
		return
	}

	s := &server{
		store:  store,
		limit:  newLimiter(10, time.Minute),
		global: newLimiter(60, 2*time.Second), // sustained 30/min across everyone
		misses: newLimiter(30, 2*time.Second),
		check:  looksLikeScenario,
	}
	srv := &http.Server{
		Addr:              *addr,
		Handler:           s.routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		for {
			if n, err := store.Purge(shareTTL); err != nil {
				log.Printf("purge: %v", err)
			} else if n > 0 {
				log.Printf("purged %d expired shares", n)
			}
			s.limit.sweep()
			s.misses.sweep()
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()

	log.Printf("listening on %s, db %s", *addr, *db)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
