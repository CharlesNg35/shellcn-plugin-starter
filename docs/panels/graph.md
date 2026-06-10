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

The source route returns a graph object with stable node ids and edge endpoints:

```json
{
  "nodes": [
    {
      "id": "svc:api",
      "label": "API",
      "group": "service",
      "summary": "Public gateway",
      "fields": [{ "name": "port", "type": "int", "key": "8080" }],
      "properties": { "namespace": "prod" }
    }
  ],
  "edges": [
    { "source": "svc:api", "target": "db:main", "label": "queries" }
  ]
}
```

Keep labels short and include enough metadata for the detail drawer. Edge
endpoints must reference node ids in the same payload.

`Exportable` is a pointer: nil means client-side image export is available. Set
it to `false` only when graph content is sensitive.

Use `ExpandRouteID` when a node can lazy-load neighbors. The expand route should
be read-only and bounded. It receives the selected node id in `ExpandParam`
(`node` by default) and returns another graph object. The renderer merges new
node ids and edge ids into the current graph.
