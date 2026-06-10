# PanelDiff

Use `PanelDiff` for a read-only comparison where the plugin can produce both
sides server-side: current vs dry-run, live vs backup, planned vs actual, or
generated before/after DDL.

```go
plugin.Panel{
    Key: "preview", Label: "Preview", Icon: icon("git-compare"),
    Type:   plugin.PanelDiff,
    Source: &plugin.DataSource{RouteID: "demo.change.preview", Params: map[string]string{"id": "${resource.uid}"}},
    Config: plugin.DiffConfig{
        Language:      "yaml",
        OriginalField: "current",
        ModifiedField: "proposed",
        OriginalLabel: "Current",
        ModifiedLabel: "Proposed",
        Mode:          plugin.DiffSideBySide,
    },
}
```

## Source route

Return an object containing the configured fields:

```json
{
  "current": "apiVersion: ...",
  "proposed": "apiVersion: ..."
}
```

If `OriginalField` or `ModifiedField` is omitted, document and test the exact
shape your panel expects. Explicit fields are clearer.

## When not to use it

- Do not use it for ordinary object inspection. Use `PanelObjectDetail` or
  `PanelDocument`.
- Do not add a diff tab just for unsaved editor changes. `PanelCodeEditor`
  already covers local edit review.
