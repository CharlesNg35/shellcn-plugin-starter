# PanelSplit

Use `PanelSplit` to compose standard panels side by side or stacked: table plus
details, editor plus preview, logs plus terminal, or graph plus object details.

```go
plugin.Panel{
    Key: "workbench", Label: "Workbench", Icon: icon("panel-left"),
    Type: plugin.PanelSplit,
    Config: plugin.SplitConfig{
        Orientation: plugin.SplitHorizontal,
        Panels: []plugin.SplitPanel{
            {Panel: plugin.Panel{Key: "list", Type: plugin.PanelTable,
                Source: &plugin.DataSource{RouteID: "myplugin.items.list"},
                Config: plugin.TableConfig{Columns: columns()}}, Size: 40, MinSize: 25},
            {Panel: plugin.Panel{Key: "details", Type: plugin.PanelObjectDetail,
                Source: &plugin.DataSource{RouteID: "myplugin.item.read"},
                Config: plugin.ObjectDetailConfig{RawToggle: true}}, Size: 60},
        },
    },
}
```

Child panels are normal panels. They may use `VisibleWhen` to hide a side of the
split when the active row does not support it. Keep nesting shallow. If a split
becomes a custom application layout, reconsider whether standard workspace
resources and detail tabs would be clearer.
