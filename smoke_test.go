package pagetree

import (
	"encoding/json"
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

type stubRenderer struct{}

func (r *stubRenderer) Render(md []byte) ([]byte, error) {
	// Simple: wrap in <p> tags (enough for smoke test)
	text := strings.ReplaceAll(string(md), "\n\n", "</p><p>")
	return []byte("<p>" + text + "</p>"), nil
}
