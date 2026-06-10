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
