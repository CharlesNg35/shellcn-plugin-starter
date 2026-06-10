# PanelRemoteDesktop

Use `PanelRemoteDesktop` for VNC/RDP/RFB-style interactive desktops. The panel is
backed by noVNC-compatible browser rendering and a `StreamDesktop` route.

```go
Streams: []plugin.Stream{{ID: "myplugin.desktop", Kind: plugin.StreamDesktop, RouteID: "myplugin.desktop"}},
plugin.Panel{
    Key: "desktop", Label: "Desktop", Icon: icon("monitor"),
    Type:   plugin.PanelRemoteDesktop,
    Source: &plugin.DataSource{RouteID: "myplugin.desktop", Method: plugin.MethodWS},
    Config: plugin.RemoteDesktopConfig{Resize: true, Clipboard: true},
}
```

The stream must bridge framebuffer/server bytes and browser input for the life
of the connection. If your channel needs a one-time init blob, implement:

```go
func (c *desktopChannel) ServerInit() []byte { return c.serverInit }
```

Use `RiskPrivileged` unless the desktop is demonstrably read-only. Declare
desktop recording when available.
