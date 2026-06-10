# PanelTerminalGrid

Use `PanelTerminalGrid` for SSH-like workflows where users should split a shell
workspace into multiple panes. It still declares one stream route; each pane
opens an independent stream through the same source.

```go
plugin.Panel{
    Key: "shell", Label: "Shell", Icon: icon("terminal"),
    Type:   plugin.PanelTerminalGrid,
    Source: &plugin.DataSource{RouteID: "demo.shell", Method: plugin.MethodWS},
    Config: plugin.TerminalGridConfig{
        MaxPanes: 6, DefaultPanes: 1, Zoom: true, Search: true,
    },
}
```

Declare the stream route in `Streams()`:

```go
plugin.Stream{ID: "demo.shell", Kind: plugin.StreamTerminal, RouteID: "demo.shell"}
```

The stream handler must be stateless per open stream. Do not share PTYs or
command state across panes.

Use `PanelTerminal` instead when a connection must always expose one clearly
recordable terminal. Mandatory terminal recording disables split workspaces so
recordings are not misleading.
