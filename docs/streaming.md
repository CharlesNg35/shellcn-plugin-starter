# Streaming: terminals, logs, channels, recording

Beyond request/response routes, a plugin can serve **interactive byte-streams** -
an SSH-style terminal, a live log tail, an exec session, a desktop. These ride
WebSocket routes and tracked channels. The gateway stays the byte-pump in the
middle, so streams use the same recording, audit, auth, and transport wrapper as
ordinary plugin routes.

This template doesn't stream (it returns `ErrNotSupported` from `OpenChannel`),
but the full shape is:

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
    go func() { _, e := io.Copy(client, ch); errc <- e }()      // upstream → browser
    go func() { errc <- plugin.CopyTerminalInput(ch, client) }() // browser → upstream
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

Rules for stream handlers:

- `defer ch.Close()` - never leak the upstream.
- Always `select` on `client.Context().Done()` so a browser disconnect tears the
  session down.
- Treat `io.EOF` as a clean exit, not an error.

### Resize (the `CopyTerminalInput` helper)

The terminal panel sends resize events in-band: a frame starting with a `0x00`
byte carries JSON (`{"type":"resize","cols":N,"rows":N}`); everything else is
keystrokes. Rather than parse that yourself, use the SDK helper for the
browser→upstream half:

```go
go func() { errc <- plugin.CopyTerminalInput(ch, client) }()
```

`plugin.CopyTerminalInput` writes keystrokes to the channel and, on a resize
frame, calls `ch.Resize(cols, rows)` **if the channel implements
`plugin.Resizer`** (`Resize(cols, rows int) error`). So all you do is implement
`Resize` on your channel (see above); the helper does the framing. Read the
initial `cols`/`rows` from `rc.Params()`. If you ever need the raw values,
`plugin.ParseResizeControl(frame)` decodes one. Exit status is just bytes your
handler writes before the stream closes - no extra wire surface.

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

### Resizable terminals and desktop init (optional channel methods)

A `Channel` can carry two control capabilities by simply adding methods. The
gateway detects them and wires them up:

```go
// Terminal/exec: let the browser resize the pty.
func (c *shellChannel) Resize(cols, rows int) error { return c.pty.Setsize(cols, rows) }

// Desktop (VNC/RDP): the one-time server-init blob the client needs to start.
func (c *desktopChannel) ServerInit() []byte { return c.serverInit }
```

If your channel implements `Resize(cols, rows int) error` (the `plugin.Resizer`
interface), browser resize events reach it - whether they arrive through
`plugin.CopyTerminalInput` in your stream handler or are forwarded by the gateway
for a tracked channel. If it implements `ServerInit() []byte`, the gateway hands
that blob to the client when the screen opens. Channels without these methods are
unaffected - a plain logs channel stays plain.

## Declaring streams in the manifest

Pair each WS route with a `Stream` entry (its `Kind` tells the UI how to render)
and a panel:

```go
Streams: []plugin.Stream{
    {ID: "shell.session", Kind: plugin.StreamTerminal, RouteID: "shell.session"},
},
Tabs: []plugin.Panel{
    {Key: "shell", Label: "Shell", Type: plugin.PanelTerminalGrid,
     Source: &plugin.DataSource{RouteID: "shell.session"},
     Config: plugin.TerminalGridConfig{MaxPanes: 6, Zoom: true, Search: true}},
},
```

Panel types for streams: `PanelTerminal` (single terminal), `PanelTerminalGrid`
(user-managed terminal splits), `PanelLogStream` (logs), `PanelRemoteDesktop`
(VNC/RDP), `PanelMetrics` (metric frames), `PanelTaskProgress` (task
status/progress frames), and `PanelCanvas` (plugin-driven drawing with optional
pointer/keyboard/wheel input). Terminal panels can opt into extras via
`TerminalConfig{Zoom, Search}` or `TerminalGridConfig{MaxPanes, DefaultPanes,
Zoom, Search}`.

