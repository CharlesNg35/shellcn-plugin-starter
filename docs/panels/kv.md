# PanelKV

Use `PanelKV` for key/value protocols where the primary workflow is browse key,
read value, edit value, delete key. For richer row data, use `PanelTable`.

```go
plugin.Panel{
    Key: "keys", Label: "Keys", Icon: icon("key-round"),
    Type:   plugin.PanelKV,
    Source: &plugin.DataSource{RouteID: "myplugin.keys.list"},
    Config: plugin.KVConfig{
        CreateRouteID: "myplugin.key.create",
        ReadRouteID:   "myplugin.key.read",
        WriteRouteID:  "myplugin.key.write",
        DeleteRouteID: "myplugin.key.delete",
        KeyParam:      "key",
        Writable:      true,
        ValueTypes:    []string{"string", "json", "binary"},
    },
}
```

The list route returns keys, usually as a `plugin.Page`:

```json
{
  "items": [
    { "key": "feature:alpha", "type": "json", "ttl": 3600, "size": 128 }
  ],
  "total": 1
}
```

The renderer uses `KeyParam` to pass the selected key to read, write, create,
and delete routes. If `KeyParam` is empty, it defaults to `key`.

Read routes receive the key as a route/query param and return the editable
detail:

```json
{ "key": "feature:alpha", "type": "json", "value": { "enabled": true } }
```

Write routes are called with `PUT`, key params, and this body:

```json
{ "key": "feature:alpha", "type": "json", "value": "{\"enabled\":false}" }
```

Create routes are also called with `PUT`. Delete routes are called with
`DELETE` and an empty body.

Keep values bounded for preview. For large binary values, return a download
route or use a file-like panel instead.
