# PanelTerminal

Use `PanelTerminal` for one interactive terminal, shell, exec session, or console
stream. The panel uses xterm.js and binds to a `StreamTerminal` route.

```go
Streams: []plugin.Stream{{ID: "myplugin.shell", Kind: plugin.StreamTerminal, RouteID: "myplugin.shell"}},
Tabs: []plugin.Panel{{
    Key: "shell", Label: "Shell", Icon: icon("terminal"),
    Type:   plugin.PanelTerminal,
    Source: &plugin.DataSource{RouteID: "myplugin.shell", Method: plugin.MethodWS, Params: map[string]string{"cols": "80", "rows": "24"}},
    Config: plugin.TerminalConfig{Zoom: true, Search: true},
}}
```

## Stream route

Declare a `MethodWS` route and pump browser input to the upstream channel and
upstream output back to the browser.

```go
func shell(rc *plugin.RequestContext, client plugin.ClientStream) error {
    ch, err := rc.Session.OpenChannel(rc.Ctx, plugin.ChannelRequest{
        Kind: plugin.StreamTerminal, Params: rc.Params(),
    })
    if err != nil {
        return err
    }
    defer ch.Close()

    errc := make(chan error, 2)
    go func() { _, e := io.Copy(client, ch); errc <- e }()
    go func() { errc <- plugin.CopyTerminalInput(ch, client) }()
    select {
    case <-client.Context().Done():
        return nil
    case err := <-errc:
        if err == io.EOF {
            return nil
        }
        return err
    }
}
```

If the channel implements `Resize(cols, rows int) error`, resize frames are
forwarded by `plugin.CopyTerminalInput`.

Use `RiskPrivileged` for shells and exec sessions. Declare recording capability
when terminal recording is supported.

## Theme

The terminal renderer itself is themed by ShellCN. Resize/control frames also
include the current workspace theme:

```json
{ "type": "resize", "cols": 120, "rows": 32, "theme": "dark" }
```

`plugin.CopyTerminalInput` consumes resize frames and applies only `cols`/`rows`
to channels implementing `plugin.Resizer`. If your terminal application needs
theme information, parse the control frames before forwarding input or wrap the
channel path intentionally. Most remote shells do not need this.

## Controls

`TerminalConfig.Controls` is the same `[]plugin.StreamControl` used by the
[log viewer](log-stream.md). A control re-parameterizes the session and
reconnects — e.g. a picker that switches which container a pod shell execs into:

```go
Config: plugin.TerminalConfig{
    Zoom: true, Search: true,
    Controls: []plugin.StreamControl{{
        Param: "container", Label: "Container",
        OptionsSource: &plugin.DataSource{RouteID: "myplugin.pod.containers"},
    }},
}
```

Changing a control resets the buffer and reconnects with the new param (the
handler reads it via `rc.Param("container")`). The picker lives in the terminal's
collapsible control overlay and is hidden entirely when there is only one option.