`PanelWasm` is not itself a stream panel. It is a sandboxed browser-side WASM
program that may open only the routes and streams declared in
`WasmConfig.Bridge`. Use it when the UI must run inside the browser, such as a
portable simulation, game, or WASM-powered visualizer. Keep ordinary long-lived
terminal/log/query/metrics flows on the native stream panels, and keep custom
server-driven drawing on `PanelCanvas`.

### Stream kind semantics and keepalive

Declare the stream `Kind` by how the browser and handler actually behave, not by
the upstream protocol name. The gateway uses the generic kind for rendering,
recording, and transport policy:

- `StreamTerminal`, `StreamDesktop`, and `StreamCanvas` are interactive streams.
  Their handlers must keep a browser-to-upstream read loop running for input,
  resize, mouse, pointer, wheel, or keyboard frames. The gateway may send
  WebSocket ping keepalives on these streams because pong frames are processed by
  that active reader.
- `StreamLogs`, `StreamMetrics`, `StreamFile`, and `StreamTask` are
  server-to-browser streams. Their handlers often only write events to the
  browser. Do not declare a log, watch, metrics feed, task, or long-running query
  as `StreamTerminal` just because it uses a WebSocket; a keepalive ping could
  time out if no handler is reading from the browser.

If you add a custom bidirectional stream shape, keep the same invariant in mind:
only streams with a continuous client-read loop should use terminal/desktop-style
transport behavior.

### Canvas streams

`PanelCanvas` is a controlled draw/input protocol, not plugin-owned frontend UI.
Use the typed SDK structs in `github.com/charlesng35/shellcn/sdk/plugin/canvas`
instead of ad-hoc `map[string]any` payloads:

```go
import "github.com/charlesng35/shellcn/sdk/plugin/canvas"

func canvasStream(rc *plugin.RequestContext, client plugin.ClientStream) error {
    if err := canvas.WriteFrame(client, canvas.Frame{
        Commands: []canvas.Command{
            canvas.Clear{Color: "#020617"},
            canvas.Rect{
                Paint: canvas.Paint{Fill: "#2563eb"},
                X: 24, Y: 32, Width: 160, Height: 44,
                Radii: &canvas.Radii{TopLeft: 12, TopRight: 12, BottomRight: 6, BottomLeft: 6},
            },
            canvas.Text{
                Paint: canvas.Paint{Fill: "#ffffff", Font: "600 16px Inter, sans-serif"},
                X: 48, Y: 59, Text: "Click me",
            },
        },
        Regions: []canvas.Region{
            canvas.RectRegion("primary", 24, 32, 160, 44, canvas.WithCursor("pointer"), canvas.WithLabel("Primary action")),
        },
    }); err != nil {
        return err
    }

    for {
        ev, err := canvas.DecodeEvent(client)
        if err != nil {
            return err
        }
        if pointer, ok := ev.(*canvas.PointerEvent); ok && pointer.RegionID == "primary" {
            _ = canvas.WriteFrame(client, canvas.Frame{
                Commands: []canvas.Command{
                    canvas.Clear{Color: "#052e16"},
                    canvas.Text{X: 24, Y: 40, Text: "Clicked"},
                },
            })
        }
    }
}
```

On the wire these structs become JSON frames such as `{ "type": "clear" }`,
`{ "type": "rect" }`, or `{ "commands": [...] }`. The browser sends typed JSON
events back, including `ready`, `resize`, `pointer`, `wheel`, and `key`. Plugins
can draw custom controls and handle hit testing themselves, or declare rectangular
`regions` so returned pointer events include a `regionId`. Use
`canvas.RawCommand` only as an explicit escape hatch for extension commands that
the current SDK does not model yet.

The typed command set covers partial clears (`canvas.Clear{X, Y, Width,
Height}`), per-corner rounded rectangles (`canvas.Radii`), path fill rules,
`canvas.Text`, `canvas.FillText`, `canvas.StrokeText`, enhanced
`canvas.TextBox` layout, images, gradients, patterns, snapshots, cursor changes,
focused regions, and screen-reader announcements. Image opacity is controlled by
the embedded `canvas.Paint.Alpha` field on `canvas.Image`; there is no separate
image-only opacity flag.

