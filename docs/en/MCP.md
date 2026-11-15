# PageTree MCP Endpoint

[Русский](../MCP.md) | [English](MCP.md)

MCP (Model Context Protocol) — JSON-RPC 2.0 protocol for connecting AI agents (Claude Desktop, MiMoCode, Cursor, etc.) to site content.

## Connection

**Endpoint:** `https://your-site.ru/api/agent/mcp`

**Authorization:** Bearer token `ptk_live_*` in `Authorization` header:

```bash
curl -X POST https://your-site.ru/api/agent/mcp \
  -H "Authorization: Bearer ptk_live_abc123..." \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"initialize","id":1,"params":{}}'
```

## MCP Client Configuration

### Claude Desktop (`claude_desktop_config.json`)

```json
{
  "mcpServers": {
    "pagetree": {
      "url": "https://your-site.ru/api/agent/mcp",
      "headers": {
        "Authorization": "Bearer ptk_live_abc123..."
      }
    }
  }
}
```

### MiMoCode

Use the `mcp-is-data` skill or direct HTTP — the endpoint is MCP Streamable HTTP compatible.

## MCP Tools

### list_pages
List pages with pagination.
```json
{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{
  "name":"list_pages","arguments":{"root":"pages","page":1,"per_page":20}
}}
```

### get_page
Read a page (markdown + metadata).
```json
{"jsonrpc":"2.0","method":"tools/call","id":3,"params":{
  "name":"get_page","arguments":{"path":"pages/about.md"}
}}
```

### save_page
Save a page atomically (.md + .json).
```json
{"jsonrpc":"2.0","method":"tools/call","id":4,"params":{
  "name":"save_page","arguments":{
    "path":"pages/about.md",
    "md":"# About us\n\nPage content...",
    "json":{"title":"About us","menu":true,"order":10,"draft":false}
  }
}}
```

### delete_page
Delete a triple (.md + .json + _media/).
```json
{"jsonrpc":"2.0","method":"tools/call","id":5,"params":{
  "name":"delete_page","arguments":{"path":"pages/old-page.md"}
}}
```

### rename_page
Rename a triple.
```json
{"jsonrpc":"2.0","method":"tools/call","id":6,"params":{
  "name":"rename_page","arguments":{"old_path":"pages/old.md","new_path":"pages/new.md"}
}}
```

### list_media
List media files.
```json
{"jsonrpc":"2.0","method":"tools/call","id":7,"params":{
  "name":"list_media","arguments":{"dir":"media"}
}}
```

### upload_media
Upload a media file (base64, up to 20MB).
```json
{"jsonrpc":"2.0","method":"tools/call","id":8,"params":{
  "name":"upload_media","arguments":{
    "filename":"photo.jpg",
    "data":"/9j/4AAQSkZJRg...",
    "dir":"media"
  }
}}
```

### get_menu
Site menu (JSON).
```json
{"jsonrpc":"2.0","method":"tools/call","id":9,"params":{
  "name":"get_menu","arguments":{}
}}
```

## Access Policy (scope)

Each tool has a scope: `pages`/`news`/`media` × `read`/`write`. Key is checked on every call.

| Scope | Tools |
|---|---|
| `pages:read` | list_pages, get_page, get_menu |
| `pages:write` | save_page, delete_page, rename_page |
| `news:read` | list_news, get_news |
| `news:write` | save_news, delete_news |
| `media:read` | list_media |
| `media:write` | upload_media, delete_media |

## nginx (SSE)

For Streamable HTTP (SSE) to work correctly:

```nginx
location = /api/agent/mcp {
    proxy_pass http://app:8080;
    proxy_buffering off;
    proxy_read_timeout 300s;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

## Response Format

Successful call:
```json
{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"..."}],"structuredContent":{"items":[...],"total":5}}}
```

Error (tool):
```json
{"jsonrpc":"2.0","id":3,"result":{"content":[{"type":"text","text":"file not found"}],"isError":true}}
```

## Files and images (for agents)

Where to put files via `upload_media` / `POST /api/agent/v1/upload`:

| Purpose | `dir` | Resulting URL | In markdown |
|---|---|---|---|
| Shared image | `media` (default) | `/media/photo.jpg` | `![](/media/photo.jpg)` |
| Page attachment | `<page>_media`, e.g. `pages/about_media` for `pages/about.md` | `/pages/about_media/photo.jpg` | `<img src="/pages/about_media/photo.jpg" width="600">` |

- Formats: jpg/jpeg/png/webp/gif; file ≤ 20 MB (base64 in `data`).
- Filenames: latin letters, digits, `._-` (upload sanitizes names).
- Attachment directory is created automatically on upload.

## Limits

- Body: 32MB max (base64 + JSON overhead)
- Decoded file: 20MB max
- Formats: jpg/jpeg/png/webp/gif (upload allowlist)
- Rate limit (per key): 10 req/s (burst 30); file uploads — 2/min (burst 2). Exceeding → `429` / tool error "rate limit exceeded".
