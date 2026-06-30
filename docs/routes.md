# Routes

A `Route` is one endpoint. It carries the metadata the gateway enforces _before_
your handler runs, and a handler that is pure business logic.

```go
{
    ID: "myplugin.set", Method: plugin.MethodPost, Path: "/entries",
    Permission: "myplugin.write", Risk: plugin.RiskWrite, AuditEvent: "myplugin.set",
    Input: setSchema(), Handle: set,
},
```

## Fields

| Field        | Purpose                                                                               |
| ------------ | ------------------------------------------------------------------------------------- |
| `ID`         | Stable handle referenced by panels/actions/manifest.                                  |
| `Method`     | `MethodGet`, `MethodPost`, `MethodPut`, `MethodPatch`, `MethodDelete`, or `MethodWS`. |
| `Path`       | Route path; `{name}` placeholders arrive via `rc.Param("name")`.                      |
| `Permission` | RBAC permission string the gateway checks.                                            |
| `Risk`       | `RiskSafe` / `RiskWrite` / `RiskDestructive` / `RiskPrivileged`.                      |
| `AuditEvent` | Name recorded in the audit log for this call.                                         |
| `Input`      | Optional `*Schema`; validated by the gateway before the handler runs.                 |
| `Timeout`    | Optional per-request timeout.                                                         |
| `Handle`     | The handler (`func(*RequestContext) (any, error)`).                                   |
| `Stream`     | For `MethodWS` routes only - see [streaming.md](streaming.md).                        |

Route IDs are scoped by plugin name. If your manifest `Name` is `myplugin`, every
route ID must begin with `myplugin.`. The gateway validates this during
registration and resolves route calls by `(connection protocol, route ID)`, not
by a global route table. A plugin cannot call another plugin's route by
declaring `other.route`; that route is not in its own route set, and declaring a
route with another plugin's prefix is rejected.

### Risk levels

Risk drives RBAC and shows (read-only) in the UI:

- `RiskSafe` - read-only (list, describe).
- `RiskWrite` - create/update.
- `RiskDestructive` - delete, truncate, restore.
- `RiskPrivileged` - shell, exec, raw socket.

The gateway resolves the user's role against `Permission`/`Risk` and runs the
audit wrapper. **A plugin cannot widen its own permissions** - it only ships
data and handler bodies.

## Handlers

```go
func set(rc *plugin.RequestContext) (any, error) {
    var in struct {
        Key   string `json:"key" validate:"required"`
        Value string `json:"value"`
    }
    if err := rc.Bind(&in); err != nil {
        return nil, err
    }
    s := rc.Session.(*session)
    s.mu.Lock()
    s.entries[in.Key] = in.Value
    s.mu.Unlock()
    return entry{Key: in.Key, Value: in.Value}, nil
}
```

A handler returns `(value, error)`. The value is JSON-encoded to the client; the
error maps to an HTTP status. Handlers never see `http.ResponseWriter`, headers,
cookies, or auth - that's all gateway-side.

### The RequestContext

`rc *plugin.RequestContext` is your typed view of the request:

- `rc.Ctx` - the request context (honor cancellation in long calls).
- `rc.User` - the acting user (`ID`, `Username`, `DisplayName`, `Roles`).
  Authorization is already enforced; use this only for identity/scoping.
- `rc.Session` - the per-connection `Session` from `Connect`; type-assert it to
  your concrete type.
- `rc.Bind(&dst)` - decode the body into a struct and run `validate` tags.
- `rc.Param("name")` - a path placeholder or renderer-supplied param.
- `rc.Query()` - raw query values.
- `rc.Page()` - parsed cursor/limit/filter/sort for list routes.
- `rc.Uploads("field")` - uploaded multipart files for `FieldFile` inputs.
- `rc.Storage` - scoped plugin-owned persistence; see [storage.md](storage.md).
- `rc.ProxyURL(...)` / `rc.ProxyPrefix()` - proxy URLs for `HTTPProxy` sessions.
- `rc.Audit(result, params, err)` - record an operation inside a long-lived
  route (see [streaming.md](streaming.md)).

### Params vs body

Renderer-supplied params and request bodies are intentionally separate:

- `DataSource.Params` and `Action.Params` become route params. Read them with
  `rc.Param("name")` or `rc.ParamList("name", plugin.ScopeSeparator)`.
- `Action.Body`, form input, editor saves, and file-browser mutations become the
  JSON or multipart body. Decode it with `rc.Bind(&dst)`.

Use params for the stable route address: database, namespace, table, object id,
path, and similar identity fields. Use body for structured mutation payloads:
new values, options, and row identity objects.

```go
// Manifest action.
plugin.Action{
    ID: "myplugin.table.row.delete", Label: "Delete rows",
    RouteID: "myplugin.table.row.delete",
    Params:  map[string]string{"database": "${resource.scope}", "table": "${resource.name}"},
    Body:    map[string]any{"key": "${record._key}"},
    Confirm: true,
    Bulk:    true,
}

// Handler.
func deleteRow(rc *plugin.RequestContext) (any, error) {
    var in struct {
        Key map[string]any `json:"key" validate:"required"`
    }
    if err := rc.Bind(&in); err != nil {
        return nil, err
    }
    database := rc.Param("database")
    table := rc.Param("table")
    // Revalidate database/table/key server-side, then delete.
    return map[string]any{"database": database, "table": table, "deleted": true}, nil
}
```

An exact single-token body value preserves its raw JSON type. For example,
`Body: map[string]any{"key": "${record._key}"}` sends the row key object as an
object. Embedded tokens such as `"row-${record.id}"` interpolate to strings.

## Returning lists

Table/list panels expect a `plugin.Page[T]`:

```go
func list(rc *plugin.RequestContext) (any, error) {
    p, err := rc.Page() // cursor, limit (clamped), filter, sort
    if err != nil {
        return nil, err
    }
    // ...query your backend using p...
    return plugin.Page[entry]{Items: items, NextCursor: next}, nil
}
```

`Page.Items` is the slice, `NextCursor` drives "load more", and `Total` is
optional. `p.Search()` returns the grid's free-text term.

## Input validation

Two layers, both before your logic mutates anything:

1. **Schema** (`Route.Input`) - the gateway validates the request against the
   manifest schema (required-ness, types, options, visibility, regex/min/max).
2. **Bind tags** - `rc.Bind` decodes into your struct and runs `validate:"..."`
   struct tags.

Declare the schema once and reuse it for the action form and the route:

```go
func setSchema() *plugin.Schema {
    return &plugin.Schema{Groups: []plugin.Group{{Name: "Entry", Fields: []plugin.Field{
        {Key: "key", Label: "Key", Type: plugin.FieldText, Required: true},
        {Key: "value", Label: "Value", Type: plugin.FieldTextarea},
    }}}}
}
```

## Errors

Return one of the SDK sentinels (wrap with `fmt.Errorf("%w: ...", ...)` to add
context); the gateway maps them to HTTP status codes:

| Sentinel                 | Meaning                     |
| ------------------------ | --------------------------- |
| `plugin.ErrInvalidInput` | Bad request (400).          |
| `plugin.ErrNotFound`     | Missing resource (404).     |
| `plugin.ErrUnauthorized` | Not authenticated (401).    |
| `plugin.ErrForbidden`    | Not allowed (403).          |
| `plugin.ErrConflict`     | Conflict (409).             |
| `plugin.ErrUnavailable`  | Upstream unreachable (503). |
| `plugin.ErrNotSupported` | Capability not implemented. |

## File uploads & downloads

For multipart routes, declare a `FieldFile` in the input schema and read parts
with `rc.Uploads("field")`. To stream a download back, return a
`*plugin.Download` (set exactly one of `Body`, `Seeker`, or `OpenRange`; `Seeker`
gives the client range requests for free). Most plugins need neither.

## Saved commands, queries, and templates

There is no separate snippets API. Use the generic `rc.Storage` surface for
saved commands, saved queries, request templates, editor drafts, and per-plugin
preferences. If a plugin wants a domain-specific helper such as a snippet store,
build it as a small typed wrapper over `plugin.Storage` rather than adding a new
storage path.

See [storage.md](storage.md) for scopes, examples, conflict behavior, and tests.