Use `CanvasConfig{Interactive, Pointer, Keyboard, WheelMode, ResizeEvents}` to
opt into input channels. Prefer responsive canvases that fit the available panel
when the visual can adapt to the reported `ready`/`resize` dimensions.

Use `ScaleMode` for sizing:

- `CanvasScaleResize`: the plugin receives the viewport size as `width` and
  `height` and redraws responsively.
- `CanvasScaleFit`: the plugin declares `Width` and `Height`; the browser scales
  that logical surface into the available panel and maps pointer/wheel
  coordinates back to the logical coordinate system. Use `MaxScale` when the
  surface should not upscale beyond its designed size.
- `CanvasScaleScroll`: the plugin declares `Width` and `Height`; the panel
  scrolls a larger 1:1 surface for dense maps, whiteboards, timelines,
  dependency graphs, and linked-node diagrams.

Ready and resize events include logical `width`/`height`, `viewportWidth`,
`viewportHeight`, `scale`, `dpr`, and `theme`.

Use `WheelMode` instead of a boolean:

- `CanvasWheelAuto`: default behavior. Wheel input is captured for
  non-scroll interactive canvases and left to the browser for scroll-mode
  canvases.
- `CanvasWheelCapture`: always send wheel events to the plugin for intentional
  zoom/pan surfaces.
- `CanvasWheelModified`: send wheel events only when Alt, Ctrl, or Meta is held;
  ordinary mouse-wheel scrolling keeps working.
- `CanvasWheelNone`: disable wheel input for interactive surfaces that do not
  need it.

Common canvas configuration recipes:

| Surface type                                                              | Recommended config                                                                                                                                                                                                | Why                                                                                                 |
| ------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------- |
| Responsive dashboard, live chart, status board                            | `ScaleMode: plugin.CanvasScaleResize`, `ResizeEvents: true`, `WheelMode: plugin.CanvasWheelNone`                                                                                                                  | The plugin can redraw to the actual viewport; no wheel input is needed.                             |
| Game, simulator, drawing tool with keyboard/pointer control               | `ScaleMode: plugin.CanvasScaleResize`, `Interactive: true`, `Keyboard: true`, `Pointer: true`, `WheelMode: plugin.CanvasWheelCapture` only if wheel is part of gameplay/tooling                                   | The app owns the interaction loop and should adapt to the available panel.                          |
| Fixed artboard, preview, report page, compact visual debugger             | `Width`/`Height`, `ScaleMode: plugin.CanvasScaleFit`, optional `MaxScale: 1`, `Pointer: true` when interactive                                                                                                    | The coordinate system stays stable while the browser scales the view into the panel.                |
| Large topology, map, timeline, whiteboard, dependency graph, canvas table | `Width`/`Height`, `ScaleMode: plugin.CanvasScaleScroll`, `WheelMode: plugin.CanvasWheelModified` when zoom/pan shortcuts are needed                                                                               | The surface is naturally larger than the viewport, so normal scrolling must remain available.       |
| Form-like controls rendered in canvas                                     | Prefer `PanelForm`; if canvas is required, use `ScaleMode: plugin.CanvasScaleFit`, `Interactive: true`, `Keyboard: true`, `Pointer: true`, `WheelMode: plugin.CanvasWheelNone`, plus `FocusRegion` and `Announce` | Generic form panels are more accessible; custom canvas controls need explicit accessibility events. |

If a canvas stream is interactive, the handler must continuously read from the
client stream while writing frames.

### WebAssembly panels

`PanelWasm` runs a declared WASM entrypoint in a core-owned sandboxed iframe. A
plugin declares assets and bridge permissions; it does not inject raw HTML or
JavaScript into the ShellCN app.

