package pagetree

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// Контракт монтирования: ContentAPI регистрирует «голые» пути (/list, /page),
// поэтому внешний префикс требует http.StripPrefix. Без него — 404
// (баг mcp-is cmd/server/main.go, 2026-11-15).
func TestContentAPIStripPrefixContract(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"pages", "news", "media"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	eng, err := New(Config{Root: root, DataDir: t.TempDir(), Renderer: &stubRenderer{}})
	if err != nil {
		t.Fatal(err)
	}

	muxOK := http.NewServeMux()
	muxOK.Handle("/api/admin/content/", http.StripPrefix("/api/admin/content", eng.ContentAPI()))

	rec := httptest.NewRecorder()
	muxOK.ServeHTTP(rec, httptest.NewRequest("GET", "/api/admin/content/list?root=pages", nil))
	if rec.Code != 200 {
		t.Errorf("GET /api/admin/content/list with StripPrefix = %d, want 200", rec.Code)
	}

	rec2 := httptest.NewRecorder()
	muxOK.ServeHTTP(rec2, httptest.NewRequest("PUT", "/api/admin/content/page", nil))
	if rec2.Code == 404 {
		t.Errorf("PUT /api/admin/content/page with StripPrefix = 404, want non-404 (route must match)")
	}
}
