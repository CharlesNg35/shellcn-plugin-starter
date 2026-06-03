# Streaming: terminals, logs, channels, recording

Beyond request/response routes, a plugin can serve **interactive byte-streams** -
an SSH-style terminal, a live log tail, an exec session, a desktop. These ride
WebSocket routes and tracked channels. The gateway stays the byte-pump in the
middle, so streams are recorded and audited exactly like a built-in's.

This template doesn't stream (it returns `ErrNotSupported` from `OpenChannel`),
but here's the full shape, mirroring how the SSH/Docker/Kubernetes built-ins do it.

## A WebSocket route

Set `Method: plugin.MethodWS` and provide `Stream` instead of `Handle`:

```go
{
    ID: "shell.session", Method: plugin.MethodWS, Path: "/shell",
    Permission: "shell.exec", Risk: plugin.RiskPrivileged, AuditEvent: "shell.session",
    Stream: shellStream,
},
```

## The canonical terminal handler

A `StreamHandler` receives the browser side (`plugin.ClientStream`) and an
upstream channel from the session, then pumps both directions until either ends:

```go
func shellStream(rc *plugin.RequestContext, client plugin.ClientStream) error {
    ch, err := rc.Session.OpenChannel(rc.Ctx, plugin.ChannelRequest{
        Kind:   plugin.StreamTerminal,
        Params: rc.Params(), // e.g. initial cols/rows
    })
    if err != nil {
        return err
    }
    defer ch.Close()

    errc := make(chan error, 2)
    go func() { _, e := io.Copy(client, ch); errc <- e }() // upstream → browser
    go func() { _, e := io.Copy(ch, client); errc <- e }() // browser → upstream
    select {
    case <-client.Context().Done(): // browser disconnected
        return nil
    case err := <-errc:
        if err == io.EOF {
            return nil
        }
        return err
    }
}
```

Rules the built-ins follow:

- `defer ch.Close()` - never leak the upstream.
- Always `select` on `client.Context().Done()` so a browser disconnect tears the
  session down.
- Treat `io.EOF` as a clean exit, not an error.

### Resize and exit-status

These are just app-level frames on the same stream - no extra wire surface. The
SSH plugin reserves a control frame (a `0x00`-prefixed JSON `{type,cols,rows}`)
for resize and reads initial `cols`/`rows` from the query params. Define whatever
framing your protocol needs and handle it in the copy loop.

## `plugin.ClientStream`

The browser side is an `io.ReadWriteCloser` plus `Context()` that's cancelled on
disconnect. For a server-stream (logs, query results) you often just _write_ to
it - encode JSON events straight onto `client`:

```go
enc := json.NewEncoder(client)
for ev := range events {
    if err := enc.Encode(ev); err != nil {
        return err
    }
}
```

## Tracked channels: `Session.OpenChannel`

`OpenChannel` is where the session opens an upstream stream and the gateway pins
the session while it's open. Implement it to dial the target (through `cfg.Net`)
and return a `plugin.Channel` (an `io.ReadWriteCloser` + `Kind()`):

```go
func (s *session) OpenChannel(ctx context.Context, req plugin.ChannelRequest) (plugin.Channel, error) {
    switch req.Kind {
    case plugin.StreamTerminal:
        return s.openShell(ctx, req.Params) // returns a Channel wrapping the pty/exec stream
    default:
        return nil, plugin.ErrNotSupported
    }
}
```

Channel kinds: `StreamTerminal`, `StreamLogs`, `StreamDesktop`, `StreamMetrics`,
`StreamFile`.

## Declaring streams in the manifest

Pair each WS route with a `Stream` entry (its `Kind` tells the UI how to render)
and a panel:

```go
Streams: []plugin.Stream{
    {ID: "shell.session", Kind: plugin.StreamTerminal, RouteID: "shell.session"},
},
Tabs: []plugin.Panel{
    {Key: "shell", Label: "Shell", Type: plugin.PanelTerminal,
     Source: &plugin.DataSource{RouteID: "shell.session"}},
},
```

Panel types for streams: `PanelTerminal` (terminal), `PanelLogStream` (logs),
`PanelRemoteDesktop` (VNC/RDP), `PanelMetrics` (metric frames). A terminal panel
can opt into extras via `TerminalConfig{Zoom, Search}`.

## Recording

Because the gateway is the byte-pump on every stream, it records any stream class
you declare - no plugin code. Declare what's recordable, naming the stream IDs:

```go
Recording: []plugin.RecordingCapability{
    {
        Class:         plugin.RecordingTerminal,
        Formats:       []plugin.RecordingFormat{plugin.FormatAsciicastV2},
        StreamIDs:     []string{"shell.session"},
        Authoritative: true, // the canonical record rides this declared stream
    },
},
```

Terminal sessions record as `FormatAsciicastV2`; desktops as `FormatWebMCanvas`
(`RecordingDesktop`). Operators choose the recording _policy_ per connection
(off / on-demand / always); you only declare what's _possible_.

## Live lists (watch)

A table or resource list can update in place from a stream instead of
re-fetching. Point `TableConfig.Watch` (or `ResourceType.Watch`) at a WS route,
and have that stream handler emit `plugin.ResourceEvent` values onto the client:

```go
func watch(rc *plugin.RequestContext, client plugin.ClientStream) error {
    enc := json.NewEncoder(client)
    for ev := range changes { // your backend's change feed
        e := plugin.ResourceEvent{
            Type:     "modified", // "added" | "modified" | "deleted"
            Ref:      plugin.ResourceRef{Kind: "container", Name: ev.Name, UID: ev.ID},
            Resource: ev.Row,
        }
        if err := enc.Encode(e); err != nil {
            return err
        }
    }
    return nil
}
```

The gateway patches the grid row keyed by `Ref.UID`. This is how the container
and Kubernetes plugins show live status. The same `ResourceRef`/event shape backs
lazy tree expansion: a `TreeGroup.Source` (or a `TreeNode.ChildrenSource`) route
returns `plugin.TreeNode` values, and the renderer expands them on click.
