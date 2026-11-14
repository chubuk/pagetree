# REST API PageTree

[Русский](API.md)

## Публичные маршруты (без авторизации)

### GET /pages/{path...}
Страница (draft → 404). Если путь указывает на файл в `_media/`/`media/` — отдаёт байты.

### GET /news/{path...}
Новость. `/news/` — блог-лента (date DESC, `?page=N`). `/news/{section}/` — листинг раздела.

### GET /media/{path...}
Статические файлы (картинки).

### GET /api/content/menu
JSON меню:
```json
{
  "pages": [{"title": "О нас", "url": "/pages/about", "order": 10}],
  "news_sections": [{"title": "Релизы", "url": "/news/releases/", "order": 1, "count": 5}]
}
```

## Admin API (JWT + роль admin/super_admin)

### GET /api/admin/content/list?root=pages|news
Дерево файлов (md+json, сироты с пометкой).

### GET /api/admin/content/file?path=pages/about.md
Прочитать `.md` или `.json`.

### PUT /api/admin/content/page
Записать `{path, md, json}` атомарно (tmp+rename).

### DELETE /api/admin/content/page?path=pages/about.md
Удалить тройку (md + json + _media/).

### POST /api/admin/content/rename
`{old_path, new_path}` — переименовать тройку.

### POST /api/admin/content/upload
multipart: `file` + `dir`. Allowlist: jpg/jpeg/png/webp/gif.

### GET /api/admin/content/media?path=media
Список файлов (name, url, size, mtime).

### DELETE /api/admin/content/media?path=media/photo.jpg
Удалить файл.

### POST /api/admin/content/media/rename
`{old_path, new_path}`.

## Agent API (Bearer ptk_*)

REST-роуты на `/api/agent/v1/` — аналогичны admin API, но авторизация через Bearer-ключ с policy.

| Метод | Путь | Scope |
|---|---|---|
| GET | `/pages`, `/pages/{path}` | pages:read |
| PUT | `/pages/{path}` | pages:write |
| DELETE | `/pages/{path}` | pages:write |
| GET | `/news`, `/news/{path}` | news:read |
| PUT | `/news/{path}` | news:write |
| DELETE | `/news/{path}` | news:write |
| POST | `/upload` | media:write |
| GET | `/menu` | pages:read |

## Keys API (JWT + роль admin/super_admin)

| Метод | Путь | Описание |
|---|---|---|
| GET | `/api/admin/content/keys` | Список ключей (без raw) |
| POST | `/api/admin/content/keys` | Создать (raw показывается один раз) |
| GET | `/api/admin/content/keys/{id}` | Получить |
| PUT | `/api/admin/content/keys/{id}` | Обновить имя/policy |
| DELETE | `/api/admin/content/keys/{id}` | Удалить |

## Коды ошибок

| Код | Значение |
|---|---|
| 400 | Неверные параметры |
| 401 | Нет/невалидный токен |
| 403 | Нет прав (policy) |
| 404 | Не найдено |
| 413 | Файл слишком большой (>20MB) |
| 500 | Внутренняя ошибка |
