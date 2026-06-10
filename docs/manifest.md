# Manifest

The `Manifest` is your plugin's single declarative contract. The gateway reads
it once at load, validates it, and the frontend renders **everything** from it -
there is no per-plugin UI code. Most of building a plugin is writing a good
manifest, so this is the longest chapter.

```go
type Manifest struct {
    APIVersion  int
    Name        string
    Version     string
    Title       string
    Description string
    Icon        Icon
    Category    Category

    Config          Schema               // connection-form fields
    Capabilities    []Capability         // declarative feature tags
    CredentialKinds []CredentialKindInfo // reusable credential kinds you own

    SupportedTransports []Transport
    Agent               *AgentProfile    // required iff you support TransportAgent

    Layout    Layout
    Tabs      []Panel        // workspace panels
    Tree      []TreeGroup    // sidebar roots (LayoutSidebarTree)
    Resources []ResourceType // managed object types (list + detail)
    Actions   []Action       // buttons bound to routes
    Streams   []Stream       // WS routes that back live panels

    HeaderActions []string         // Action IDs shown in the workspace header
    Scope         []ScopeFilter    // global selectors injected into requests
    Recording     []RecordingCapability
}
```

Everything below is optional except identity, `Layout`, and at least one
`SupportedTransports` entry - declare only what your plugin uses.

## Identity

