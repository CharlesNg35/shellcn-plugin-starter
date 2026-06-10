# PanelCanvas

Use `PanelCanvas` for custom server-driven drawing when standard panels cannot
represent the task: topology maps, protocol visualizers, simulators, games,
whiteboard-like tools, custom graph editors, or dense visual debuggers.

Do not use canvas for ordinary forms, tables, object details, timelines, or task
progress. Generic panels are more accessible, easier to test, and participate in
resource actions, export, loading, and empty/error states.

The detailed stream guide lives in [../streaming.md](../streaming.md#canvas-streams).

## Manifest pattern

Canvas panels bind to a `StreamCanvas` stream through a WebSocket data source.

```go
Streams: []plugin.Stream{
    {ID: "demo.canvas", Kind: plugin.StreamCanvas, RouteID: "demo.canvas"},
},
Tabs: []plugin.Panel{{
    Key:   "canvas",
    Label: "Canvas",
    Icon:  plugin.Icon{Type: plugin.IconLucide, Value: "scan"},
    Type:  plugin.PanelCanvas,
    Source: &plugin.DataSource{
        RouteID: "demo.canvas",
        Method:  plugin.MethodWS,
    },
    Config: plugin.CanvasConfig{
        ScaleMode:     plugin.CanvasScaleResize,
        Interactive:   true,
        Pointer:       true,
        Keyboard:      true,
        ResizeEvents:  true,
        WheelMode:     plugin.CanvasWheelModified,
        HiDPI:         true,
        Background:    "#0f172a",
        FocusOnPointer: true,
        AriaLabel:     "Topology canvas",
        Instructions:  "Inspect and move topology nodes.",
    },
}}
```

Set `Interactive` only when the stream actually consumes input. For display-only
canvases, leave `Pointer`, `Keyboard`, and `WheelMode` empty so the page remains
easy to scroll and navigate.

## Scale modes

- `CanvasScaleResize`: the canvas follows the panel size. Use it for dashboards,
  maps, live simulations, and responsive tools.
- `CanvasScaleFit`: the renderer fits a fixed logical drawing into the panel.
  Use it for game boards, diagrams, and visualizers with stable coordinates.
- `CanvasScaleScroll`: the drawing keeps its natural size and scrolls. Use it
  for large schematics or timeline-like visualizations.

`CanvasScaleFit` and `CanvasScaleScroll` require positive `Width` and `Height`.
For responsive panels, use `CanvasScaleResize` and leave fixed dimensions empty.

When using `CanvasScaleResize`, enable `ResizeEvents` and update the scene from
`ReadyEvent` and `ResizeEvent`.

## Input configuration

`Pointer` enables pointer events with coordinates and an optional region id.
`Keyboard` enables key events while the canvas is focused. `WheelMode` controls
how wheel events are captured:

- `CanvasWheelNone`: no wheel capture.
- `CanvasWheelCapture`: send every wheel event to the stream.
- `CanvasWheelModified`: send wheel events only when a modifier key is held.

Prefer `CanvasWheelModified` when the panel is inside a scrollable workspace.

## Stream loop pattern

Use `github.com/charlesng35/shellcn/sdk/plugin/canvas` for typed frames and
events. Avoid hand-built `map[string]any` canvas payloads.

```go
func canvasStream(_ *plugin.RequestContext, client plugin.ClientStream) error {
    scene := newScene()
    events := make(chan canvas.Event, 32)
    errc := make(chan error, 1)

    go func() {
        for {
            ev, err := canvas.DecodeEvent(client)
            if err != nil {
                errc <- err
                return
            }
            select {
            case events <- ev:
            case <-client.Context().Done():
                return
            }
        }
    }()

    ticker := time.NewTicker(33 * time.Millisecond)
    defer ticker.Stop()

    dirty := true
    for {
        if dirty {
            if err := canvas.WriteFrame(client, scene.Frame()); err != nil {
                return err
            }
            dirty = false
        }

        select {
        case <-client.Context().Done():
            return nil
        case err := <-errc:
            if errors.Is(err, io.EOF) {
                return nil
            }
            return err
        case ev := <-events:
            dirty = scene.Handle(ev)
        case <-ticker.C:
            dirty = scene.Animate()
        }
    }
}
```

This pattern keeps reads and writes from blocking each other, exits on context
cancel, and only redraws when the scene changes.

## Event handling

Handle the typed event values directly:

```go
func (s *scene) Handle(ev canvas.Event) bool {
    switch e := ev.(type) {
    case *canvas.ReadyEvent:
        s.theme = e.Theme
        s.width, s.height = e.Width, e.Height
        return true
    case *canvas.ResizeEvent:
        s.theme = e.Theme
        s.width, s.height = e.Width, e.Height
        return true
    case *canvas.PointerEvent:
        s.pointer = canvas.Point{X: e.X, Y: e.Y}
        if e.Event == canvas.PointerDown && e.RegionID != "" {
            s.dragID = e.RegionID
        }
        return true
    case *canvas.WheelEvent:
        s.zoom = clamp(s.zoom*(1-e.DeltaY*0.001), 0.5, 2.5)
        return true
    case *canvas.KeyEvent:
        return s.handleKey(e)
    }
    return false
}
```

Do not assume a pointer event always targets a region. Region ids are optional
and depend on the frame's `Regions` list.

## Theme

Canvas receives the current ShellCN theme in `ReadyEvent` and `ResizeEvent`.
The value is `plugin.PanelThemeLight` or `plugin.PanelThemeDark`.

```go
func (s *scene) colors() colors {
    if s.theme == plugin.PanelThemeLight {
        return colors{
            Background: "#f8fafc",
            Surface:    "#ffffff",
            Text:       "#0f172a",
            Border:     "#cbd5e1",
            Accent:     "#0284c7",
        }
    }
    return colors{
        Background: "#020617",
        Surface:    "#0f172a",
        Text:       "#e2e8f0",
        Border:     "#334155",
        Accent:     "#38bdf8",
    }
}
```

The frontend sends a new resize event when the workspace theme changes. Treat it
as a redraw signal. Do not make route behavior, storage keys, target selection,
or permissions depend on theme.

## Frames and regions

A frame contains drawing commands and optional interactive regions:

```go
func (s *scene) Frame() canvas.Frame {
    return canvas.Frame{
        Commands: []canvas.Command{
            canvas.Clear{Color: "#0f172a"},
            canvas.Rect{
                Paint: canvas.Paint{Fill: "#2563eb"},
                X: 48, Y: 48, Width: 240, Height: 120,
            },
            canvas.FillText{
                Paint: canvas.Paint{Fill: "#ffffff"},
                Text: "Gateway", X: 72, Y: 112,
            },
        },
        Regions: []canvas.Region{{
            ID:     "node:gateway",
            Shape:  canvas.RegionRect,
            X:      48,
            Y:      48,
            Width:  240,
            Height: 120,
            Cursor: "grab",
            Label:  "Gateway node",
        }},
    }
}
```

Regions are how the renderer maps pointer input and accessibility labels back to
logical scene objects. Keep ids stable while the object exists.

## Accessibility

Canvas content is otherwise opaque to assistive technology. Add:

- `AriaLabel` on the panel.
- `Instructions` when interaction is not obvious.
- Region `Label` values for clickable or draggable objects.
- `canvas.Announce` commands when important state changes.
- Keyboard alternatives for pointer-only operations when the panel is a primary
  workflow.

If you cannot provide meaningful labels or keyboard behavior, use a standard
panel instead.

## Performance

- Keep frame payloads small. Redraw only when dirty.
- Use `HiDPI` for precise visual tools; skip it for very large animated scenes
  if bandwidth or CPU becomes a problem.
- Coalesce high-frequency pointer movement before expensive recomputation.
- Prefer stable logical coordinates and transform commands over rebuilding large
  point arrays every tick.
- Stop goroutines when `client.Context()` is done.

## Testing checklist

- Stream exits cleanly on client context cancel and `io.EOF`.
- Fixed-size `fit`/`scroll` canvases declare positive `Width` and `Height`.
- `ReadyEvent`/`ResizeEvent` update the scene size.
- Pointer, wheel, and key events are ignored safely when the corresponding
  manifest flag is disabled or the region id is empty.
- Frames are produced through `canvas.WriteFrame`.
- Interactive regions have stable ids and labels.
- Theme changes redraw with readable light and dark palettes.
