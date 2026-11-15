package pagetree

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSmokeRoundTrip — full integration: create page → verify disk → render → menu → delete.
func TestSmokeRoundTrip(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"pages", "news", "media"} {
		_ = os.MkdirAll(filepath.Join(root, d), 0o755)
	}
	dataDir := t.TempDir()

	// Simple renderer stub
	r := &stubRenderer{}
	eng, err := New(Config{
		Root:     root,
		DataDir:  dataDir,
		SiteURL:  "https://test.ru",
		Renderer: r,
	})
	if err != nil {
		t.Fatal(err)
	}

	// 1. Create page
	meta := &PageMeta{
		Title: "О компании",
		Menu:  true,
		Order: 5,
		SEO: SEOFields{
			MetaDescription: "Описание для поисковиков",
			OgImage:         "/media/og-about.jpg",
		},
	}
	md := []byte("# О компании\n\nМы — лучшая компания.")
	err = eng.store.WritePage("pages/about.md", md, meta)
	if err != nil {
		t.Fatal("WritePage:", err)
	}

	// 2. Verify .md on disk
	mdData, err := os.ReadFile(filepath.Join(root, "pages", "about.md"))
	if err != nil {
		t.Fatal("md not on disk:", err)
	}
	if !strings.Contains(string(mdData), "О компании") {
		t.Error("md content mismatch")
	}

	// 3. Verify .json on disk
	jsonData, err := os.ReadFile(filepath.Join(root, "pages", "about.json"))
	if err != nil {
		t.Fatal("json not on disk:", err)
	}
	var diskMeta PageMeta
	if err := json.Unmarshal(jsonData, &diskMeta); err != nil {
		t.Fatal("parse json:", err)
	}
	if diskMeta.Title != "О компании" {
		t.Errorf("Title on disk = %q", diskMeta.Title)
	}

	// 4. ReadPage → render
	p, err := eng.store.ReadPage("pages/about.md")
	if err != nil {
		t.Fatal("ReadPage:", err)
	}
	html, err := eng.renderPage(p)
	if err != nil {
		t.Fatal("renderPage:", err)
	}
	htmlStr := string(html)
	if !strings.Contains(htmlStr, "<title>О компании</title>") {
		t.Error("render: missing <title>")
	}
	if !strings.Contains(htmlStr, `name="description"`) {
		t.Error("render: missing meta description")
	}
	if !strings.Contains(htmlStr, "og:image") {
		t.Error("render: missing OG image")
	}

	// 5. Menu
	menu := eng.buildMenu()
	found := false
	for _, item := range menu.Pages {
		if item.Title == "О компании" {
			found = true
			if item.URL != "/pages/about" {
				t.Errorf("menu URL = %q", item.URL)
			}
		}
	}
	if !found {
		t.Error("page not in menu")
	}

	// 6. Save media
	url, err := eng.store.SaveMedia("media", "test.jpg", []byte("fake"))
	if err != nil {
		t.Fatal("SaveMedia:", err)
	}
	if url != "/media/test.jpg" {
		t.Errorf("media URL = %q", url)
	}

	// 7. Delete page (triple)
	err = eng.store.DeletePage("pages/about.md")
	if err != nil {
		t.Fatal("DeletePage:", err)
	}
	// Verify all gone
	for _, f := range []string{"pages/about.md", "pages/about.json"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(f))); !os.IsNotExist(err) {
			t.Errorf("%s should be deleted", f)
		}
	}

	// 8. Keys
	ks, err := NewKeyStore(dataDir, nil, nil)
	if err != nil {
		t.Fatal("NewKeyStore:", err)
	}
	key, raw, err := ks.GenerateKey("smoke-agent", KeyPolicy{Pages: "rw", News: "rw", Media: "rw"})
	if err != nil {
		t.Fatal("GenerateKey:", err)
	}
	if !strings.HasPrefix(raw, "ptk_live_") {
		t.Error("key format wrong")
	}
	// Authenticate
	authKey, err := ks.Authenticate(raw)
	if err != nil {
		t.Fatal("Authenticate:", err)
	}
	if authKey.ID != key.ID {
		t.Error("key ID mismatch")
	}
	// Check policy
	if !CheckPolicy(key.Policy, "pages", true) {
		t.Error("policy should allow pages write")
	}
}

