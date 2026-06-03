# Streaming: terminals, logs, channels, recording

Beyond request/response routes, a plugin can serve **interactive byte-streams** —
an SSH-style terminal, a live log tail, an exec session, a desktop. These ride
WebSocket routes and tracked channels. The gateway stays the byte-pump in the
middle, so streams are recorded and audited exactly like a built-in's.

This template doesn't stream (it returns `ErrNotSupported` from `OpenChannel`),
but here's the full shape.

## A WebSocket route

Set `Method: plugin.MethodWS` and provide `Stream` instead of `Handle`:

```go
{
    ID: "shell.session", Method: plugin.MethodWS, Path: "/shell",
    Permission: "shell.exec", Risk: plugin.RiskPrivileged, AuditEvent: "shell.session",
    Stream: shellStream,
},
```

The stream handler bridges the browser to the upstream:

```go
func shellStream(rc *plugin.RequestContext, client plugin.ClientStream) error {
    s := rc.Session.(*session)
    upstream, err := s.openShell(rc.Ctx) // an io.ReadWriteCloser to the target
    if err != nil {
        return err
    }
    defer upstream.Close()

    // Pump both directions until either side closes.
    go func() { _, _ = io.Copy(upstream, client) }() // browser → target
    _, err = io.Copy(client, upstream)                // target → browser
    return err
}
```

`plugin.ClientStream` is the browser side — an `io.ReadWriteCloser` plus a
`Context()` that's cancelled when the client disconnects. Watch it to tear down
the upstream cleanly:

```go
go func() {
    <-client.Context().Done()
    upstream.Close()
}()
```

App-level concerns (terminal resize, exit status) are just bytes/frames your
handler reads from the same stream — no extra wire surface.

## Declaring the stream in the manifest

Pair the route with a `Stream` entry (its `Kind` tells the UI how to render) and,
usually, a panel:

```go
Streams: []plugin.Stream{
    {ID: "shell.session", Kind: plugin.StreamTerminal, RouteID: "shell.session"},
},
Tabs: []plugin.Panel{
    {Key: "shell", Label: "Shell", Type: plugin.PanelTerminal,
     Source: &plugin.DataSource{RouteID: "shell.session"}},
},
```

Stream kinds: `StreamTerminal`, `StreamLogs`, `StreamDesktop`, `StreamMetrics`,
`StreamFile`. Panel types that consume them include `PanelTerminal`, `PanelLog`,
and `PanelRemote` (desktop).

## Tracked channels: `OpenChannel`

When you need an upstream stream that isn't a top-level WS route — a port-forward,
a secondary exec — implement `Session.OpenChannel`. The gateway pins the session
while the channel is open and bridges its bytes.

```go
func (s *session) OpenChannel(ctx context.Context, req plugin.ChannelRequest) (plugin.Channel, error) {
    conn, err := s.net.DialContext(ctx, "tcp", req.Params["addr"]) // egress via the gateway
    if err != nil {
        return nil, err
    }
    return &channel{conn: conn, kind: req.Kind}, nil
}
```

A `Channel` is an `io.ReadWriteCloser` plus `Kind() StreamKind`.

## Recording

Because the gateway is the byte-pump on every stream, it records any stream class
you declare — no plugin code. Declare what's recordable:

```go
Recording: []plugin.RecordingCapability{
    {Class: plugin.RecordingTerminal, Formats: []plugin.RecordingFormat{plugin.FormatAsciicastV2}, StreamIDs: []string{"shell.session"}},
},
```

For *authoritative* recording (e.g. asciinema frames the plugin itself emits),
ride the canonical frames on a declared server-stream and the gateway records
those. Operators control recording policy per connection; you only declare
what's possible.