| Field         | Notes                                                                                                                                                                                         |
| ------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `APIVersion`  | Always `plugin.CurrentAPIVersion`. A mismatch is refused at load.                                                                                                                             |
| `Name`        | Unique, lowercase id (`redis`, `acme-db`). Must match `[a-z][a-z0-9_-]*`; no dots, spaces, slashes, uppercase, or leading digits. Stored on every connection - don't change it after release. |
| `Version`     | Your plugin's own version string (bump per release).                                                                                                                                          |
| `Title`       | Human label in the catalog and workspace.                                                                                                                                                     |
| `Description` | One line in the protocol picker.                                                                                                                                                              |
| `Icon`        | See [Icon](#icon).                                                                                                                                                                            |
| `Category`    | Groups the protocol in the picker (see [Category](#category)).                                                                                                                                |

### Icon

```go
plugin.Icon{Type: plugin.IconLucide, Value: "database"}
```

`Type` is `IconLucide` (a [Lucide](https://lucide.dev) name, kebab-case),
`IconURL`, `IconBase64` (data URI), `IconEmoji`, or `IconSVG` (inline markup,
sanitized before render). Icons appear throughout - the manifest, panels, tree
nodes, actions, and scope filters all take one.

### Category

One of: `CategoryShell`, `CategoryFiles`, `CategoryContainers`,
`CategoryOrchestration`, `CategoryVirtualization`, `CategoryRemoteDesktop`,
`CategoryDatabases`, `CategorySearch`, `CategoryObservability`,
`CategoryMessaging`, `CategoryNetwork`, `CategoryCloud`, `CategoryDevOps`,
`CategorySecurity`, `CategoryLookup`, `CategoryOther`.

## Connection form: `Config`

`Config` is a `Schema` - ordered `Groups` of typed `Field`s - that becomes the
connection-form inputs (host, port, credentials, options) and the validation
contract for the saved config. Leave it empty if your plugin needs none.

```go
Config: plugin.Schema{Groups: []plugin.Group{{
    Name: "Connection",
    Fields: []plugin.Field{
        {Key: "host", Label: "Host", Type: plugin.FieldText, Required: true},
        {Key: "port", Label: "Port", Type: plugin.FieldNumber, Default: 6379},
        {Key: "tls",  Label: "Use TLS", Type: plugin.FieldToggle},
        {
            Key: "ca", Label: "CA certificate", Type: plugin.FieldTextarea,
            VisibleWhen: &plugin.Condition{AllOf: []plugin.Rule{
                {Field: "tls", Op: plugin.OpEq, Value: true},
            }},
        },
        {Key: "password", Label: "Password", Type: plugin.FieldPassword, Secret: true},
    },
}}},
```

### Field

| Field                                                                               | Purpose                                                                                  |
| ----------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------- |
| `Key`                                                                               | Config key; how you read it at runtime (`cfg.String("host")`).                           |
| `Label`                                                                             | Form label.                                                                              |
| `Type`                                                                              | Widget (see below).                                                                      |
| `Required`                                                                          | Validated server-side before save and before route handlers.                             |
| `Secret`                                                                            | Encrypted at rest; never returned to the client. Use for keys/passwords.                 |
| `Default`                                                                           | Pre-filled value.                                                                        |
| `Placeholder` / `Help`                                                              | Hints shown in the form.                                                                 |
| `Options`                                                                           | Static choices for select/radio/multiselect.                                             |
| `OptionsSource`                                                                     | A `*DataSource` to populate choices from a route at form-open.                           |
| `Credential`                                                                        | A `*CredentialSelector` for `FieldCredentialRef` (stores only the chosen credential id). |
| `VisibleWhen`                                                                       | A `*Condition` - show the field only when other values match.                            |
| `Validators`                                                                        | Server-side checks (`min`/`max`/`regex`/`oneOf`).                                        |
| `Step`                                                                              | Increment for number/slider.                                                             |
| `Fields` / `Item` / `MinItems` / `MaxItems` / `ItemLabel` / `AddLabel` / `KeyLabel` | Composite shapes - see [composite fields](#composite-fields).                            |

Field types: `FieldText`, `FieldTextarea`, `FieldPassword`, `FieldEmail`,
`FieldURL`, `FieldTel`, `FieldNumber`, `FieldStepper`, `FieldSlider`,
`FieldToggle`, `FieldSelect`, `FieldRadio`, `FieldMultiSelect`,
`FieldAutocomplete`, `FieldJSON`, `FieldDuration`, `FieldFile`,
`FieldCredentialRef`, and the composites `FieldObject`, `FieldArray`, `FieldMap`.

### Conditions (`VisibleWhen`)

A `Condition` is `AllOf`/`AnyOf` lists of `Rule{Field, Op, Value}` evaluated
against the other field values. A field with a `VisibleWhen` is shown (and
required/validated) only when its condition holds. Operators: `OpEq`, `OpNeq`,
`OpIn`, `OpNin`, `OpEmpty`, `OpNotEmpty`.

Besides field keys, two context keys are available so a field can depend on the
chosen **transport** or the protocol: `SchemaContextTransport` (`$transport`) and
`SchemaContextProtocol` (`$protocol`).

**Transport-conditional fields.** This is the most common use: when a connection
runs through an agent, the agent supplies the endpoint, so the host/port/socket
fields should disappear. Define a condition once and reuse it across fields:

```go
func configSchema() plugin.Schema {
    // Only when the user picks the direct transport.
    directOnly := plugin.Condition{AllOf: []plugin.Rule{
        {Field: plugin.SchemaContextTransport, Op: plugin.OpEq, Value: string(plugin.TransportDirect)},
    }}
    // Combine rules: direct transport AND a specific endpoint_type value.
    directTCP := plugin.Condition{AllOf: []plugin.Rule{
        {Field: plugin.SchemaContextTransport, Op: plugin.OpEq, Value: string(plugin.TransportDirect)},
        {Field: "endpoint_type", Op: plugin.OpEq, Value: "tcp"},
    }}
    return plugin.Schema{Groups: []plugin.Group{{Name: "Connection", Fields: []plugin.Field{
        {Key: "endpoint_type", Label: "Endpoint", Type: plugin.FieldSelect, Default: "unix",
         VisibleWhen: &directOnly, Options: []plugin.Option{{Label: "Socket", Value: "unix"}, {Label: "TCP", Value: "tcp"}}},
        {Key: "host", Label: "Host", Type: plugin.FieldText, Required: true, VisibleWhen: &directTCP},
        {Key: "port", Label: "Port", Type: plugin.FieldNumber, Default: 2375, VisibleWhen: &directTCP},
    }}}}
}
```

A hidden field is not required and not validated, so gating a `Required` field
behind a `VisibleWhen` is safe. The renderer hides the field live as the user
toggles the transport or another value; the server applies the same rules when it
validates the saved config.

### Validators

```go
Validators: []plugin.Validator{
    {Type: plugin.ValidatorMin, Value: 1},
    {Type: plugin.ValidatorMax, Value: 65535},
    {Type: plugin.ValidatorRegex, Value: `^[a-z0-9-]+$`, Message: "lowercase, digits, dashes"},
},
```

### Reusable credentials (`FieldCredentialRef`)

Instead of inline secret fields, a connection can reference a **reusable
credential** the user manages once. Declare a `credential_ref` field constrained
by kind:

```go
{
    Key: "credential", Label: "Credential", Type: plugin.FieldCredentialRef,
    Credential: &plugin.CredentialSelector{
        Kinds:    []plugin.CredentialKind{plugin.CredentialDBPassword},
        Required: true,
    },
},
```

Built-in kinds: `CredentialDBPassword`, `CredentialAPIToken`,
`CredentialBasicAuth`, `CredentialBearerToken`, `CredentialTLSClientCert`,
`CredentialCloudAccessKey`. To define your own kind, list `CredentialKindInfo`
entries in `Manifest.CredentialKinds`:

```go
CredentialKinds: []plugin.CredentialKindInfo{{
    Kind:          "acme_api_key",
    Label:         "ACME API key",
    SecretLabel:   "API key",   // labels the secret input in the credential form
    IdentityLabel: "Key ID",    // optional non-secret identity (e.g. username/key id)
}},
```

Reference it from a `credential_ref` field's `CredentialSelector{Kinds: ...}`. The
field stores only the credential id; the gateway resolves and injects the secret,
which you read with `cfg.CredentialSecretFor(...)` (see
[sessions.md](sessions.md)). The client never sees it.

### Composite fields

- `FieldObject` nests `Fields` (a sub-form).
- `FieldArray` repeats `Item` (a list of values/objects; `MinItems`/`MaxItems`/`AddLabel`).
- `FieldMap` is repeatable key/value rows whose value type is `Item`
  (`KeyLabel`/`KeyPlaceholder`).

## Workspace layout

`Layout` arranges the connection workspace:

| Layout              | Use for                                                |
| ------------------- | ------------------------------------------------------ |
| `LayoutTabs`        | A flat tab bar, one `Panel` at a time (most plugins).  |
| `LayoutSidebarTree` | A resource `Tree` on the left + a detail pane.         |
| `LayoutDashboard`   | A grid of panels (from `Tabs`) shown at once.          |
| `LayoutSingle`      | One full-bleed panel (a terminal/desktop/file screen). |

### Choosing a layout by protocol shape

Pick the layout from how your protocol is navigated, not from its category. Most
plugins map cleanly onto four shapes:

| Your protocol is...                                                        | Layout              | Typical panels                                                      |
| -------------------------------------------------------------------------- | ------------------- | ------------------------------------------------------------------- |
| **One screen** (a terminal workspace, file tree, or desktop)               | `LayoutSingle`      | one `PanelTerminalGrid` / `PanelFileBrowser` / `PanelRemoteDesktop` |
| **A few flat views** (terminal + files, or browse + admin)                 | `LayoutTabs`        | a handful of `Tabs`                                                 |
| **A big hierarchy** (databases->tables, namespaces->pods, topics->partitions) | `LayoutSidebarTree` | `Tree` + `Resources` with `DetailView`s                             |
| **An at-a-glance board** (several charts/tables at once)                   | `LayoutDashboard`   | `Tabs` as dashboard cells                                           |

Rules of thumb:

- A pure **terminal**, **desktop**, or **single file tree** is `LayoutSingle` -
  no tab bar, just the screen.
- A **shell with extras** (terminal + a file browser + saved snippets) is
  `LayoutTabs` - the ssh plugin is the canonical example.
- Anything you **explore** - a database with schemas and tables, an orchestrator
  with many resource kinds, a broker with topics - is `LayoutSidebarTree`: a lazy
  `Tree` on the left, a `ResourceType` per object kind, and a `DetailView` opened
  on click. This is where most non-trivial plugins land.
- A small **key/value or status** protocol (e.g. redis) is fine as `LayoutTabs`;
  reach for `LayoutSidebarTree` only once the object count makes a tree worth it.

```go
// One screen (a file manager):
Layout: plugin.LayoutSingle,
Tabs:   []plugin.Panel{{Key: "files", Type: plugin.PanelFileBrowser, Source: &plugin.DataSource{RouteID: "fs.list"}}},

// Explorer (databases -> tables): a tree plus resource types with detail views.
Layout:    plugin.LayoutSidebarTree,
Tree:      []plugin.TreeGroup{{Key: "databases", Label: "Databases", Source: plugin.DataSource{RouteID: "db.tree"}}},
Resources: []plugin.ResourceType{ /* "table": list columns + DetailView with a data + query panel */ },
```

## Panels (`Tabs`)

A `Panel` is one screen. It has a `Key`, `Label`, `Icon`, a `Type`, an optional
`Source` (`*DataSource` binding it to a route), and a `Config` shaped for the
type.

```go
Tabs: []plugin.Panel{{
    Key: "entries", Label: "Entries", Icon: icon("list"),
    Type:   plugin.PanelTable,
    Source: &plugin.DataSource{RouteID: "starter.list"},
    Config: plugin.TableConfig{ /* ... */ },
}},
```

### Panel types and their config

| `PanelType`          | Config type           | Renders                            |
| -------------------- | --------------------- | ---------------------------------- |
| `PanelTable`         | `TableConfig`         | A data grid (optionally editable). |
| `PanelForm`          | `FormPanelConfig`     | A submit form.                     |
| `PanelTerminal`      | `TerminalConfig`      | One xterm terminal (WS route).     |
| `PanelTerminalGrid`  | `TerminalGridConfig`  | A split terminal workspace.        |
| `PanelLogStream`     | -                     | A live log tail (WS route).        |
| `PanelQueryEditor`   | `QueryEditorConfig`   | A SQL/query editor + results.      |
| `PanelFileBrowser`   | `FileBrowserConfig`   | A file manager.                    |
| `PanelCodeEditor`    | `CodeEditorConfig`    | A CodeMirror editor.               |
| `PanelDiff`          | `DiffConfig`          | A read-only before/after diff.     |
| `PanelMetrics`       | `MetricsConfig`       | KPI cards, gauges, time-series.    |
| `PanelGraph`         | `GraphConfig`         | A node/edge graph.                 |
| `PanelTrace`         | `TraceConfig`         | A distributed-trace view.          |
| `PanelKV`            | `KVConfig`            | A key/value browser.               |
| `PanelHTTPClient`    | `HTTPClientConfig`    | A REST client.                     |
| `PanelRemoteDesktop` | `RemoteDesktopConfig` | A VNC/RDP screen.                  |
| `PanelDocument`      | -                     | Rendered document/markdown.        |
| `PanelDashboard`     | `DashboardConfig`     | A grid of nested panels (`Cells`). |
| `PanelObjectDetail`  | `ObjectDetailConfig`  | A structured property sheet.       |
| `PanelTimeline`      | `TimelineConfig`      | Events, tasks, or audit history.   |
| `PanelTaskProgress`  | `TaskProgressConfig`  | A streamed long-running task.      |
| `PanelSplit`         | `SplitConfig`         | Resizable child panel composition. |
| `PanelCanvas`        | `CanvasConfig`        | Plugin-driven canvas draw/input.   |
| `PanelWasm`          | `WasmConfig`          | A sandboxed WebAssembly app.       |
| `PanelEnroll`        | -                     | Core-owned agent enrollment screen. |

Each panel has a dedicated route and payload contract in the
[panel reference](panels/README.md). Start there before choosing a panel. The
SDK types are still the source of truth for field names, but the panel docs
explain which route method to use, what the route should return, and common
mistakes.

Use the most structured panel that fits the data:

- Use `PanelObjectDetail` for typed fields, copy buttons, badges, redaction, and
  optional raw JSON. Prefer it over `PanelDocument` for object properties.
- Use `PanelTimeline` for Kubernetes events, Docker events, audit trails, task
  history, or database activity feeds returned by a list route.
- Use `PanelTaskProgress` for cancellable/retryable long-running jobs that stream
  status/progress frames.
- Use `PanelGraph` for dependency maps, topology, ER diagrams, and relationship
  views. Graph image export is enabled by default when `GraphConfig.Exportable`
  is nil/omitted/null; set it to a `*bool` containing `false` only for sensitive
  graphs that should not expose the PNG/JPEG/SVG export menu.
- Use `PanelDiff` only when the route can return both sides of a comparison,
  such as current YAML vs dry-run output, current document vs proposed
  replacement, or generated SQL before/after DDL. Do not use it for ordinary
  inspect tabs; `PanelObjectDetail` is better for current object state.
- Use `PanelTerminalGrid` when users should be able to split a terminal workspace
  themselves. It still binds to one `StreamTerminal` route; each pane opens its own
  channel through the same manifest source.
- Use `PanelSplit` to compose generic panels side by side, such as table +
  details, editor + preview, or logs + terminal. Do not create plugin-specific
  layouts for those cases.
- `PanelEnroll` is core-owned for agent-mode connection setup. Do not normally
  add it to a plugin manifest. Declare `SupportedTransports` plus
  `AgentProfile.Install` and let the workspace render enrollment when the agent
  tunnel is offline; see [panels/enroll.md](panels/enroll.md) and
  [agents.md](agents.md).
- Use `PanelCanvas` for custom visual or game-like tools that genuinely need a
  plugin-controlled drawing surface: topology maps with custom interaction,
  simulators, whiteboard-like tools, visual debuggers, or games. The plugin sends
  typed SDK canvas frames from
  `github.com/charlesng35/shellcn/sdk/plugin/canvas` and receives typed
  pointer/keyboard/wheel/resize events; it never ships JavaScript or DOM. Do not
  use canvas for ordinary tables, forms, settings, confirmations, or object
  details because the generic panels are more accessible, theme-consistent,
  searchable, and easier to validate.
- Choose a canvas sizing mode deliberately. Use
  `CanvasConfig{ScaleMode: plugin.CanvasScaleResize}` when the plugin can redraw
  responsively from the reported `ready`/`resize` dimensions. Use
  `CanvasConfig{Width, Height, ScaleMode: plugin.CanvasScaleFit}` for fixed
  logical coordinate systems that should scale down into the available panel
  while pointer events still report logical coordinates. Use
  `CanvasConfig{Width, Height, ScaleMode: plugin.CanvasScaleScroll}` for dense
  topology maps, whiteboards, timelines, dependency graphs, or linked-node
  diagrams where the content is naturally larger than the available viewport and
  the user should pan by normal scrolling. `MaxScale` can cap fit-mode upscaling
  when a surface should not grow beyond its designed size.
- Use `CanvasConfig.WheelMode` to control mouse-wheel behavior:
  `CanvasWheelAuto` captures wheel only for non-scroll interactive canvases,
  `CanvasWheelCapture` always sends wheel events to the plugin,
  `CanvasWheelModified` sends wheel events only with Alt/Ctrl/Meta held so
  ordinary scrolling still works, and `CanvasWheelNone` disables wheel input.
  Use `CanvasWheelNone` for passive charts and dashboards,
  `CanvasWheelModified` for optional zoom/pan in editors and maps, and
  `CanvasWheelCapture` only when wheel gestures are core to the canvas
  experience.
- Canvas frames should use the typed `sdk/plugin/canvas` helpers for richer
  primitives: partial clears, per-corner `Radii`, path fill rules, enhanced
  text boxes, explicit fill/stroke text, cursor changes, focus regions,
  announcements, images, gradients, patterns, snapshots, and SDK region builders.
  Image opacity is expressed through the embedded `canvas.Paint.Alpha` field.
- Use `PanelWasm` only when an isolated browser-side program is the right tool:
  games, heavy simulations, portable visualizers, or WASM libraries that cannot
  be represented cleanly with the standard panels or the streamed canvas
  protocol. The plugin still does not ship arbitrary ShellCN frontend code. It
  declares `WasmConfig{Entry, Assets, Boot, Bridge}`; every asset is loaded
  through a read route, every route/stream bridge is allow-listed in the
  manifest, and the gateway runs the app in a sandboxed iframe without
  `allow-same-origin`. Do not use WASM for tables, forms, settings, CRUD, object
  details, logs, terminals, or ordinary charts that generic panels already
  handle.
- Leave `Width` and `Height` empty for a normal full-panel WASM app. Use
  `WasmScaleScroll` for naturally taller content. Declare `Width` and `Height`
  together only for a fixed logical viewport that must be fitted or scrolled as
  a surface.
- For Go WASM, set `Runtime: plugin.WasmRuntimeGo`, declare `app.wasm` as the
  `Entry`, and include `wasm_exec.js` in `Boot.Scripts` and `Assets`. The parent
  renderer fetches the bytes through authenticated plugin routes and posts them
  into the sandbox; the WASM app uses `window.shellcn.route`,
  `window.shellcn.stream`, and `window.shellcn.asset` for declared bridge access.
  Read `window.shellcn.theme` and subscribe with `window.shellcn.onTheme(fn)` so
  custom DOM/canvas/WebGL rendering follows ShellCN light and dark mode.
- For Rust, Leptos, Yew, wasm-bindgen, Emscripten, TinyGo without the Go runtime,
  or any framework that emits JavaScript glue, use
  `Runtime: plugin.WasmRuntimeGeneric`. The `Entry` is still the WASM binary
  ShellCN validates and serves through `window.shellcn.asset`, but the framework
  normally needs an `app.js`, `boot.js`, or similar loader in `Boot.Scripts`.
  Declare both the JS loader and every referenced WASM/data file in `Assets`.
  The host exposes the declared entry path as `window.shellcn.entry`; the boot
  script should load bytes through
  `window.shellcn.asset(window.shellcn.entry)` instead of relying on relative
  URLs, `document.currentScript.src`, cookies, or same-origin fetches. Do not
  use placeholder entries such as `noop.wasm`; `Entry` should be the real main
  WASM artifact.
- For simple generic WASM, such as a small C/C++/Rust module that can be
  instantiated with an empty import object and exports `_start` or `main`, omit
  `Boot.Scripts`; ShellCN will instantiate `Entry` directly. Add boot scripts
  only when the toolchain needs generated imports or runtime setup.

The gateway projects the SDK panel-config schema to the browser. Unknown config
keys and wrong types are rejected by the generic renderer, so keep panel configs
closed and typed rather than passing ad-hoc maps.

### DataSource

A panel (or column source, watch, action) binds to a route by id:

```go
&plugin.DataSource{RouteID: "starter.list", Method: plugin.MethodGet, Params: map[string]string{"scope": "${scope.region}"}}
```

The gateway resolves `RouteID` + `Params` to a URL. Params interpolate from the
active resource (`${resource.uid}`, `${resource.namespace}`), scope filters
(`${scope.<param>}`), or static values.

### TableConfig (the workhorse)

```go
plugin.TableConfig{
    Columns: []plugin.Column{
        {Key: "key", Label: "Key", Sortable: true},
        {Key: "size", Label: "Size", Type: plugin.ColumnBytes},
        {Key: "state", Label: "State", Type: plugin.ColumnBadge,
         Severities: map[string]plugin.Severity{"running": plugin.SeveritySuccess, "down": plugin.SeverityDanger}},
    },
    ActionIDs:    []string{"starter.set"},    // toolbar buttons
    RowActionIDs: []string{"starter.delete"}, // per-row buttons (implies row selection)
    DefaultSort:  &plugin.SortKey{Field: "key"},
    Exportable:   true,                       // allow CSV/JSON export of loaded rows
    // Editable grids: set RowKey + Insert/Update/Delete DataSources.
}
```

`Column.Type` selects a cell renderer: `ColumnText`, `ColumnBadge`,
`ColumnBytes`, `ColumnDateTime`, `ColumnNumber`, `ColumnPercent`, `ColumnBool`,
`ColumnJSON`, `ColumnIcon`. Other notable `TableConfig` fields: `RefreshIntervalMs`
(poll-and-replace), `Watch` (a stream that patches rows), `Editable`+`RowKey`+
`Insert`/`Update`/`Delete` (inline editing), `StagedEdits`, `EmptyText`. Column
`Key`s must match the JSON field names your list route returns.

For an **editable grid**, set `Editable: true`, name the primary-key column(s) in
`RowKey`, and point `Insert`/`Update`/`Delete` at mutation routes; the gateway
sends each edited row as JSON to those routes. Mutation handlers must revalidate
table names, primary keys, writable columns, and backend permissions server-side.

## Actions

An `Action` binds a button to a route. With an `Input` schema on the route it
opens a form; otherwise it fires immediately.

```go
Actions: []plugin.Action{
    {ID: "starter.set", Label: "Set entry", Icon: icon("plus"), RouteID: "starter.set"},
    {
        ID: "starter.delete", Label: "Delete", Icon: icon("trash-2"), RouteID: "starter.delete",
        Params:      map[string]string{"key": "${resource.uid}"},
        Confirm:     true, ConfirmText: "Delete this entry?",
    },
},
```

Useful `Action` fields:

- `Params` - templated route params (e.g. `${resource.uid}`).
- `Confirm` / `ConfirmText` - a confirmation dialog before firing.
- `OnSuccess` - `{SelectTab, Navigate}` to move the workbench after success
  (`NavigateList` returns to the list).
- `Open` + `Panel` + `Config` - open a panel in the dock (`OpenDock`), a modal
  (`OpenDialog`), the main view (`OpenView`), or a new browser tab (`OpenURL`,
  using the route-returned URL).
- `EnabledWhen` - a `*Condition` over the active row's fields to gray out the button.
- `Group` - cluster actions into a labeled dropdown. `IconOnly` - icon button + tooltip.

Reference actions from a panel's `ActionIDs`/`RowActionIDs`, a resource's
`Actions` (toolbar/row/detail), or `Manifest.HeaderActions`.

## Resource trees & detail views

For `LayoutSidebarTree`, declare `Tree` roots (`TreeGroup`) that expand lazily
via a `DataSource` returning `TreeNode`s, and `Resources` (`ResourceType`) that
define a managed object's list columns, watch stream, actions (toolbar/row/
detail), and a `DetailView` (header + tabbed panels) opened on row click. This is
how the container/k8s-style plugins build deep navigation - all declarative.

## Scope filters

`Scope` declares global selectors (a namespace picker, a region dropdown)
injected into every read/stream route's params. Each `ScopeFilter` has a
`Param`, `Label`, a `Control` (`ScopeSelect`, `ScopeMultiSelect`, `ScopeSearch`,
`ScopeToggle`), and either static `Options` or an `OptionsSource`. Read the
chosen value in a handler with `rc.Param("<param>")`; for multi-select scopes,
use `rc.ParamList("<param>", plugin.ScopeSeparator)`.

## Transports

ShellCN supports exactly two transports, and `SupportedTransports` lists which
of them your connection form offers:

| Transport         | Meaning                                                                                             |
| ----------------- | --------------------------------------------------------------------------------------------------- |
| `TransportDirect` | The gateway reaches the target itself (you give it host/port).                                      |
| `TransportAgent`  | The gateway tunnels through an agent running next to the target. Requires an `Agent *AgentProfile`. |

Declare one or both. Your handler and session code are **identical** either way -
you always dial through `cfg.Net` (see [sessions.md](sessions.md)); the gateway
wires it to a direct dial or the agent tunnel for you.

```go
// Direct-only (most plugins):
SupportedTransports: []plugin.Transport{plugin.TransportDirect},

// Both - the form lets the user choose; add an AgentProfile for the agent path:
SupportedTransports: []plugin.Transport{plugin.TransportDirect, plugin.TransportAgent},
Agent: &plugin.AgentProfile{
    Proxy: plugin.ProxyTarget{Mode: plugin.AgentTCP, Address: "127.0.0.1:5432", Risk: plugin.RiskPrivileged},
    Install: []plugin.InstallArtifact{
        {Label: "Docker", Kind: "docker", Template: "docker run ... {{.ConnectURL}} {{.Token}}"},
    },
},
```

When you offer both, hide the direct-only config fields (host, port, socket) under
the agent transport with a `$transport` condition - see
[Conditions](#conditions-visiblewhen). Full agent details: [agents.md](agents.md).
For protocols that discover additional upstream addresses after bootstrap, such
as Kafka brokers, see the `Forward` guidance in [agents.md](agents.md#fixed-targets-vs-forwarded-targets).

## Capabilities

`Capabilities []Capability` are free-form, declarative feature tags (e.g.
`"metrics"`, `"promql"`, `"exec"`). They are descriptive only - the gateway does
**not** dispatch behavior on them; routes and panels drive everything. Use them to
advertise what the plugin can do; leave the slice empty if you have nothing to tag.

## Streaming & recording

Interactive screens (terminal, logs, desktop) are WS routes declared in
`Streams`, paired with a `PanelTerminal`/`PanelLogStream`/`PanelRemoteDesktop`.
`Recording` declares which stream classes can be recorded. See
[streaming.md](streaming.md).

## Validation at load

The gateway validates the whole manifest when it loads the plugin - unknown
references, malformed schemas, an agent transport without an `AgentProfile`, etc.
A bad manifest is rejected up front with a clear operator-facing error, the same
gate a built-in passes.
