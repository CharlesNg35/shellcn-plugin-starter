# PanelLogStream

Use `PanelLogStream` for live text output: container logs, pod logs, daemon logs,
tailing files, broker consumer output, or process stdout.

```go
Streams: []plugin.Stream{{ID: "demo.logs", Kind: plugin.StreamLogs, RouteID: "demo.logs"}},
plugin.Panel{
    Key: "logs", Label: "Logs", Icon: icon("scroll-text"),
    Type:   plugin.PanelLogStream,
    Source: &plugin.DataSource{RouteID: "demo.logs", Method: plugin.MethodWS, Params: map[string]string{"tail": "200"}},
}
```

Log streams are usually server-to-browser. The handler writes text or JSON log
frames to `plugin.ClientStream` and exits when `client.Context()` is done.

Do not declare logs as `StreamTerminal`. Terminal streams imply continuous
browser input handling and different keepalive behavior.
