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

## Canonical refresh after save (`RefreshField`)

After a save the editor's baseline (the buffer it diffs unsaved edits against) is
stale: the server may have normalized, defaulted, or reformatted the document.
Set `RefreshField` to a key in the **save response** whose value is the canonical
content as a string; the editor resets both its buffer and baseline to it.

```go
Config: plugin.CodeEditorConfig{
    Language:     "json",
    SaveRouteID:  "myplugin.document.update",
    SaveMethod:   plugin.MethodPut,
    RefreshField: "content",
}
```

The save handler re-reads the stored object and returns the content already
serialized the same way the read route renders it (the editor shows objects as
2-space-indented JSON, so marshal with `json.MarshalIndent(doc, "", "  ")`):

```go
func documentUpdate(rc *plugin.RequestContext) (any, error) {
    // ...write, then read back the canonical object...
    out, err := json.MarshalIndent(doc, "", "  ")
    if err != nil {
        return nil, err
    }
    return map[string]any{"content": string(out)}, nil
}
```

The value must be a **string**; a non-string (or absent) `RefreshField` leaves the
baseline at the editor's current text. Use this only when the write is
synchronous and the re-read is canonical — skip it for async/eventually-applied
writes (e.g. task-queued updates) where a re-read would be stale.

## Live editing (`Watch`) and Preview (`DryRunKey`)

`Watch` points at a `StreamResource` WS route that pushes the current content; the
editor live-updates while clean and shows a non-destructive "changed on server"
notice when there are unsaved edits. With `RefreshField` set, `DryRunKey` enables
a Preview: the save body is re-sent with that key set to `true`, and the returned
`RefreshField` content is diffed against the live baseline so the user previews the
server's would-be result before committing. Only set `DryRunKey` when the save
route honors a dry-run flag (validate, don't persist).

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
