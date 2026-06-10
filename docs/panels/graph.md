# PanelGraph

Use `PanelGraph` for node/edge relationships: service topology, dependency
maps, database relationships, cluster links, index aliases, or network graphs.

```go
plugin.Panel{
    Key: "relations", Label: "Relationships", Icon: icon("workflow"),
    Type:   plugin.PanelGraph,
    Source: &plugin.DataSource{RouteID: "demo.graph", Params: map[string]string{"id": "${resource.uid}"}},
    Config: plugin.GraphConfig{
        Layout:        plugin.GraphLayoutGrid,
        FitView:       true,
        ExpandRouteID: "demo.graph.expand",
        ExpandParam:   "node",
    },
}
```

The source route should return a graph object with stable node ids and edge
endpoints. Keep labels short and include enough metadata for tooltips/details.

`Exportable` is a pointer: nil means client-side image export is available. Set
it to `false` only when graph content is sensitive.

Use `ExpandRouteID` when a node can lazy-load neighbors. The expand route should
be read-only and bounded.
