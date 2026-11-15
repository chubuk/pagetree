# MCP-эндпоинт PageTree

[Русский](MCP.md) | [English](en/MCP.md)

MCP (Model Context Protocol) — JSON-RPC 2.0 протокол для подключения AI-агентов (Claude Desktop, MiMoCode, Cursor и др.) к контенту сайта.

## Подключение

**Endpoint:** `https://your-site.ru/api/agent/mcp`

**Авторизация:** Bearer-токен `ptk_live_*` в заголовке `Authorization`:

```bash
curl -X POST https://your-site.ru/api/agent/mcp \
  -H "Authorization: Bearer ptk_live_abc123..." \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"initialize","id":1,"params":{}}'
```

## Конфигурация MCP-клиента

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

Используйте skill `mcp-is-data` или прямой HTTP — endpoint совместим с MCP Streamable HTTP.

## MCP-тулы

### list_pages
Список страниц с пагинацией.
```json
{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{
  "name":"list_pages","arguments":{"root":"pages","page":1,"per_page":20}
}}
```

### get_page
Чтение страницы (markdown + метаданные).
```json
{"jsonrpc":"2.0","method":"tools/call","id":3,"params":{
  "name":"get_page","arguments":{"path":"pages/about.md"}
}}
```

### save_page
Сохранение страницы (атомарно: .md + .json).
```json
{"jsonrpc":"2.0","method":"tools/call","id":4,"params":{
  "name":"save_page","arguments":{
    "path":"pages/about.md",
    "md":"# О нас\n\nТекст страницы...",
    "json":{"title":"О нас","menu":true,"order":10,"draft":false}
  }
}}
```

### delete_page
Удаление тройки (.md + .json + _media/).
```json
{"jsonrpc":"2.0","method":"tools/call","id":5,"params":{
  "name":"delete_page","arguments":{"path":"pages/old-page.md"}
}}
```

### rename_page
Переименование тройки.
```json
{"jsonrpc":"2.0","method":"tools/call","id":6,"params":{
  "name":"rename_page","arguments":{"old_path":"pages/old.md","new_path":"pages/new.md"}
}}
```

### list_media
Список картинок.
```json
{"jsonrpc":"2.0","method":"tools/call","id":7,"params":{
  "name":"list_media","arguments":{"dir":"media"}
}}
```

### upload_media
Загрузка картинки (base64, до 20MB).
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
Меню сайта (JSON).
```json
{"jsonrpc":"2.0","method":"tools/call","id":9,"params":{
  "name":"get_menu","arguments":{}
}}
```

## Политика доступа (scope)

Каждый тул имеет scope: `pages`/`news`/`media` × `read`/`write`. Ключ проверяется на каждый вызов.

| Scope | Тулы |
|---|---|
| `pages:read` | list_pages, get_page, get_menu |
| `pages:write` | save_page, delete_page, rename_page |
| `news:read` | list_news, get_news |
| `news:write` | save_news, delete_news |
| `media:read` | list_media |
| `media:write` | upload_media, delete_media |

Дополнительно к scope действует **path-скоуп**: `paths` в политике ключа — список префиксов путей (например, `pages/blog/`). Если список задан, вызовы с `path`/`old_path`/`new_path`/`dir` вне префиксов получают отказ. Пустой список = без ограничений.

## nginx (SSE)

Для корректной работы Streamable HTTP (SSE) нужен специальный location:

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

## Формат ответов

Успешный вызов:
```json
{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"..."}],"structuredContent":{"items":[...],"total":5}}}
```

Ошибка (тул):
```json
{"jsonrpc":"2.0","id":3,"result":{"content":[{"type":"text","text":"file not found"}],"isError":true}}
```

## Файлы и картинки (для агентов)

Куда класть файлы через `upload_media` / `POST /api/agent/v1/upload`:

| Назначение | `dir` | Итоговый URL | В markdown |
|---|---|---|---|
| Общая картинка | `media` (по умолчанию) | `/media/photo.jpg` | `![](/media/photo.jpg)` |
| Вложение страницы | `<страница>_media`, напр. `pages/about_media` для `pages/about.md` | `/pages/about_media/photo.jpg` | `<img src="/pages/about_media/photo.jpg" width="600">` |

- Форматы: jpg/jpeg/png/webp/gif; файл ≤ 20 МБ (в `data` — base64).
- Имя файла: латиница, цифры, `._-` (загрузка санитизирует имена).
- Каталог вложения создаётся автоматически при загрузке.

## Ограничения

- Body: 32MB max (base64 + JSON overhead)
- Decoded файл: 20MB max
- Форматы: jpg/jpeg/png/webp/gif (upload allowlist)
- Rate limit (per ключ): 10 req/s (burst 30); загрузка файлов — 2/min (burst 2). Превышение → `429` / ошибка тула «превышен лимит запросов».
