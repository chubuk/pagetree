# MCP API PageTree — полные возможности

MCP (Model Context Protocol) эндпоинт PageTree для ИИ-агентов: 13 инструментов поверх контента сайта. Транспорт — Streamable HTTP (JSON-RPC 2.0).

## Подключение

- **Эндпоинт:** `https://<ваш-сайт>/api/agent/mcp`
- **Авторизация:** Bearer-ключ `ptk_*` (заголовок `Authorization` или параметр `?access_token=`)
- **Сессия:** `initialize` возвращает `Mcp-Session-Id` — обязателен для последующих вызовов
- **Body cap:** 32 МБ (base64-загрузки); раскодированный файл — 20 МБ

Готовые конфиги для 10 клиентов (Claude Desktop, Cursor, VS Code, MiMoCode, OpenCode, LibreChat, Open WebUI, Cline, Cherry Studio, Continue) — в админке на странице «Ключи и доступы» (`/content/admin/keys.html`): выбор агента → «Скачать файл» или «Скачать .env». Плейсхолдеры шаблонов: `{{KEY_NAME}}` (имя ключа), `{{URL}}` (этот эндпоинт), `{{API_KEY}}` (raw-ключ).

## Инструменты (13)

| Тул | Scope | Аргументы | Описание |
|---|---|---|---|
| `list_pages` | pages:r | `root?`, `page?`, `per_page?` | Дерево страниц |
| `get_page` | pages:r | `path` | Страница: markdown + метаданные |
| `save_page` | pages:w | `path`, `md`, `json?` | Сохранить страницу (md+json атомарно) |
| `delete_page` | pages:w | `path` | Удалить тройку (.md+.json+`*_media/`) |
| `rename_page` | pages:w | `old_path`, `new_path` | Переименовать тройку |
| `list_news` | news:r | `page?`, `per_page?` | Список новостей |
| `get_news` | news:r | `path` | Новость: markdown + метаданные |
| `save_news` | news:w | `path`, `md`, `json?` | Сохранить новость (поле `date` важно для сортировки) |
| `delete_news` | news:w | `path` | Удалить новость |
| `list_media` | media:r | `dir?` | Файлы каталога (name, url, size, mtime) |
| `upload_media` | media:w | `filename`, `data`, `dir?` | Загрузка (base64, jpg/png/webp/gif) → URL |
| `delete_media` | media:w | `path` | Удалить медиафайл |
| `get_menu` | pages:r | — | Меню сайта (JSON) |

### Куда класть файлы (media)

| Назначение | `dir` | Итоговый URL | В markdown |
|---|---|---|---|
| Общая картинка | `media` (по умолчанию) | `/media/photo.jpg` | `![](/media/photo.jpg)` |
| Вложение страницы | `<страница>_media`, напр. `pages/about_media` для `pages/about.md` | `/pages/about_media/photo.jpg` | `<img src="/pages/about_media/photo.jpg" width="600">` |

Имена файлов санитизируются (латиница, цифры, `._-`); каталог вложения создаётся автоматически.

## Политика доступа

Каждый вызов проверяется по политике ключа:
- **Scope:** `pages`/`news`/`media` × `r`/`rw`/`w`/`none` (см. таблицу выше).
- **Path-скоуп:** `paths` в политике — префиксы путей (например, `pages/blog/`); вызовы с `path`/`old_path`/`new_path`/`dir` вне префиксов получают отказ.
- **Лимиты:** 10 req/s на ключ (burst 30); `upload_media` — 2/min (burst 2). Превышение → ошибка «превышен лимит запросов».

Отказ — дружелюбная ошибка тула (`isError`), а не падение сессии.

## Формат ответов

Успех:
```json
{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"..."}],"structuredContent":{"path":"pages/about.md","url":"/pages/about","markdown":"...","meta":{...}}}}
```

Ошибка:
```json
{"jsonrpc":"2.0","id":3,"result":{"content":[{"type":"text","text":"file not found"}],"isError":true}}
```

## Примеры JSON-RPC

```json
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"my-agent","version":"1"}}}

{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"save_page","arguments":{
  "path":"pages/blog/post.md","md":"# Заголовок\n\nТекст",
  "json":{"title":"Запись","menu":true,"order":1,"date":"2026-01-15"}
}}}

{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"upload_media","arguments":{
  "filename":"photo.jpg","data":"/9j/4AAQSkZJRg...","dir":"pages/blog_media"
}}}
```

Связанное: [REST API PageTree](api-pagetree.md), [MCP-эндпоинт (справка)](MCP.md), [Безопасность](SECURITY.md).
