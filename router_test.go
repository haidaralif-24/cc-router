package main

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func newTestRouter(baseURL string, keys ...string) (*Router, []*KeyState) {
	prov := Provider{Name: "test", BaseURL: baseURL, AuthStyle: "x-api-key", Keys: keys}
	cfg := &Config{DefaultCooldownSeconds: 60, Providers: []Provider{prov}}
	r := &Router{
		pool:   NewPool(cfg),
		token:  "t",
		client: &http.Client{},
	}
	return r, r.pool.keys
}

func TestNormalizeListenAddr(t *testing.T) {
	ok := map[string]string{
		"":               "127.0.0.1:8787",
		":8787":          "127.0.0.1:8787",
		"127.0.0.1:9000": "127.0.0.1:9000",
		"localhost:9000": "localhost:9000",
		"[::1]:9000":     "[::1]:9000",
	}
	for in, want := range ok {
		got, err := normalizeListenAddr(in)
		if err != nil || got != want {
			t.Errorf("normalizeListenAddr(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"0.0.0.0:8787", "192.168.1.4:8787", "8787"} {
		if _, err := normalizeListenAddr(in); err == nil {
			t.Errorf("normalizeListenAddr(%q) should reject non-local address", in)
		}
	}
}

func Test400ForwardedImmediately(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"error":"bad payload"}`)
	}))
	defer srv.Close()

	r, keys := newTestRouter(srv.URL, "key-1", "key-2")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/messages", nil)

	if stop := r.tryUpstream(rec, req, []byte(`{}`), keys[0], "/v1/messages"); !stop {
		t.Fatal("expected upstream 400 to stop failover")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "bad payload") {
		t.Fatalf("upstream body not forwarded: %q", rec.Body.String())
	}
	if hits != 1 {
		t.Fatalf("upstream hit %d times, want 1", hits)
	}
	if keys[0].Status != "active" {
		t.Fatalf("400 should not penalize key, status = %q", keys[0].Status)
	}
}

func Test429FailsOver(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "5")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	r, keys := newTestRouter(srv.URL, "key-1")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/messages", nil)

	if stop := r.tryUpstream(rec, req, []byte(`{}`), keys[0], "/v1/messages"); stop {
		t.Fatal("429 should continue to next key")
	}
	if keys[0].Status != "cooling" {
		t.Fatalf("429 key status = %q, want cooling", keys[0].Status)
	}
}

func TestShortKeyNoPanic(t *testing.T) {
	r, keys := newTestRouter("http://127.0.0.1:1", "x")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/messages", nil)
	if stop := r.tryUpstream(rec, req, []byte(`{}`), keys[0], "/v1/messages"); stop {
		t.Fatal("network error should continue to next key")
	}
}

func TestRoundRobinUnderConcurrency(t *testing.T) {
	prov := Provider{Name: "p", BaseURL: "http://x", Keys: []string{"a", "b", "c"}}
	p := NewPool(&Config{Providers: []Provider{prov}})

	const n = 30
	counts := map[string]int{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := p.Candidates()
			mu.Lock()
			counts[c[0].ID]++
			mu.Unlock()
		}()
	}
	wg.Wait()

	for _, id := range []string{"p[0]", "p[1]", "p[2]"} {
		if counts[id] == 0 {
			t.Fatalf("round-robin never selected %s: %v", id, counts)
		}
	}
}

type failWriter struct {
	header http.Header
	n      int
}

func (f *failWriter) Header() http.Header { return f.header }
func (f *failWriter) WriteHeader(int)     {}
func (f *failWriter) Write(p []byte) (int, error) {
	f.n += len(p)
	return 0, errors.New("client went away")
}

func TestStreamCopyReportsWriteError(t *testing.T) {
	w := &failWriter{header: http.Header{}}
	if err := streamCopy(w, strings.NewReader("hello")); err == nil {
		t.Fatal("expected write error to be reported")
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("upstream broke") }

func TestStreamCopyReportsReadError(t *testing.T) {
	rec := httptest.NewRecorder()
	if err := streamCopy(rec, errReader{}); err == nil {
		t.Fatal("expected read error to be reported")
	}
}
