# PageTree

[Русский](README.md) | [English](README.en.md)

File-based markdown page/news engine for Go. Pages are `.md` files with sidecar `.json` metadata; URL = directory nesting. No database — files are the source of truth.

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
        Renderer: myMDRenderer,  // any pagetree.Renderer implementation
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

    // Agent API — REST (Bearer ptk_*)
    http.Handle("/api/agent/v1/", engine.AgentAPI())

    // Agent API — MCP (JSON-RPC 2.0, Bearer ptk_*)
    http.Handle("/api/agent/mcp", engine.AgentMCPHandler())

    http.ListenAndServe(":8080", nil)
}
```

## Two APIs for agents

| | REST (`/api/agent/v1/`) | MCP (`/api/agent/mcp`) |
|---|---|---|
| Protocol | HTTP + JSON | JSON-RPC 2.0 (Streamable HTTP) |
| Clients | curl, fetch, LangChain | Claude Desktop, MiMoCode, Cursor |
| Discovery | By documentation | `tools/list` (automatic) |
| Format | URL+method = operation | `tools/call` with tool name |

Both APIs work in parallel with the same authorization (Bearer `ptk_*`) and the same permissions (policy per key).

## Content structure

```
content/
  pages/
    about.md              → /pages/about
    about.json            # metadata
    about_media/          # page images
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

## Sidecar JSON

```json
{
  "title": "About us",
  "menu": true,
  "order": 20,
  "date": "2026-11-01",
  "modified": "2026-11-14",
  "draft": false,
  "seo": {
    "meta_title": "About us — Company N",
    "meta_description": "Brief description",
    "robots": "index, follow",
    "og_image": "/media/og-about.jpg",
    "canonical": "https://site/pages/about"
  }
}
```

See [docs/CONTENT.md](docs/CONTENT.md) (Russian)

## AI keys (agent access)

```go
key, raw, _ := engine.KeyStore().GenerateKey("agent", pagetree.KeyPolicy{
    Pages: "rw", News: "r", Media: "w",
    Paths: []string{"pages/blog/", "news/"}, // optional: path scope, empty = unrestricted
})
// raw = "ptk_live_<64hex>" — save it, can't retrieve later
```

Format: `ptk_live_<64hex>`. Stored as SHA-256 hash. Optional AES-256-GCM encryption of `keys.json`. Policy (scope + path scope) is checked on every REST and MCP call; rate limit — 10 req/s per key (burst 30), uploads — 2/min. Key management lives on the "Ключи и доступы" page in the admin UI (`/content/admin/keys.html`, menu item after "Страницы"): create/revoke, REST/MCP endpoint URLs, ready-made configs for 10 MCP clients (Claude Desktop, Cursor, VS Code, etc.) and `.env` export. See [docs/SECURITY.md](docs/SECURITY.md) (Russian)

## MCP tools (13 operations)

| Tool | Description | Scope |
|---|---|---|
| `list_pages` / `get_page` | Read pages | pages:read |
| `save_page` / `delete_page` / `rename_page` | Write pages | pages:write |
| `list_news` / `get_news` | Read news | news:read |
| `save_news` / `delete_news` | Write news | news:write |
| `list_media` / `upload_media` / `delete_media` | Images | media:read / media:write |
| `get_menu` | Site menu | pages:read |

See [docs/en/MCP.md](docs/en/MCP.md)

## SEO

Auto-generated `<head>`: title, description, robots, canonical, Open Graph, Twitter Card, JSON-LD (Article + BreadcrumbList), `<link rel="alternate" type="text/markdown">`. Plus `sitemap.xml`, `llms.txt`, `robots.txt`. See [docs/SEO.md](docs/SEO.md) (Russian)

## Security

- Path guard: `filepath.Clean` + prefix check + symlink
- Upload allowlist: `jpg/jpeg/png/webp/gif` (no HTML/SVG — prevents stored XSS)
- HTML sanitizer: tag/attribute allowlist, scheme allowlist for src/href
- Keys: `crypto/subtle.ConstantTimeCompare`, SHA-256 hash-then-store; policy (scope + path scope) and per-key rate limit checked on every call

## Documentation

| File | Content |
|---|---|
| [docs/MCP.md](docs/en/MCP.md) | MCP endpoint (JSON-RPC 2.0) |
| [docs/API.md](docs/API.md) | Full REST API (Russian) |
| [docs/CONTENT.md](docs/CONTENT.md) | Content format (Russian) |
| [docs/SECURITY.md](docs/SECURITY.md) | Security model (Russian) |
| [docs/SEO.md](docs/SEO.md) | SEO package (Russian) |

## Tests

```bash
go test ./...    # 37 tests
go vet ./...
```
