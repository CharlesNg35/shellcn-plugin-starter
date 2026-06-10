# Plugin storage

`rc.Storage` is the scoped persistence surface ShellCN exposes to plugin route
handlers. Use it for plugin-owned user data: saved queries, saved HTTP request
templates, snippets, local preferences, drafts, recent filters, WASM app state,
and small records that should survive a session restart.

Do not use plugin storage as a cache of live infrastructure state. Resource list
and watch routes should read the target system directly so ShellCN stays
accurate and auditable.

## Contract

```go
type Storage interface {
    Get(ctx context.Context, scope StorageScope, key string) (StorageItem, error)
    Put(ctx context.Context, collection string, item StorageItem) (StorageItem, error)
    Delete(ctx context.Context, scope StorageScope, key string) error
    List(ctx context.Context, scope StorageScope) ([]StorageItem, error)
}
```

The gateway resolves and enforces plugin id, user id, and connection id.
Plugins only provide a logical collection, an item key, and opaque bytes.

```go
type StorageItem struct {
    Key         string
    Value       []byte
    ContentType string
    Metadata    map[string]string
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

`Value` is opaque to the core. JSON is common, but the storage layer does not
inspect it. `Metadata` is for lightweight labels, grouping, or local sorting.
Do not duplicate owner id, plugin id, connection id, `CreatedAt`, or `UpdatedAt`
inside the JSON payload; the core owns those fields.

## How records are stored

Under the hood, ShellCN stores plugin records in the gateway database as rows
scoped by five identity columns plus the payload:

```text
collection | plugin | connection_id | owner_id | item_key | value | content_type | metadata | timestamps
```

Those scope columns are not supplied by the plugin:

- `plugin` is the active plugin id.
- `owner_id` is the authenticated ShellCN user.
- `connection_id` is the active ShellCN connection.
- `collection` is the logical bucket passed by the plugin.
- `item_key` is the record key passed by the plugin.

The primary identity of a stored row is:

```text
(collection, plugin, connection_id, owner_id, item_key)
```

This is deliberate. A plugin cannot read another plugin's records, cannot read
another user's records, and cannot choose a different connection scope by
constructing ids in user input. The route wrapper creates a storage bridge for
the current request/session and fills those scope values before the store is
called.

`Put` is an upsert for the current connection row. If the same
`collection/plugin/connection/owner/key` already exists, the value, content
type, metadata, and update timestamp are replaced. If it does not exist, a new
row is created. Plugins should therefore treat `Key` as the identity of one
logical saved object inside a collection.

The gateway owns timestamps. Use the returned `StorageItem.CreatedAt` and
`StorageItem.UpdatedAt` for display and sorting instead of storing duplicate
timestamp fields in the JSON value.

## Scopes

```go
plugin.ConnectionStorage("saved_queries")
plugin.UserStorage("saved_queries")
```

`ConnectionStorage(collection)` filters records to the current user, current
plugin, and current connection. This is the normal private scope for data that
belongs to one configured target.

`UserStorage(collection)` filters records to the current user and current plugin
across all of that user's connection rows for the same plugin. It intentionally
does not include `connection_id` in the read/list/delete filter.

Important write behavior:

- `Put(ctx, collection, item)` always writes a row for the current connection.
- `ConnectionStorage(collection)` reads/lists/deletes only that connection's
  rows.
- `UserStorage(collection)` reads/lists/deletes the current user's rows for that
  plugin and collection across connections.

There is no separate `PutUserStorage` call. If a plugin wants records to behave
as user-level data, it writes normal records with globally unique keys and reads
them back with `UserStorage`. That keeps writes anchored to the connection that
created the record while still allowing user-wide reuse.

Use connection scope for records that are tied to one target system:

- saved SQL for one database connection
- file browser bookmarks for one server
- query editor history for one cluster
- WASM app state that represents the current connection

Use user scope for records that should follow the user across connections of the
same plugin:

- reusable request templates
- snippets
- UI preferences
- shared connection profiles that do not contain secrets

`Collection` separates logical record groups inside one plugin. Use stable,
lowercase plural names such as `saved_queries`, `snippets`, `templates`,
`profiles`, or `todos`.

`Key` is the record identifier inside the collection. The storage API does not
expose prefix search. Model hierarchy in the JSON value or use generated ids.

For `UserStorage`, keys must be unique for that user/plugin/collection across
connections when you call `Get` or `Delete` by key. If two connection rows share
the same key, a user-scope `Get` or `Delete` by key returns
`plugin.ErrConflict` because the gateway cannot know which connection-owned row
you intended. Generate ids for user-level records unless there is a real natural
key.

`List(UserStorage(collection))` can return rows from multiple connections. If
the UI needs to show where a record came from, include that context in
`Metadata` or in the JSON value when the record is written. Do not rely on
plugins receiving raw database scope columns; they are intentionally hidden.

## Write a record

`Put` writes or replaces a record for the current user/plugin/connection.

```go
type savedQuery struct {
    Name  string `json:"name"`
    Query string `json:"query"`
}

