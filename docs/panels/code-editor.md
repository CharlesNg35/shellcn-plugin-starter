# PanelCodeEditor

Use `PanelCodeEditor` for editable text, JSON, YAML, SQL, scripts, manifests,
and request bodies. It can be used as a tab or as an action-opened dialog.

```go
plugin.Panel{
    Key: "editor", Label: "Editor", Icon: icon("code"),
    Type:   plugin.PanelCodeEditor,
    Source: &plugin.DataSource{RouteID: "myplugin.document.read", Params: map[string]string{"id": "${resource.uid}"}},
    Config: plugin.CodeEditorConfig{
        Language:    "json",
        SaveRouteID: "myplugin.document.update",
        SaveMethod:  plugin.MethodPut,
        SaveParams:  map[string]string{"id": "${resource.uid}"},
    },
}
```

## Read route

The source route returns the current content. It may return a string, bytes, or a
JSON object. Keep the route read-only and safe.

## Save route

By default the save route receives:

```json
{ "content": "..." }
```

Use `SaveBodyKey` when the backend route expects a different field:

```go
Config: plugin.CodeEditorConfig{
    Language:    "json",
    SaveRouteID: "myplugin.mapping.update",
    SaveMethod:  plugin.MethodPut,
    SaveBodyKey: "mapping",
}
```

Then the body is:

```json
{ "mapping": { "properties": {} } }
```

When `SaveBodyKey` is set, the renderer parses the editor text as JSON and puts
the parsed value under that key. Use it for JSON structured update routes. Leave
`SaveBodyKey` empty when the backend expects raw text in `content`.

`SaveExtra` adds fixed fields to the save body. Validate the body again in the
handler with `rc.Bind`.

## Action dialogs

For create/update dialogs, define an action with `Open: plugin.OpenDialog`,
`Panel: plugin.PanelCodeEditor`, and a `CodeEditorConfig` containing
`InitialContent` and save route fields.

```go
plugin.Action{
    ID: "myplugin.document.create", Label: "Create document", Icon: icon("plus"),
    RouteID: "myplugin.document.create", Open: plugin.OpenDialog,
    Panel: plugin.PanelCodeEditor,
    Config: plugin.CodeEditorConfig{
        Language: "json", InitialContent: "{\n  \"id\": \"example\"\n}",
        SaveRouteID: "myplugin.document.create", SaveMethod: plugin.MethodPost,
        SaveBodyKey: "document",
    },
}
```

## Diff

The code editor already gives users a local changed-buffer diff before saving.
Use `PanelDiff` only for server-produced comparisons.
