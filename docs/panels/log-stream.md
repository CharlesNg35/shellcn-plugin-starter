# PanelLogStream

Use `PanelLogStream` for live text output: container logs, pod logs, daemon logs,
tailing files, broker consumer output, or process stdout.

```go
Streams: []plugin.Stream{{ID: "myplugin.logs", Kind: plugin.StreamLogs, RouteID: "myplugin.logs"}},
plugin.Panel{
    Key: "logs", Label: "Logs", Icon: icon("scroll-text"),
    Type:   plugin.PanelLogStream,
    Source: &plugin.DataSource{RouteID: "myplugin.logs", Method: plugin.MethodWS, Params: map[string]string{"tail": "200"}},
}
```

Log streams are usually server-to-browser. The handler writes text or JSON log
frames to `plugin.ClientStream` and exits when `client.Context()` is done.

Do not declare logs as `StreamTerminal`. Terminal streams imply continuous
browser input handling and different keepalive behavior.

## Controls and previous logs (`LogStreamConfig`)

`LogStreamConfig` adds a manifest-driven control bar above the viewer:

```go
Config: plugin.LogStreamConfig{
    Controls: []plugin.StreamControl{{
        Param: "container", Label: "Container",
        OptionsSource: &plugin.DataSource{RouteID: "myplugin.pod.containers"},
    }},
    AllowPrevious: true,
}
```

A `StreamControl` is a generic selector that re-parameterizes the stream — the
same type is reused by `PanelTerminal` and `PanelFileBrowser`:

- `Param` — the source param the selected value is written to. The stream
  reconnects with it, so the handler reads it via `rc.Param("container")`.
- `Label` — placeholder / aria-label for the selector.
- `OptionsSource` — a read route returning `[]plugin.Option` (`{label, value}`);
  the first option is selected by default. The selector is hidden when the route
  returns one option or fewer, so single-choice streams stay clean.

`AllowPrevious` shows a "Previous" toggle that re-streams with `previous=true`
(read it via `rc.Param("previous") == "true"`) — typically the last terminated
instance's logs. Return a friendly message when no previous instance exists
instead of erroring.
