# Routes

A `Route` is one endpoint. It carries the metadata the gateway enforces *before*
your handler runs, and a handler that is pure business logic.

```go
{
    ID: "starter.set", Method: plugin.MethodPost, Path: "/entries",
    Permission: "starter.write", Risk: plugin.RiskWrite, AuditEvent: "starter.set",
    Input: setSchema(), Handle: set,
},
```

## Fields

| Field        | Purpose                                                                 |
| ------------ | ----------------------------------------------------------------------- |
| `ID`         | Stable handle referenced by panels/actions/manifest.                    |
| `Method`     | `MethodGet` / `Post` / `Put` / `Patch` / `Delete`, or `MethodWS`.       |
| `Path`       | Route path; `{name}` placeholders arrive via `rc.Param("name")`.        |
| `Permission` | RBAC permission string the gateway checks.                              |
| `Risk`       | `RiskSafe` / `RiskWrite` / `RiskDestructive` / `RiskPrivileged`.        |
| `AuditEvent` | Name recorded in the audit log for this call.                           |
| `Input`      | Optional `*Schema`; validated by the gateway before the handler runs.   |
| `Timeout`    | Optional per-request timeout.                                           |
| `Handle`     | The handler (`func(*RequestContext) (any, error)`).                     |
| `Stream`     | For `MethodWS` routes only - see [streaming.md](streaming.md).          |

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
- `rc.Audit(result, params, err)` - record an operation inside a long-lived
  route (see [streaming.md](streaming.md)).

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

| Sentinel                  | Meaning                          |
| ------------------------- | -------------------------------- |
| `plugin.ErrInvalidInput`  | Bad request (400).               |
| `plugin.ErrNotFound`      | Missing resource (404).          |
| `plugin.ErrUnauthorized`  | Not authenticated (401).         |
| `plugin.ErrForbidden`     | Not allowed (403).               |
| `plugin.ErrConflict`      | Conflict (409).                  |
| `plugin.ErrUnavailable`   | Upstream unreachable (503).      |
| `plugin.ErrNotSupported`  | Capability not implemented.      |

## File uploads & downloads

For multipart routes, declare a `FieldFile` in the input schema and read parts
with `rc.Uploads("field")`. To stream a download back, return a
`*plugin.Download` (set exactly one of `Body`, `Seeker`, or `OpenRange`; `Seeker`
gives the client range requests for free). Most plugins need neither.

## Snippets (saved commands)

The gateway offers a generic, user-owned store of saved commands/queries, scoped
per protocol. A handler reaches it through `rc.Snippets`, a `plugin.SnippetStore`
(`Create`/`Get`/`ListByOwner`/`Update`/`Delete` over `plugin.Snippet`). Scope by
`rc.User.ID` and your plugin `Name`. The SSH plugin uses this for saved shell
snippets. It's optional - ignore it if your protocol has no such concept.