func saveQuery(rc *plugin.RequestContext) (any, error) {
    var in savedQuery
    if err := rc.Bind(&in); err != nil {
        return nil, err
    }
    in.Name = strings.TrimSpace(in.Name)
    in.Query = strings.TrimSpace(in.Query)
    if in.Name == "" || in.Query == "" {
        return nil, fmt.Errorf("%w: name and query are required", plugin.ErrInvalidInput)
    }

    body, err := json.Marshal(in)
    if err != nil {
        return nil, err
    }
    id := uuid.NewString()
    item, err := rc.Storage.Put(rc.Ctx, "saved_queries", plugin.StorageItem{
        Key:         id,
        Value:       body,
        ContentType: "application/json",
        Metadata:    map[string]string{"name": in.Name},
    })
    if err != nil {
        return nil, err
    }
    return queryFromItem(item), nil
}
```

Use generated keys for records users can rename. Use stable natural keys only
when replacing the same logical record is the expected behavior.

## Read and list

```go
func listQueries(rc *plugin.RequestContext) (any, error) {
    rows, err := rc.Storage.List(rc.Ctx, plugin.ConnectionStorage("saved_queries"))
    if err != nil {
        return nil, err
    }

    out := make([]queryRow, 0, len(rows))
    for _, item := range rows {
        row, err := queryFromItem(item)
        if err != nil {
            return nil, err
        }
        out = append(out, row)
    }
    sort.Slice(out, func(i, j int) bool {
        return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
    })
    return plugin.Page[queryRow]{Items: out}, nil
}

func getQuery(rc *plugin.RequestContext) (any, error) {
    item, err := rc.Storage.Get(
        rc.Ctx,
        plugin.ConnectionStorage("saved_queries"),
        rc.Param("id"),
    )
    if err != nil {
        return nil, err
    }
    return queryFromItem(item)
}
```

Keep decode helpers small and strict:

```go
func queryFromItem(item plugin.StorageItem) (queryRow, error) {
    var value savedQuery
    if err := json.Unmarshal(item.Value, &value); err != nil {
        return queryRow{}, fmt.Errorf("%w: invalid saved query", plugin.ErrUnavailable)
    }
    return queryRow{
        Key:       item.Key,
        Name:      value.Name,
        Query:     value.Query,
        UpdatedAt: item.UpdatedAt,
    }, nil
}
```

Map malformed stored JSON to `plugin.ErrInvalidInput` only if the user can fix
it through the current request. Otherwise treat it as unavailable or corrupt
plugin data and return a wrapped `plugin.ErrUnavailable`.

## Delete

```go
func deleteQuery(rc *plugin.RequestContext) (any, error) {
    if err := rc.Storage.Delete(
        rc.Ctx,
        plugin.ConnectionStorage("saved_queries"),
        rc.Param("id"),
    ); err != nil {
        return nil, err
    }
    return map[string]any{"ok": true}, nil
}
```

Use a destructive route risk for user-visible deletes:

```go
plugin.Route{
    ID: "myplugin.query.delete", Method: plugin.MethodDelete, Path: "/queries/{id}",
    Permission: "myplugin.query.delete", Risk: plugin.RiskDestructive,
    AuditEvent: "myplugin.query.delete", Handle: deleteQuery,
}
```

## WASM storage pattern

WASM apps persist through bridge routes, not browser storage. The sandbox runs
with an opaque browser origin, so browser `localStorage`, cookies, and IndexedDB
are not the persistence model for plugin data. The durable state belongs in the
gateway store and is exposed through normal plugin routes.

```go
func listTodos(rc *plugin.RequestContext) (any, error) {
    rows, err := rc.Storage.List(rc.Ctx, plugin.ConnectionStorage("todos"))
    if err != nil {
        return nil, err
    }
    out := make([]todoItem, 0, len(rows))
    for _, item := range rows {
        row, err := todoFromItem(item)
        if err != nil {
            return nil, err
        }
        out = append(out, row)
    }
    return map[string]any{
        "items":     out,
        "total":     len(out),
        "remaining": countRemaining(out),
    }, nil
}
```

The WASM side calls `myplugin.todos.list` through the declared bridge. The route
handler owns validation, storage scope, and response shape. This keeps the WASM
app isolated from ShellCN internals while preserving the same authorization,
audit, and storage rules as every other panel.

## Where storage belongs in the UI

Storage is usually paired with generic panels:

- `PanelQueryEditor`: saved SQL, PromQL, LogQL, SurrealQL, or search bodies.
- `PanelHTTPClient`: saved request templates and header sets.
- `PanelCodeEditor`: saved manifests, scripts, snippets, or drafts.
- `PanelTable`: a list of saved user objects.
- `PanelForm`: plugin preferences or template metadata.
- `PanelWasm`: app state exposed through declared bridge routes.

The panel still talks to normal routes. The handler decides whether the data
comes from the target system or `rc.Storage`.

## Request context vs connect config

Prefer `rc.Storage` in route and stream handlers:

```go
func route(rc *plugin.RequestContext) (any, error) {
    return rc.Storage.List(rc.Ctx, plugin.ConnectionStorage("templates"))
}
```

`plugin.ConnectConfig.Storage` exists for session-level helpers that need
storage outside a request path. Keep it rare. Most plugin storage work should be
auditable through routes.

## Secrets

Never put secrets in plugin storage. Use connection `Secret` fields or reusable
credentials instead. Storage values are plugin-owned application data, not a
secret vault. Examples that must not go into storage:

- passwords
- API tokens
- private keys
- database connection strings with credentials
- kubeconfigs with embedded secrets

## Testing

Use a small fake `plugin.Storage` in handler tests. Keep it scoped like the real
API: `Put` writes by collection/key, `List` filters by `StorageScope`, and
`Get`/`Delete` return `plugin.ErrNotFound` for missing keys.

```go
type fakeStorage struct {
    items map[string]plugin.StorageItem
}

func (s *fakeStorage) Put(_ context.Context, collection string, item plugin.StorageItem) (plugin.StorageItem, error) {
    if s.items == nil {
        s.items = map[string]plugin.StorageItem{}
    }
    s.items[collection+"/"+item.Key] = item
    return item, nil
}
```

For scope behavior such as user-level conflict handling, test through the
gateway bridge or an integration-style helper. A fake is enough for most route
validation and response-shape tests.