```go
plugin.WasmConfig{
    Entry:   "app.wasm",
    Runtime: plugin.WasmRuntimeGo,
    Boot:   plugin.WasmBoot{Scripts: []string{"wasm_exec.js"}},
    Assets: []plugin.WasmAsset{
        {Path: "wasm_exec.js", MIME: "text/javascript",
            Source: plugin.DataSource{RouteID: "demo.asset", Params: map[string]string{"path": "wasm_exec.js"}}},
        {Path: "app.wasm", MIME: "application/wasm",
            Source: plugin.DataSource{RouteID: "demo.asset", Params: map[string]string{"path": "app.wasm"}}},
    },
    Bridge: plugin.WasmBridge{
        Routes:  []plugin.WasmBridgeRoute{{RouteID: "demo.state.get", Method: plugin.MethodGet}},
        Streams: []plugin.WasmBridgeStream{{RouteID: "demo.events"}},
    },
}
```

Use `Runtime: plugin.WasmRuntimeGeneric` when the WASM artifact is not a Go
`wasm_exec.js` program. This is common for Rust frameworks such as Leptos or
Yew, wasm-bindgen outputs, Emscripten builds, and other toolchains that generate
JavaScript glue. In that shape, declare the compiled WASM as `Entry`, declare the
generated `app.js` or your own `boot.js` in both `Boot.Scripts` and `Assets`, and
declare any additional generated files in `Assets`.

For simple C/C++/Rust modules that can be instantiated with an empty import
object and export `_start` or `main`, leave `Boot.Scripts` empty. ShellCN will
instantiate `Entry` directly. Add boot scripts only when the toolchain needs
generated imports, runtime glue, or framework setup.

Generic boot scripts run inside the sandbox before the entrypoint is
instantiated. They should load generated WASM/data bytes with
`window.shellcn.asset(window.shellcn.entry)` and pass those bytes to the
framework loader. Do not depend on relative URLs, `document.currentScript.src`,
cookies, localStorage, or same-origin fetches; the iframe intentionally has an
opaque origin. Do not point `Entry` at a fake or placeholder file just because a
framework has a boot script. `Entry` is still the real primary WASM artifact.

Inside the sandbox, the app calls `window.shellcn.route(routeId, body, options)`,
`window.shellcn.stream(routeId, params)`, and `window.shellcn.asset(path)`.
The renderer rejects calls to undeclared routes, wrong methods, undeclared
streams, and undeclared assets. Declare `Capabilities` only for browser features
the app actually needs, such as keyboard, pointer, fullscreen, pointer lock, or
gamepad.

The sandbox also receives the ShellCN theme through `window.shellcn.theme`
(`"light"` or `"dark"`). For live changes, call
`window.shellcn.onTheme((theme) => { ... })`; the parent posts theme updates
through the same bridge, so the WASM app does not need same-origin access or
parent DOM reads. See [panel theming](panels/theming.md) and
[PanelWasm](panels/wasm.md) for the full bridge API.

The sandbox intentionally omits `allow-same-origin`. The app runs with an opaque
origin and communicates with the host only through the declared `postMessage`
bridge. Do not design a WASM app that needs cookies, localStorage, IndexedDB, or
direct access to the parent DOM.

Leave `Width` and `Height` empty for a normal full-panel app. Use
`ScaleMode: plugin.WasmScaleScroll` when the app has naturally taller content
and should scroll inside the sandbox. Declare `Width` and `Height` together only
when the WASM app has a fixed logical viewport that should be fitted or scrolled
as a surface.

### Split terminal workspaces

`PanelTerminalGrid` is a renderer-owned workspace for SSH-style shells. A plugin
does not declare one stream per pane; it declares one `StreamTerminal` route, and
the browser opens a separate channel for each split pane through that same source.
Keep the handler stateless per stream open so two panes do not accidentally share
PTY state.

Recording is intentionally conservative. Mandatory terminal recording disables
split workspaces. Manual recording is hidden once the workspace is split, so the
gateway never creates misleading per-pane recordings. Use `PanelTerminal` when a
connection must always expose a recordable single terminal view.

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
