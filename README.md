# PageTree

[Русский](README.md) | [English](README.en.md)

Файловый движок markdown-страниц и новостей для Go. Страницы — `.md` файлы с sidecar `.json`; URL = вложенность папок. Без БД — файлы являются источником истины.

## Быстрый старт

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
        Renderer: myMDRenderer,  // любая реализация pagetree.Renderer
    })

    // Публичные маршруты — без авторизации
    http.Handle("GET /pages/", engine.PageHandler())
    http.Handle("GET /news/", engine.NewsHandler())
    http.Handle("GET /media/", engine.MediaHandler())
    http.Handle("GET /api/content/menu", engine.MenuHandler())

    // Админка — обернуть своей auth middleware
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

## Два API для агентов

| | REST (`/api/agent/v1/`) | MCP (`/api/agent/mcp`) |
|---|---|---|
| Протокол | HTTP + JSON | JSON-RPC 2.0 (Streamable HTTP) |
| Клиенты | curl, fetch, LangChain | Claude Desktop, MiMoCode, Cursor |
| Обнаружение операций | По документации | `tools/list` (автоматически) |
| Формат | URL+метод = операция | `tools/call` с именем тула |

Оба API работают параллельно с одинаковой авторизацией (Bearer `ptk_*`) и одинаковыми правами (policy per key).

## Структура контента

```
content/
  pages/
    about.md              → /pages/about
    about.json            # метаданные
    about_media/          # картинки страницы
      photo.jpg
    docs/
      install.md          → /pages/docs/install
      install.json
      section.json        # метаданные папки (title, order, menu)
  news/
    releases/
      v1.2.md             → /news/releases/v1.2
      v1.2.json           # {date: "2026-11-01", ...}
  media/                  # общие картинки
```

## Sidecar JSON

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
    "meta_description": "Краткое описание",
    "robots": "index, follow",
    "og_image": "/media/og-about.jpg",
    "canonical": "https://site/pages/about"
  }
}
```

Подробнее: [docs/CONTENT.md](docs/CONTENT.md)

## AI-ключи (доступ агентов)

```go
key, raw, _ := engine.KeyStore().GenerateKey("agent", pagetree.KeyPolicy{
    Pages: "rw", News: "r", Media: "w",
    Paths: []string{"pages/blog/", "news/"}, // опционально: path-скоуп, пусто = всё
})
// raw = "ptk_live_<64hex>" — сохраните, повторно получить нельзя
```

Формат: `ptk_live_<64hex>`. Хранится как SHA-256 хэш. Опционально AES-256-GCM шифрование `keys.json`. Политика (scope + path-скоуп) проверяется на каждый вызов REST и MCP; лимит — 10 req/s на ключ (burst 30), загрузки — 2/min. Управление ключами — страница «Ключи и доступы» в админке (`/content/admin/keys.html`, пункт меню после «Страницы»): создание/отзыв, адреса REST/MCP, готовые конфиги 10 MCP-клиентов (Claude Desktop, Cursor, VS Code и др.) и экспорт `.env`. Подробнее: [docs/SECURITY.md](docs/SECURITY.md)

## MCP-тулы (13 операций)

| Тул | Описание | Scope |
|---|---|---|
| `list_pages` / `get_page` | Чтение страниц | pages:read |
| `save_page` / `delete_page` / `rename_page` | Запись страниц | pages:write |
| `list_news` / `get_news` | Чтение новостей | news:read |
| `save_news` / `delete_news` | Запись новостей | news:write |
| `list_media` / `upload_media` / `delete_media` | Картинки | media:read / media:write |
| `get_menu` | Меню сайта | pages:read |

Подробнее: [docs/MCP.md](docs/MCP.md)

## SEO

Автоматически в `<head>`: title, description, robots, canonical, Open Graph, Twitter Card, JSON-LD (Article + BreadcrumbList), `<link rel="alternate" type="text/markdown">`. Плюс `sitemap.xml`, `llms.txt`, `robots.txt`. Подробнее: [docs/SEO.md](docs/SEO.md)

## Безопасность

- Path guard: `filepath.Clean` + prefix check + symlink
- Upload allowlist: `jpg/jpeg/png/webp/gif` (без HTML/SVG — предотвращает stored XSS)
- HTML sanitizer: allowlist тегов/атрибутов, scheme allowlist для src/href
- Ключи: `crypto/subtle.ConstantTimeCompare`, SHA-256 hash-then-store; policy (scope + path-скоуп) и per-key rate limit — на каждый вызов

## Документация

| Файл | Содержимое |
|---|---|
| [docs/API.md](docs/API.md) | Полный REST API |
| [docs/MCP.md](docs/MCP.md) | MCP-эндпоинт (JSON-RPC 2.0) |
| [docs/CONTENT.md](docs/CONTENT.md) | Формат контента и жизненный цикл |
| [docs/SECURITY.md](docs/SECURITY.md) | Модель безопасности |
| [docs/SEO.md](docs/SEO.md) | SEO-пакет |

## Тесты

```bash
go test ./...    # 37 тестов
go vet ./...
```
