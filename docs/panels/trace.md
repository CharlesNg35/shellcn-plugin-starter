# PanelTrace

Use `PanelTrace` for distributed trace or span waterfall data. It is intended
for observability plugins such as Jaeger, Tempo, or OpenTelemetry backends.

```go
plugin.Panel{
    Key: "trace", Label: "Trace", Icon: icon("route"),
    Type:   plugin.PanelTrace,
    Source: &plugin.DataSource{RouteID: "demo.trace.read", Params: map[string]string{"id": "${resource.uid}"}},
    Config: plugin.TraceConfig{ServiceField: "service"},
}
```

The source route returns trace data with spans, timings, hierarchy, and service
names:

```json
{
  "traceId": "7f2f",
  "spans": [
    {
      "id": "root",
      "name": "GET /v1/items",
      "service": "api",
      "startTime": "2026-06-10T12:00:00Z",
      "durationMs": 42.5,
      "status": "ok",
      "tags": { "http.method": "GET" }
    },
    {
      "id": "db",
      "parentId": "root",
      "name": "SELECT items",
      "service": "postgres",
      "startMs": 12,
      "durationMs": 8.1
    }
  ]
}
```

`id` and `durationMs` are required for useful rendering. `parentId` builds the
waterfall hierarchy. `startTime` may be an ISO timestamp or number; `startMs`
may be used for relative offsets. `ServiceField` tells the renderer which span
field carries the service label when the payload uses a custom field name.

Use `PanelTimeline` for event history and `PanelGraph` for service topology.
Use `PanelTrace` only for actual trace/span waterfalls.
