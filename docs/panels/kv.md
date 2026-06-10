# PanelKV

Use `PanelKV` for key/value protocols where the primary workflow is browse key,
read value, edit value, delete key. For richer row data, use `PanelTable`.

```go
plugin.Panel{
    Key: "keys", Label: "Keys", Icon: icon("key-round"),
    Type:   plugin.PanelKV,
    Source: &plugin.DataSource{RouteID: "demo.keys.list"},
    Config: plugin.KVConfig{
        CreateRouteID: "demo.key.create",
        ReadRouteID:   "demo.key.read",
        WriteRouteID:  "demo.key.write",
        DeleteRouteID: "demo.key.delete",
        KeyParam:      "key",
        Writable:      true,
        ValueTypes:    []string{"string", "json", "binary"},
    },
}
```

The list route returns keys, usually as a `plugin.Page`. Read/write/delete routes
use `KeyParam` to receive the selected key.

Keep values bounded for preview. For large binary values, return a download
route or use a file-like panel instead.
