# REST API Reference

golink-url-shortener exposes a REST API on the same host as the web UI. All API
routes live under the `/api/` prefix. Responses are always JSON.

---

## Authentication

Every API request must be authenticated. Two methods are supported:

### API Key

API keys are created by administrators on the `/apikeys` page or via
`POST /api/apikeys`. Supply the raw key in one of two ways:

```
X-API-Key: <raw-key>
```

or

```
Authorization: Bearer <raw-key>
```

API keys are either **read-only** (default) or **read/write**:

- **Read-only** keys may resolve links, list/get link details, and export the
  full link database.
- **Read/write** keys additionally have full write access: create, edit, delete
  links; manage API keys; and import.

Keys are stored server-side only as a SHA-256 hash; the raw value is shown to
the administrator once at creation time.

### Session Cookie

Browser sessions established via OIDC or Tailscale auth are also accepted. The
session JWT is stored in a `Secure`, `HttpOnly`, `SameSite=Lax` cookie and is
validated automatically on every request.

---

## Common Response Codes

- **200** — Success
- **201** — Resource created
- **204** — Success, no content
- **400** — Bad request — invalid input
- **401** — Not authenticated
- **403** — Forbidden — authenticated but not allowed
- **404** — Resource not found
- **409** — Conflict — e.g. duplicate link name
- **500** — Internal server error

Error responses have the form:

```json
{"error": "human-readable message"}
```

---

## Links

### List Links

```
GET /api/links
```

Returns a paginated list of all links.

**Query parameters**

- **`page`** (integer, default: 1) — 1-based page number.
- **`limit`** (integer, default: 100) — Results per page (max 100).
- **`q`** (string) — Search query matched against link names, target URLs, alias
  targets, and owner emails. Supports field prefixes (`name:`/`n:`, `target:`/`t:`,
  `alias:`/`a:`, `owner:`/`o:`) and `^`/`$` anchors. See the
  [search syntax help](/help/search) for details.
- **`sort`** (string, default: `name`) — Sort field: `name`, `created`,
  `last_used`, or `use_count`.
- **`dir`** (string, default: `asc`) — Sort direction: `asc` or `desc`.

**Response**

```json
{
  "links": [
    {
      "name": "docs",
      "target": "https://docs.example.com",
      "owner_email": "alice@example.com",
      "is_advanced": false,
      "require_auth": false,
      "created_at": "2024-01-15T10:30:00Z",
      "use_count": 42
    }
  ],
  "total": 1,
  "page": 1,
  "total_pages": 1
}
```

---

### Get Link

```
GET /api/links/{name}
```

Returns a single link by name (case-insensitive).

**Response** `200 OK`

```json
{
  "name": "docs",
  "target": "https://docs.example.com",
  "owner_email": "alice@example.com",
  "is_advanced": false,
  "require_auth": false,
  "created_at": "2024-01-15T10:30:00Z",
  "use_count": 42
}
```

Returns `404` if the link does not exist.

---

### Create Link

```
POST /api/links
Content-Type: application/json
```

Creates a new short link.

**Request body**

```json
{
  "name": "docs",
  "target": "https://docs.example.com",
  "is_advanced": false,
  "require_auth": false
}
```

- **`name`** (string, required) — Link name: ASCII alphanumeric, `-`, `_`, `.`;
  not a reserved word.
- **`target`** (string, required) — Redirect target URL (`http://` or `https://`
  only).
- **`is_advanced`** (boolean) — When true, `target` is treated as a Go template.
- **`require_auth`** (boolean) — When true, only authenticated users may follow
  the redirect.

**Response** `201 Created` with the created LinkResponse.

Returns `400` for invalid name or target, `409` if the name already exists.

---

### Update Link

```
PATCH /api/links/{name}
Content-Type: application/json
```

Updates one or more fields of an existing link. Only supply the fields you want
to change (field-mask semantics — omitted fields are unchanged).

The caller must be the link owner, a shared user, or an admin.

**Request body** (all fields optional)

```json
{
  "target": "https://new-target.example.com",
  "is_advanced": true,
  "require_auth": false
}
```

**Response** `200 OK` with the updated LinkResponse.

Returns `400` for an invalid target, `403` if the caller lacks permission,
`404` if the link does not exist.

---

### Delete Link

```
DELETE /api/links/{name}
```

Permanently deletes a link. Only the link owner or an admin may delete.

**Response** `204 No Content`

Returns `403` if the caller lacks permission, `404` if the link does not exist.

---

## Quick Name

```
GET /api/quickname
```

Returns an HTML `<input>` element pre-filled with a randomly generated link
name. Intended for use with HTMX in the link creation form, but usable directly
to obtain a random name suggestion.

**Response** `200 OK` `text/html` — an `<input>` element string.

---

## API Keys (admin only)

