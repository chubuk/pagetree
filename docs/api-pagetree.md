# REST API PageTree — полные возможности

PageTree — контент-движок (`.md` + sidecar `.json`), встраиваемый Go-модуль. Все адреса ниже — относительно корня сайта, на котором смонтирован модуль (например, `https://mcp-is.ru`).

## Типы доступа

| Контур | Авторизация | Назначение |
|---|---|---|
| Публичный | нет | Отдача страниц, новостей, медиа, меню |
| Agent API | Bearer `ptk_*` (ключ PageTree) | Доступ агентов (REST) |
| Admin Content API | JWT (роль `admin`/`super_admin`) | Редактор, файлы |
| Keys API | JWT (роль `admin`/`super_admin`) | Управление ключами агентов |

Страницы редактора: `/content/admin/` (Страницы) и `/content/admin/keys.html` (Ключи и доступы) — UI-шлюз только для admin/super_admin; данные всегда проверяются серверно.

## 1. Публичные роуты (кэшируются)

| Метод | Путь | Описание |
|---|---|---|
| GET | `/pages/{path...}` | Страница (markdown → HTML); листинг секции; `*_media/` вложения |
| GET | `/news/{path...}` | Новость; листинг новостей/блога |
| GET | `/media/{path...}` | Медиафайлы |
| GET | `/api/content/menu` | Меню сайта (JSON) |

Кэш: страницы/новости/меню — `public, max-age=60`; медиа — `public, max-age=3600`.

## 2. Agent API — `/api/agent/v1/` (Bearer `ptk_*`)

| Метод | Путь | Scope | Описание |
|---|---|---|---|
| GET | `/pages` | pages:r | Дерево страниц (`?root=pages\|news`) |
| GET | `/pages/{path}` | pages:r | Файл страницы (md + json) |
| PUT | `/pages/{path}` | pages:w | Сохранить страницу (md + json) |
| DELETE | `/pages/{path}` | pages:w | Удалить тройку: .md + .json + `*_media/` |
| GET | `/news`, `/news/{path}` | news:r | То же для новостей |
| PUT | `/news/{path}` | news:w | Сохранить новость |
| DELETE | `/news/{path}` | news:w | Удалить новость |
| POST | `/upload` | media:w | Загрузка файла (multipart: `file`, `dir`) |
| GET | `/menu` | pages:r | Меню сайта (JSON) |

### Политика ключа
- Scope: `pages`/`news`/`media` × `r` / `rw` / `w` / `none`.
- Path-скоуп: список префиксов путей (например, только `pages/blog/`) — проверяется для `path`/`old_path`/`new_path`/`dir`/`filename`; пусто = без ограничений.
- Отказ: `403 {"error":"недостаточно прав"}`.

### Лимиты
- 10 запросов/с на ключ (burst 30); загрузки — 2/min (burst 2). Превышение → `429`.
- Тело запроса: до 32 МБ (base64-загрузки); раскодированный файл — до 20 МБ.

## 3. Admin Content API — `/api/admin/content/` (JWT, admin)

Кэш ответов: `no-store` (админ всегда видит свежее).

| Метод | Путь | Описание |
|---|---|---|
| GET | `/list?root=pages\|news` | Дерево контента |
| GET | `/file?path=` | Содержимое `.md` или `.json` |
| PUT | `/page` | Сохранить страницу: `{path, md, json}` (атомарно md+json) |
| PUT | `/section` | Сохранить `section.json` раздела: `{path, json}` |
| POST | `/upload` | Загрузка медиа (multipart: `file`, `dir`) |
| POST | `/rename` | Переименовать: `{old_path, new_path}` (тройка вместе) |
| DELETE | `/page?path=` | Удалить страницу (тройку) |
| GET | `/media?path=` | Список медиа каталога |
| DELETE | `/media?path=` | Удалить медиафайл |
| POST | `/media/rename` | Переименовать медиа: `{old_path, new_path}` |

## 4. Keys API — `/api/admin/content/keys/` (JWT, admin)

| Метод | Путь | Описание |
|---|---|---|
| GET | `/` | Список ключей (без raw) |
| GET | `/client-types` | Шаблоны конфигов 10 MCP-клиентов (статические, без БД) |
| POST | `/` | Создать: `{name, policy}` → `{key, raw_key}` (raw показывается **один раз**) |
| GET | `/{id}` | Получить ключ |
| PUT | `/{id}` | Обновить имя/policy (merge) |
| POST | `/{id}/revoke` | Отозвать (soft — запись остаётся в журнале) |
| DELETE | `/{id}` | Удалить безвозвратно |

Формат ключа: `ptk_live_<64hex>` (или `ptk_test_`), хранится только SHA-256-хэш; опционально AES-256-GCM шифрование `keys.json`.

## 5. Форматы данных (sidecar JSON)

**Страница (`about.json` рядом с `about.md`):**
```json
{
  "title": "Контакты",
  "menu": true,
  "order": 5,
  "date": "2026-01-15",
  "modified": "2026-01-20",
  "draft": false,
  "seo": {
    "meta_title": "…", "meta_description": "…",
    "robots": "index, follow", "og_image": "/media/og.jpg",
    "canonical": "https://…", "breadcrumb_title": "…"
  }
}
```

**Раздел (`section.json` в каталоге):**
```json
{ "title": "Документация", "order": 2, "menu": true }
```

Создание страницы в новой папке автоматически создаёт `section.json` во всех новых каталогах.

## 6. Коды ошибок

| Код | Значение |
|---|---|
| 400 | Неверные параметры / невалидный JSON |
| 401 | Нет/невалидный токен |
| 403 | Нет прав (scope / path-скоуп) |
| 404 | Не найдено |
| 413 | Файл слишком большой (>20 МБ) |
| 429 | Превышен лимит запросов |
| 500 | Внутренняя ошибка |

## 7. Примеры

```bash
# Публичная страница
curl https://example.com/pages/about

# Agent API: создать страницу ключом с path-скоупом pages/blog/
curl -X PUT https://example.com/api/agent/v1/pages/pages/blog/post.md \
  -H "Authorization: Bearer ptk_live_..." -H 'Content-Type: application/json' \
  -d '{"path":"pages/blog/post.md","md":"# Заголовок","json":{"title":"Запись","menu":true,"order":1}}'

# Admin: список ключей (JWT)
curl https://example.com/api/admin/content/keys/ -H "Authorization: Bearer <jwt>"
```

Связанное: [MCP API PageTree](mcp-pagetree.md), [Формат контента](CONTENT.md), [Безопасность](SECURITY.md).