// TestSectionListingAndBreadcrumbs: /pages/<dir> без index.md даёт листинг секции (не 404),
// крошки берут название из section.json правильного каталога.
func TestSectionListingAndBreadcrumbs(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"pages", "news", "media"} {
		_ = os.MkdirAll(filepath.Join(root, d), 0o755)
	}
	eng, err := New(Config{
		Root:     root,
		DataDir:  t.TempDir(),
		SiteURL:  "https://test.ru",
		Renderer: &stubRenderer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.store.WritePage("pages/docs/guide.md", []byte("# Guide\n"), &PageMeta{Title: "Руководство"}); err != nil {
		t.Fatal(err)
	}
	if err := eng.store.WriteSection("pages/docs", &SectionMeta{Title: "Документация"}); err != nil {
		t.Fatal(err)
	}

	h := eng.PageHandler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/pages/docs", nil))
	if rec.Code != 200 {
		t.Fatalf("GET /pages/docs = %d, want 200 (section listing)", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=60" {
		t.Errorf("public listing Cache-Control = %q, want public, max-age=60", cc)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Документация") || !strings.Contains(body, "/pages/docs/guide") {
		t.Errorf("listing should show section title and page link, got: %.200s", body)
	}
	if !strings.Contains(body, `class="breadcrumbs"`) || !strings.Contains(body, `<a href="/pages">`) || !strings.Contains(body, `aria-current="page"`) {
		t.Errorf("listing should show breadcrumbs (Главная / pages / Документация), got: %.300s", body)
	}

	p, err := eng.store.ReadPage("pages/docs/guide.md")
	if err != nil {
		t.Fatal(err)
	}
	crumbs := eng.buildBreadcrumbs(p)
	if len(crumbs) != 4 {
		t.Fatalf("crumbs = %+v, want 4 (Главная / pages / Документация / Руководство)", crumbs)
	}
	if crumbs[2].Name != "Документация" || crumbs[2].URL != "/pages/docs" {
		t.Errorf("section crumb = %+v, want {Документация /pages/docs}", crumbs[2])
	}

	rec404 := httptest.NewRecorder()
	h.ServeHTTP(rec404, httptest.NewRequest("GET", "/pages/nope", nil))
	if rec404.Code != 404 {
		t.Errorf("GET /pages/nope = %d, want 404", rec404.Code)
	}

	// PUT /section — сохранение секции через admin API (не через PUT /page)
	apiH := eng.ContentAPI()
	secBody := strings.NewReader(`{"path":"pages/docs","json":{"title":"Документация v2","order":5,"menu":true}}`)
	req := httptest.NewRequest("PUT", "/section", secBody)
	recSec := httptest.NewRecorder()
	apiH.ServeHTTP(recSec, req)
	if recSec.Code != 200 {
		t.Fatalf("PUT /section = %d, want 200: %s", recSec.Code, recSec.Body.String())
	}
	if cc := recSec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("admin API Cache-Control = %q, want no-store", cc)
	}
	sec, _ := eng.store.ReadSection("pages/docs")
	if sec.Title != "Документация v2" || sec.Order != 5 || !sec.Menu {
		t.Errorf("section = %+v, want {Документация v2 5 true}", sec)
	}
}

type stubRenderer struct{}

func (r *stubRenderer) Render(md []byte) ([]byte, error) {
	// Simple: wrap in <p> tags (enough for smoke test)
	text := strings.ReplaceAll(string(md), "\n\n", "</p><p>")
	return []byte("<p>" + text + "</p>"), nil
}
