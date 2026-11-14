# PageTree

Embeddable file-based markdown page/news engine for Go. Pages and news are `.md` files with sidecar `.json` metadata; URL = directory nesting. No database — files are the source of truth.

## Quick start

```go
package main

import (
    "net/http"
    "github.com/chubuk/pagetree"
)

func main() {
    engine, _ := pagetree.New(pagetree.Config{
        Root:     "./content",   // pages/, news/, media/
        DataDir:  "./data",      // keys.json, robots.txt
        SiteURL:  "https://site.ru",
        Renderer: myMDRenderer,  // any pagetree.Renderer impl
    })

    // Public routes — no auth
    http.Handle("GET /pages/", engine.PageHandler())
    http.Handle("GET /news/", engine.NewsHandler())
    http.Handle("GET /media/", engine.MediaHandler())
    http.Handle("GET /api/content/menu", engine.MenuHandler())

    // Admin — wrap with your auth middleware
    http.Handle("/api/admin/content/", myAuthMW(engine.ContentAPI()))
    http.Handle("/api/admin/content/keys/", myAuthMW(engine.KeysAPI()))
    http.Handle("/content/admin/", engine.AdminUIHandler())

    // Agent API — Bearer ptk_* keys (module-owned auth)
    http.Handle("/api/agent/v1/", engine.AgentAPI())

    http.ListenAndServe(":8080", nil)
}
```

## Content structure

```
content/
  pages/
    about.md              → /pages/about
    about.json            # sidecar metadata
    about_media/          # page-specific images
      photo.jpg
    docs/
      install.md          → /pages/docs/install
      install.json
      section.json        # folder metadata (title, order, menu)
  news/
    releases/
      v1.2.md             → /news/releases/v1.2
      v1.2.json           # {date: "2026-11-01", ...}
  media/                  # shared images
```

## Sidecar JSON schema

```json
{
  "title": "О нас",
  "menu": true,
  "order": 20,
  "date": "2026-11-01",
  "modified": "2026-11-14",
  "draft": false,
  "seo": {
    "meta_title": "О нас — компания N",
    "meta_description": "Краткое описание для поисковиков",
    "robots": "index, follow",
    "og_title": "О нас",
    "og_description": "Описание для соцсетей",
    "og_image": "/media/og-about.jpg",
    "og_type": "website",
    "twitter_card": "summary_large_image",
    "canonical": "https://site/pages/about",
    "breadcrumb_title": "О компании"
  }
}
```

## Sidecar lifecycle

- `.md` without `.json` → valid: defaults (`title` from first H1, `menu:false`, `draft:false`)
- `.json` without `.md` → orphan: visible in admin tree, not rendered
- rename/delete → atomic on triple (md + json + `_media/`)
- save → atomic `{md, json}` write via tmp+rename

## AI keys (agent access)

```go
// Create key (raw shown once)
key, raw, _ := engine.KeyStore().GenerateKey("agent", pagetree.KeyPolicy{
    Pages: "rw", News: "r", Media: "w",
})
// raw = "ptk_live_<64hex>" — save it, can't retrieve later

// Authenticate (done automatically by AgentAPI middleware)
key, _ := engine.KeyStore().Authenticate(raw)
```

Key format: `ptk_live_<64hex>`. Stored as SHA-256 hash (hash-then-store). Optional AES-256-GCM encryption of `keys.json` via `Config.MasterKey`.

Policy scopes: `r` (read), `w` (write), `rw` (read+write), `none` (denied) per resource.

## Security

- Path guard: `filepath.Clean` + prefix check + symlink evaluation
- Upload allowlist: `jpg/jpeg/png/webp/gif` only (no HTML/SVG — prevents stored XSS)
- Filename sanitize: `[a-z0-9._-]`, lowercase
- HTML sanitizer at render: allowlist tags/attrs, scheme allowlist for src/href (no `javascript:`, `data:`, `//`)
- Key comparison: `crypto/subtle.ConstantTimeCompare`
- Keys file: `0600` permissions, atomic writes

## SEO output

Automatically generated in `<head>`: title, meta description, robots, canonical, Open Graph, Twitter Card, JSON-LD (Article + BreadcrumbList), `<link rel="alternate" type="text/markdown">`. Also: `sitemap.xml`, `llms.txt`, `robots.txt` (editable).

Breadcrumbs: auto from directory nesting with editable names via `section.json` and `seo.breadcrumb_title`.

## Custom renderer

```go
type myRenderer struct{}

func (r *myRenderer) Render(md []byte) ([]byte, error) {
    // Use any md→HTML library (gomarkdown, goldmark, etc.)
    return myMDToHTML(md), nil
}
```

## Testing

```bash
go test ./...         # 33 tests: store, render, keystore, smoke
go vet ./...
```
