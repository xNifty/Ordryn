# GoTodo API

All paths in this document are relative to the configured `BASE_PATH`.

## Authentication

The saved views API uses the same session cookie as the GoTodo web application.
Clients must authenticate through the application before calling these endpoints.
An `HX-Request` header is not required.

Saved views are private to the authenticated user. An item owned by another user
is returned as `404 Not Found`, rather than revealing that it exists.

## Saved views

A saved view is a named, reusable set of task-list filters. Fetching a view does
not fetch tasks; clients apply a view by passing its `filter` values to the task
list endpoint.

Each user can store at most 20 views. Names must be unique for that user.

### Endpoints

| Method | Path | Description | Success |
| --- | --- | --- | --- |
| `GET` | `/api/v1/saved-views` | List the current user's views | `200 OK` |
| `POST` | `/api/v1/saved-views` | Create a view | `201 Created` |
| `GET` | `/api/v1/saved-views/{id}` | Fetch one view | `200 OK` |
| `PUT` | `/api/v1/saved-views/{id}` | Replace a view's name and filter | `200 OK` |
| `PATCH` | `/api/v1/saved-views/{id}` | Update one or more fields | `200 OK` |
| `DELETE` | `/api/v1/saved-views/{id}` | Delete a view | `204 No Content` |

Collection results are ordered by `sort_order`, then case-insensitively by
`name`, and finally by `id`.

### Saved view object

```json
{
  "id": 12,
  "name": "Overdue work",
  "filter": {
    "project": "3",
    "status": "incomplete",
    "due": "overdue",
    "priority": "2",
    "tag": "7",
    "sort": "priority",
    "search": "release"
  },
  "sort_order": 0,
  "created_at": "2026-07-14T15:44:17Z",
  "updated_at": "2026-07-14T15:44:17Z"
}
```

`id`, `created_at`, and `updated_at` are server-managed. Request bodies may
contain:

| Field | Type | Rules |
| --- | --- | --- |
| `name` | string | Required on create and `PUT`; trimmed; 1–80 characters |
| `filter` | object | Optional on create, required on `PUT`, optional on `PATCH` |
| `sort_order` | integer | Optional; between `0` and `2147483647` |

If `sort_order` is omitted when creating a view, it defaults to the number of
views the user already has. If omitted during an update, its current value is
preserved.

The filter object accepts these string fields:

| Field | Accepted values |
| --- | --- |
| `project` | Empty for all projects; a positive project ID; `0` or `none` for tasks without a project |
| `status` | Empty for any status; `complete`, `completed`, or `incomplete` |
| `due` | Empty for any due date; `overdue`, `today`, `week`, or `none` |
| `priority` | Empty for any priority; `0`, `1`, `2`, or `3` |
| `tag` | Empty for any tag; a positive tag ID |
| `sort` | Empty for default ordering; `priority` |
| `search` | Any string up to 500 characters |

Filter values are trimmed. Named values are case-insensitive and normalized in
responses; for example, `completed` is returned as `complete`.

Request bodies must contain one JSON object, may be at most 1 MiB, and may not
contain unknown fields.

### Create a view

```http
POST /api/v1/saved-views
Content-Type: application/json

{
  "name": "Overdue work",
  "filter": {
    "status": "incomplete",
    "due": "overdue",
    "sort": "priority"
  }
}
```

The response contains the created saved view and a `Location` header pointing
to `/api/v1/saved-views/{id}`.

### Update a view

`PUT` requires both `name` and `filter`. `sort_order` remains optional:

```http
PUT /api/v1/saved-views/12
Content-Type: application/json

{
  "name": "Today's work",
  "filter": {
    "status": "incomplete",
    "due": "today"
  },
  "sort_order": 1
}
```

`PATCH` requires at least one of `name`, `filter`, or `sort_order`:

```http
PATCH /api/v1/saved-views/12
Content-Type: application/json

{
  "sort_order": 2
}
```

### Errors

Errors use a consistent JSON shape:

```json
{
  "error": "invalid_request",
  "message": "At least one field must be provided."
}
```

| Status | Error code | Meaning |
| --- | --- | --- |
| `400 Bad Request` | `invalid_request` | Invalid path ID, JSON, field, or filter value |
| `401 Unauthorized` | `unauthorized` | No valid authenticated session |
| `404 Not Found` | `not_found` | The view does not exist or belongs to another user |
| `405 Method Not Allowed` | `method_not_allowed` | The method is not supported for this path |
| `409 Conflict` | `name_conflict` | The user already has a view with this name |
| `409 Conflict` | `limit_reached` | The user already has 20 saved views |
| `500 Internal Server Error` | `internal_error` | The operation failed unexpectedly |
