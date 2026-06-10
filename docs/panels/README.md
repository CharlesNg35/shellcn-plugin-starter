# Panel reference

Panels are the frontend contract of a plugin. A plugin does not ship Vue,
JavaScript, HTML, or CSS. It declares a `PanelType`, optional `DataSource`, and
typed `Config`; the ShellCN renderer chooses the PrimeVue/xterm/noVNC/editor
component and calls the declared routes.

Every panel follows the same shape:

```go
plugin.Panel{
    Key:    "main",
    Label:  "Main",
    Icon:   icon("table"),
    Type:   plugin.PanelTable,
    Source: &plugin.DataSource{RouteID: "demo.list"},
    Config: plugin.TableConfig{Columns: columns()},
}
```

`Source.RouteID` references one of your `Routes()`. `Source.Params` may use
resource interpolation such as `${resource.uid}`, `${resource.name}`,
`${resource.namespace}`, and `${resource.scope}` inside resource detail tabs.

## Choose the standard panel first

| Task | Preferred panel |
| --- | --- |
| Collections, rows, child objects | [Table](table.md) |
| Create/update form | [Form](form.md) or action input schema |
| Structured properties | [Object detail](object-detail.md) |
| Raw JSON/YAML/document readout | [Document](document.md) |
| Editable JSON/YAML/text/script | [Code editor](code-editor.md) |
| Server-side before/after comparison | [Diff](diff.md) |
| SQL, PromQL, LogQL, search, command consoles | [Query editor](query-editor.md) |
| File systems and object stores | [File browser](file-browser.md) |
| Live numbers and charts | [Metrics](metrics.md) |
| Overview grid of child panels | [Dashboard](dashboard.md) |
| Events, history, jobs, audits | [Timeline](timeline.md) |
| Long-running operation progress | [Task progress](task-progress.md) |
| Shell/exec | [Terminal](terminal.md) or [Terminal grid](terminal-grid.md) |
| Live text stream | [Log stream](log-stream.md) |
| VNC/RDP/RFB screen | [Remote desktop](remote-desktop.md) |
| Topology or relationships | [Graph](graph.md) |
| Distributed trace waterfall | [Trace](trace.md) |
| Key/value stores | [KV](kv.md) |
| REST-like request builder | [HTTP client](http-client.md) |
| Side-by-side generic composition | [Split](split.md) |
| Custom server-driven drawing | [Canvas](canvas.md) |
| Sandboxed browser-side WASM app | [WASM](wasm.md) |
| Agent enrollment | [Enroll](enroll.md) |

Use `PanelCanvas` and `PanelWasm` only when a standard panel cannot represent
the task. Standard panels provide validation, accessibility, keyboard behavior,
theming, export, empty states, and consistent action handling.

## Theme-aware custom surfaces

Standard panels are themed by the core renderer. Plugins only need to handle
theme directly when they draw their own surface:

- `PanelCanvas` receives `theme` in `ReadyEvent` and `ResizeEvent`.
- `PanelWasm` receives `window.shellcn.theme` and `window.shellcn.onTheme`.
- `PanelTerminal` resize/control frames include `theme`.

See [Panel theming](theming.md) for exact payloads and examples.

## Route method rules

- Read panels normally use `MethodGet`.
- Stream panels use `MethodWS` and must also declare a matching `Stream`.
- Mutating panels call routes named in their config, such as
  `CodeEditorConfig.SaveRouteID` or `TableConfig.Insert`.
- Action-opened panels (`OpenDialog`, `OpenDock`) still use normal panel config.

The manifest validator rejects stream panels that point at non-WS routes and
config route IDs that do not exist.

## Data rules

- List-like routes return `plugin.Page[T]`.
- Stream-like routes write frames to `plugin.ClientStream`.
- Resource rows should include a `ref` field when row/detail actions need stable
  identity.
- Errors should wrap an SDK sentinel such as `plugin.ErrInvalidInput`,
  `plugin.ErrNotFound`, or `plugin.ErrUnavailable`.
