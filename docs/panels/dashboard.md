# PanelDashboard

Use `PanelDashboard` to compose multiple generic panels into one overview grid.
It is useful for connection overviews, cluster summaries, and resource summary
tabs that need charts plus lists.

```go
dash := plugin.DashboardConfig{Cells: []plugin.Panel{
    {Key: "metrics", Label: "Metrics", Type: plugin.PanelMetrics, Span: 2,
        Source: &plugin.DataSource{RouteID: "myplugin.metrics", Method: plugin.MethodWS},
        Config: metricsConfig()},
    {Key: "tasks", Label: "Tasks", Type: plugin.PanelTable, Span: 2,
        Source: &plugin.DataSource{RouteID: "myplugin.tasks.list"},
        Config: plugin.TableConfig{Columns: taskColumns()}},
}}

plugin.Panel{Key: "overview", Label: "Overview", Type: plugin.PanelDashboard, Config: dash}
```

Dashboard cells are normal panels and follow their own route/config contracts.
`Span` is a sizing hint for grid width. A cell may set `VisibleWhen` to hide
itself when the active row does not support that view. Avoid nesting dashboards
inside dashboards.