All `/api/apikeys` endpoints require admin access.

### List API Keys

```
GET /api/apikeys
```

Returns all API keys (without the raw key value).

**Response** `200 OK`

```json
[
  {
    "id": 1,
    "name": "CI pipeline",
    "created_by": "alice@example.com",
    "created_at": "2024-01-15T10:30:00Z",
    "last_used": "2024-03-01T08:00:00Z",
    "read_only": true
  }
]
```

`last_used` is omitted when the key has never been used.

---

### Create API Key

```
POST /api/apikeys
Content-Type: application/json
```

Creates a new API key. The raw key is returned exactly once in the response and
is never retrievable again.

**Request body**

```json
{"name": "CI pipeline", "read_only": true}
```

- **`name`** (string, required) — Human-readable description for the key.
- **`read_only`** (boolean) — `true` (default) for read-only access; `false`
  for read/write.

**Response** `201 Created`

```json
{
  "id": 2,
  "name": "CI pipeline",
  "created_by": "alice@example.com",
  "created_at": "2024-03-25T12:00:00Z",
  "read_only": true,
  "raw_key": "abcdefghijklmnopqrstuvwxyz012345"
}
```

Store `raw_key` immediately — it will not be shown again.

---

### Revoke API Key

```
DELETE /api/apikeys/{id}
```

Permanently revokes an API key.

**Response** `204 No Content`

Returns `404` if the key ID does not exist.

---

## Import / Export (admin only)

Both import and export endpoints require admin access.

### Export All Links

```
GET /api/export
```

Streams a full JSON dump of all links and their shares. The response is
streamed in pages of 500 links to avoid loading the entire database into
memory. Suitable for large installations.

**Response** `200 OK` `application/json`

```json
{
  "version": 1,
  "exported_at": "2024-01-15T10:30:00Z",
  "links": [
    {
      "name": "docs",
      "target": "https://docs.example.com",
      "owner_email": "alice@example.com",
      "is_advanced": false,
      "require_auth": false,
      "created_at": "2024-01-15T10:30:00Z",
      "use_count": 42,
      "shares": ["bob@example.com"]
    }
  ]
}
```

The `shares` field is omitted when a link has no shares. The response also
sets `Content-Disposition: attachment; filename="golink-export.json"` to
encourage browsers to download rather than display the file.

Returns `401` if not authenticated, `403` if not admin.

---

### Import Links

```
POST /api/import
Content-Type: application/json
```

Upserts all links from an export document. For each link:

- If a link with the same name already exists, its mutable fields (target,
  is_advanced, require_auth) are updated.
- If no link with that name exists, a new link is created. Missing
  `owner_email` falls back to the importing admin's email.
- Shares are restored for newly created links.
- Links with invalid names or targets are skipped and reported in the
  response errors list.

**Request body** — the same format produced by `GET /api/export`:

```json
{
  "version": 1,
  "exported_at": "2024-01-15T10:30:00Z",
  "links": [...]
}
```

**Response** `200 OK`

```json
{
  "created": 5,
  "updated": 2,
  "skipped": 1,
  "errors": [
    "bad name!: link name contains invalid characters"
  ]
}
```

- **`created`** — Number of new links created.
- **`updated`** — Number of existing links updated.
- **`skipped`** — Number of links skipped due to validation or DB errors.
- **`errors`** — Human-readable error messages for each skipped link (omitted
  when empty).

Returns `400` for malformed JSON body, `401` if not authenticated, `403` if
not admin.

---

## Admin UI

The admin web interface for managing API keys is available at:

```
GET  /apikeys              List all keys; form to create a new key
POST /apikeys              Create a new key (form submission)
POST /apikeys/{id}/delete  Revoke a key (form submission)
```

These routes require an authenticated admin session (not API key auth).

---

## Advanced Links

When `is_advanced` is `true`, the `target` field is evaluated as a Go template
before the redirect is issued.

**Template variables:**

- **`path`** (string) — Full path suffix after the link name.
- **`parts`** ([]string) — Path suffix split by `/`.
- **`args`** (string) — Query string portion.
- **`ua`** (string) — User-Agent header value.
- **`email`** (string) — Authenticated user's email (empty if anonymous).

**Additional template functions:**

- **`match(pattern, s)`** — True if `s` contains a match for `pattern`.
- **`extract(pattern, s)`** — Returns the first submatch group.
- **`replace(pattern, repl, s)`** — Regexp replace on `s`.

**Example**: redirect `go/jira/PROJ-123` to
`https://jira.example.com/browse/PROJ-123`:

```
https://jira.example.com/browse/{{index .parts 0}}
```

---

## Rate Limiting

No rate limiting is currently enforced at the application layer. Operators are
encouraged to place a reverse proxy in front of the service for rate limiting
and TLS termination.
