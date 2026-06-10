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

The source route should return trace data with spans, timings, hierarchy, and
service names. `ServiceField` tells the renderer which span field carries the
service label.

Use `PanelTimeline` for event history and `PanelGraph` for service topology.
Use `PanelTrace` only for actual trace/span waterfalls.
