package pagetree

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{"pages", "news", "media"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	s, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestPathToURL(t *testing.T) {
	s := newTestStore(t)
	tests := []struct{ path, want string }{
		{"pages/about.md", "/pages/about"},
		{"pages/docs/install.md", "/pages/docs/install"},
		{"pages/docs/index.md", "/pages/docs/"},
		{"pages/index.md", "/pages/"},
		{"news/releases/v1.2.md", "/news/releases/v1.2"},
	}
	for _, tt := range tests {
		got := s.pathToURL(tt.path)
		if got != tt.want {
			t.Errorf("pathToURL(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestResolveURL(t *testing.T) {
	s := newTestStore(t)
	// Create test files
	files := map[string]string{
		"pages/about.md":        "# About",
		"pages/docs/install.md": "# Install",
		"pages/docs/index.md":   "# Docs Index",
	}
	for p, content := range files {
		abs := filepath.Join(s.root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct{ url, want string }{
		{"/pages/about", "pages/about.md"},
		{"/pages/about/", "pages/about.md"},
		{"/pages/docs/install", "pages/docs/install.md"},
		{"/pages/docs", "pages/docs/index.md"},
		{"/pages/docs/", "pages/docs/index.md"},
	}
	for _, tt := range tests {
		got, err := s.resolveURL(tt.url)
		if err != nil {
			t.Errorf("resolveURL(%q) error: %v", tt.url, err)
			continue
		}
		if got != tt.want {
			t.Errorf("resolveURL(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

func TestResolveURLNotFound(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.resolveURL("/pages/nonexistent"); err == nil {
		t.Error("expected error for nonexistent page")
	}
}

func TestGuardTraversal(t *testing.T) {
	s := newTestStore(t)
	tests := []string{
		"../etc/passwd",
		"..",
		"./..",
		"pages/../../etc",
	}
	for _, p := range tests {
		if _, err := s.guard(p); err == nil {
			t.Errorf("guard(%q) should reject", p)
		}
	}
}

func TestGuardValid(t *testing.T) {
	s := newTestStore(t)
	tests := []string{
		"pages/about.md",
		"pages/docs/install.md",
		"media/photo.jpg",
		"news/releases/v1.2.md",
	}
	for _, p := range tests {
		if _, err := s.guard(p); err != nil {
			t.Errorf("guard(%q) error: %v", p, err)
		}
	}
}

func TestWriteReadPage(t *testing.T) {
	s := newTestStore(t)
	meta := &PageMeta{Title: "Test Page", Menu: true, Order: 5}
	md := []byte("# Test Page\n\nHello world.")
	err := s.WritePage("pages/about.md", md, meta)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.ReadPage("pages/about.md")
	if err != nil {
		t.Fatal(err)
	}
	if p.Meta.Title != "Test Page" {
		t.Errorf("Title = %q, want %q", p.Meta.Title, "Test Page")
	}
	if !p.Meta.Menu {
		t.Error("Menu should be true")
	}
	if p.Meta.Order != 5 {
		t.Errorf("Order = %d, want 5", p.Meta.Order)
	}
	if p.URL != "/pages/about" {
		t.Errorf("URL = %q, want %q", p.URL, "/pages/about")
	}
}

func TestReadPageDefaults(t *testing.T) {
	s := newTestStore(t)
	// Write .md without .json
	abs := filepath.Join(s.root, "pages", "test.md")
	if err := os.WriteFile(abs, []byte("# My Title\n\ntext"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := s.ReadPage("pages/test.md")
	if err != nil {
		t.Fatal(err)
	}
	if p.Meta.Title != "My Title" {
		t.Errorf("Title = %q, want %q (from H1)", p.Meta.Title, "My Title")
	}
	if p.Meta.Draft {
		t.Error("Draft should default to false")
	}
}

func TestDeletePageTriple(t *testing.T) {
	s := newTestStore(t)
	meta := &PageMeta{Title: "Del"}
	if err := s.WritePage("pages/del.md", []byte("# Del"), meta); err != nil {
		t.Fatal(err)
	}
	// Create _media dir
	mediaDir := filepath.Join(s.root, "pages", "del_media")
	if err := os.MkdirAll(mediaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mediaDir, "img.jpg"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.DeletePage("pages/del.md"); err != nil {
		t.Fatal(err)
	}
	// Verify all three are gone
	for _, p := range []string{"pages/del.md", "pages/del.json", "pages/del_media"} {
		abs := filepath.Join(s.root, filepath.FromSlash(p))
		if _, err := os.Stat(abs); !os.IsNotExist(err) {
			t.Errorf("%s should be deleted", p)
		}
	}
}

func TestRenamePageTriple(t *testing.T) {
	s := newTestStore(t)
	meta := &PageMeta{Title: "Ren"}
	if err := s.WritePage("pages/old.md", []byte("# Ren"), meta); err != nil {
		t.Fatal(err)
	}
	mediaDir := filepath.Join(s.root, "pages", "old_media")
	if err := os.MkdirAll(mediaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mediaDir, "img.jpg"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.RenamePage("pages/old.md", "pages/new.md"); err != nil {
		t.Fatal(err)
	}
	// Verify new files exist
	for _, p := range []string{"pages/new.md", "pages/new.json", "pages/new_media"} {
		abs := filepath.Join(s.root, filepath.FromSlash(p))
		if _, err := os.Stat(abs); err != nil {
			t.Errorf("%s should exist after rename", p)
		}
	}
	// Verify old files gone
	for _, p := range []string{"pages/old.md", "pages/old.json", "pages/old_media"} {
		abs := filepath.Join(s.root, filepath.FromSlash(p))
		if _, err := os.Stat(abs); !os.IsNotExist(err) {
			t.Errorf("%s should be gone after rename", p)
		}
	}
}

func TestOrphanJSON(t *testing.T) {
	s := newTestStore(t)
	// Write .json without .md
	abs := filepath.Join(s.root, "pages", "orphan.json")
	if err := os.WriteFile(abs, []byte(`{"title":"Orphan"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	tree, err := s.ListTree("pages")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range tree {
		if e.Type == "orphan" {
			found = true
			if e.HasMD {
				t.Error("orphan should not have HasMD")
			}
			if !e.HasJSON {
				t.Error("orphan should have HasJSON")
			}
		}
	}
	if !found {
		t.Error("orphan .json not found in tree")
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct{ input, want string }{
		{"photo.jpg", "photo.jpg"},
		{"Photo.JPG", "photo.jpg"},
		{"my-file_2.png", "my-file_2.png"},
		{"../../../etc/passwd", "passwd"},
		{"..", ""},
		{"x.html", "x.html"}, // extension check is at upload level
		{"", ""},
	}
	for _, tt := range tests {
		got := sanitizeFilename(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSectionMeta(t *testing.T) {
	s := newTestStore(t)
	sec := &SectionMeta{Title: "Документация", Order: 10, Menu: true}
	if err := s.WriteSection("pages/docs", sec); err != nil {
		t.Fatal(err)
	}
	got, err := s.ReadSection("pages/docs")
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Документация" {
		t.Errorf("Title = %q, want %q", got.Title, "Документация")
	}
	if got.Order != 10 {
		t.Errorf("Order = %d, want 10", got.Order)
	}
}

func TestSaveMedia(t *testing.T) {
	s := newTestStore(t)
	url, err := s.SaveMedia("media", "test-image.jpg", []byte("fakeimage"))
	if err != nil {
		t.Fatal(err)
	}
	if url != "/media/test-image.jpg" {
		t.Errorf("URL = %q, want %q", url, "/media/test-image.jpg")
	}
	// Verify file exists
	if _, err := os.Stat(filepath.Join(s.root, "media", "test-image.jpg")); err != nil {
		t.Fatal("media file not saved")
	}
}
