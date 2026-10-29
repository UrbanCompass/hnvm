package resolve

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/UrbanCompass/hnvm/internal/output"
)

// registry serves a fake package "tool" with three versions and a latest dist-tag. Like the real
// registry it answers /tool/<tag-or-version> but not /tool/<range>.
func registry(t *testing.T, hits *atomic.Int32) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch r.URL.Path {
		case "/tool":
			w.Write([]byte(`{"dist-tags":{"latest":"2.1.0","next":"3.0.0"},"versions":{"1.2.3":{},"1.9.0":{},"2.1.0":{},"3.0.0":{},"4.0.0-rc.1":{}}}`))
		case "/tool/latest":
			w.Write([]byte(`{"version":"2.1.0"}`))
		case "/tool/rc":
			w.Write([]byte(`{"version":"4.0.0-rc.1"}`))
		default:
			http.Error(w, "nope", http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newResolver(t *testing.T, srv *httptest.Server) *Resolver {
	return &Resolver{Root: t.TempDir(), CacheTTL: time.Minute, Registry: srv.URL, Out: output.Discard(), Client: srv.Client()}
}

func TestResolve(t *testing.T) {
	var hits atomic.Int32
	srv := registry(t, &hits)
	for _, tc := range []struct {
		spec, want, how string
	}{
		{"1.0.0", "1.0.0", "exact"},
		{"latest", "2.1.0", "registry"},
		{"^1.0.0", "1.9.0", "range"},
		{">=1 <2.1", "1.9.0", "range"},
		{"next", "3.0.0", "dist-tag"},
		{"4.0.0-rc.1", "4.0.0-rc.1", "exact"},
		{"rc", "4.0.0-rc.1", "registry"},
		{">=4.0.0-rc.0", "4.0.0-rc.1", "range"},
	} {
		t.Run(tc.spec, func(t *testing.T) {
			r := newResolver(t, srv)
			got, err := r.Resolve(context.Background(), "tool", tc.spec)
			if err != nil || got.Version != tc.want || got.How != tc.how {
				t.Fatalf("Resolve(%q) = %+v, %v; want %s by %s", tc.spec, got, err, tc.want, tc.how)
			}
		})
	}
}

func TestResolveFailures(t *testing.T) {
	var hits atomic.Int32
	srv := registry(t, &hits)
	for _, spec := range []string{"^9.0.0", "nonexistent-tag"} {
		if _, err := newResolver(t, srv).Resolve(context.Background(), "tool", spec); err != ErrReported {
			t.Errorf("Resolve(%q): err = %v, want ErrReported", spec, err)
		}
	}
}

func TestResolvePrefersInstalled(t *testing.T) {
	var hits atomic.Int32
	srv := registry(t, &hits)
	r := newResolver(t, srv)
	os.MkdirAll(filepath.Join(r.Root, "tool", "1.2.3"), 0o755)

	got, err := r.Resolve(context.Background(), "tool", "^1.0.0")
	if err != nil || got.Version != "1.2.3" || got.How != "local" {
		t.Fatalf("got %+v, %v; want the installed 1.2.3", got, err)
	}
}

func TestResolveCache(t *testing.T) {
	var hits atomic.Int32
	srv := registry(t, &hits)
	r := newResolver(t, srv)
	now := time.Now()
	r.Now = func() time.Time { return now }

	if _, err := r.Resolve(context.Background(), "tool", "^1.0.0"); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(r.Root, ".tmp", "tool", "_1_0_0")); string(data) != "1.9.0\n" {
		t.Fatalf("cache file = %q (the layout is shared with bash HNVM)", data)
	}
	before := hits.Load()
	if got, _ := r.Resolve(context.Background(), "tool", "^1.0.0"); got.How != "cache" {
		t.Fatalf("second resolve was %q, want cache", got.How)
	}
	if hits.Load() != before {
		t.Fatal("a cached resolve hit the registry")
	}

	now = now.Add(2 * time.Minute)
	if got, _ := r.Resolve(context.Background(), "tool", "^1.0.0"); got.How == "cache" {
		t.Fatal("an expired cache entry was used")
	}
}
