package pagetree

import (
	"strings"
	"testing"
)

func TestSanitizeHTMLBasic(t *testing.T) {
	tests := []struct{ input, want string }{
		{`<p>Hello</p>`, `<p>Hello</p>`},
		{`<script>alert(1)</script>`, ``},
		{`<p onclick="alert(1)">text</p>`, `<p>text</p>`},
		{`<a href="javascript:alert(1)">bad</a>`, `<a>bad</a>`},
		{`<div onmouseover="alert(1)">x</div>`, `<div>x</div>`},
		{`<iframe src="...">`, ``},
		{`<img src="//evil.com/x.jpg">`, `<img>`},
	}
	for _, tt := range tests {
		got := sanitizeHTML(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeHTML(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
	// Order-independent checks
	got := sanitizeHTML(`<img src="/media/photo.jpg" alt="test">`)
	if !strings.Contains(got, `src="/media/photo.jpg"`) || !strings.Contains(got, `alt="test"`) {
		t.Errorf("img attrs lost: %q", got)
	}
	got2 := sanitizeHTML(`<img src="data:text/html,<script>alert(1)</script>">`)
	if strings.Contains(got2, "script") || strings.Contains(got2, "alert") {
		t.Errorf("script leaked through data: URL: %q", got2)
	}
}

func TestSanitizeHTMLNested(t *testing.T) {
	input := `<p>text <strong>bold</strong> <em>italic</em></p>`
	want := `<p>text <strong>bold</strong> <em>italic</em></p>`
	got := sanitizeHTML(input)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSanitizeHTMLAllowImgAttrs(t *testing.T) {
	input := `<img src="/media/x.jpg" width="600" height="400" alt="test" align="center">`
	got := sanitizeHTML(input)
	for _, attr := range []string{`width="600"`, `height="400"`, `alt="test"`, `align="center"`} {
		if !strings.Contains(got, attr) {
			t.Errorf("missing attr %s in %q", attr, got)
		}
	}
}

func TestSanitizeHTMLRejectsStyle(t *testing.T) {
	input := `<img src="/media/x.jpg" style="position:fixed">`
	got := sanitizeHTML(input)
	if strings.Contains(got, "style") {
		t.Error("style attribute should be removed")
	}
}

func TestIsAllowedURL(t *testing.T) {
	allowed := map[string]bool{"https": true, "http": true, "/": true}
	tests := []struct {
		url   string
		allow bool
	}{
		{"/media/photo.jpg", true},
		{"https://example.com", true},
		{"http://example.com", true},
		{"relative/path.jpg", true},
		{"javascript:alert(1)", false},
		{"data:text/html,...", false},
		{"vbscript:...", false},
		{"//evil.com/x", false},
		{"", false},
	}
	for _, tt := range tests {
		got := isAllowedURL(tt.url, allowed)
		if got != tt.allow {
			t.Errorf("isAllowedURL(%q) = %v, want %v", tt.url, got, tt.allow)
		}
	}
}

func TestBuildBreadcrumbs(t *testing.T) {
	s := newTestStore(t)
	e := &Engine{store: s, cfg: Config{SiteURL: "https://site.ru"}}

	p := &Page{
		Path: "pages/docs/install.md",
		URL:  "/pages/docs/install",
		Meta: PageMeta{Title: "Установка"},
	}
	crumbs := e.buildBreadcrumbs(p)
	if len(crumbs) < 3 {
		t.Fatalf("expected >=3 breadcrumbs, got %d: %+v", len(crumbs), crumbs)
	}
	if crumbs[0].Name != "Главная" {
		t.Errorf("first crumb = %q, want Главная", crumbs[0].Name)
	}
	last := crumbs[len(crumbs)-1]
	if last.Name != "Установка" {
		t.Errorf("last crumb = %q, want Установка", last.Name)
	}
	if last.URL != "" {
		t.Errorf("last crumb URL = %q, want empty (current page)", last.URL)
	}
}

func TestBuildJSONLD(t *testing.T) {
	s := newTestStore(t)
	e := &Engine{store: s, cfg: Config{SiteURL: "https://site.ru"}}

	p := &Page{
		Path: "pages/about.md",
		URL:  "/pages/about",
		Meta: PageMeta{Title: "About", Date: "2026-11-01", Modified: "2026-11-14"},
	}
	crumbs := []Breadcrumb{{Name: "Главная", URL: "/"}, {Name: "About", URL: ""}}
	jsonld := e.buildJSONLD(p, "https://site.ru/pages/about", "https://site.ru/media/x.jpg", crumbs)

	if !strings.Contains(jsonld, "Article") {
		t.Error("JSON-LD should contain Article type")
	}
	if !strings.Contains(jsonld, "BreadcrumbList") {
		t.Error("JSON-LD should contain BreadcrumbList")
	}
	if !strings.Contains(jsonld, "2026-11-01") {
		t.Error("JSON-LD should contain datePublished")
	}
	if !strings.Contains(jsonld, "2026-11-14") {
		t.Error("JSON-LD should contain dateModified")
	}
}
