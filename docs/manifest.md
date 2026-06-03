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

| Field         | Notes                                                               |
| ------------- | ------------------------------------------------------------------- |
| `APIVersion`  | Always `plugin.CurrentAPIVersion`. A mismatch is refused at load.   |
| `Name`        | Unique, lowercase id (`redis`, `acme-db`). Stored on every connection - don't change it after release. |
| `Version`     | Your plugin's own version string (bump per release).                |
| `Title`       | Human label in the catalog and workspace.                           |
| `Description` | One line in the protocol picker.                                    |
| `Icon`        | See [Icon](#icon).                                                   |
| `Category`    | Groups the protocol in the picker (see [Category](#category)).      |

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

| Field             | Purpose                                                          |
| ----------------- | ---------------------------------------------------------------- |
| `Key`             | Config key; how you read it at runtime (`cfg.String("host")`).   |
| `Label`           | Form label.                                                      |
| `Type`            | Widget (see below).                                              |
| `Required`        | Validated server-side before save and before route handlers.    |
| `Secret`          | Encrypted at rest; never returned to the client. Use for keys/passwords. |
| `Default`         | Pre-filled value.                                                |
| `Placeholder` / `Help` | Hints shown in the form.                                    |
| `Options`         | Static choices for select/radio/multiselect.                     |
| `OptionsSource`   | A `*DataSource` to populate choices from a route at form-open.   |
| `Credential`      | A `*CredentialSelector` for `FieldCredentialRef` (stores only the chosen credential id). |
| `VisibleWhen`     | A `*Condition` - show the field only when other values match.    |
| `Validators`      | Server-side checks (`min`/`max`/`regex`/`oneOf`).                |
| `Step`            | Increment for number/slider.                                     |
| `Fields` / `Item` / `MinItems` / `MaxItems` / `ItemLabel` / `AddLabel` / `KeyLabel` | Composite shapes - see [composite fields](#composite-fields). |

Field types: `FieldText`, `FieldTextarea`, `FieldPassword`, `FieldEmail`,
`FieldURL`, `FieldTel`, `FieldNumber`, `FieldStepper`, `FieldSlider`,
`FieldToggle`, `FieldSelect`, `FieldRadio`, `FieldMultiSelect`,
`FieldAutocomplete`, `FieldJSON`, `FieldDuration`, `FieldFile`,
`FieldCredentialRef`, and the composites `FieldObject`, `FieldArray`, `FieldMap`.

### Conditions (`VisibleWhen`)

A `Condition` is `AllOf`/`AnyOf` lists of `Rule{Field, Op, Value}`. Operators:
`OpEq`, `OpNeq`, `OpIn`, `OpNin`, `OpEmpty`, `OpNotEmpty`. Two context keys are
available besides field values: `SchemaContextTransport` (`$transport`) and
`SchemaContextProtocol` (`$protocol`) - e.g. show a field only for the direct
transport.

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
`CredentialCloudAccessKey`. To define your own, list `CredentialKindInfo`
entries in `Manifest.CredentialKinds`. The field stores only the credential id;
the gateway resolves and injects the secret - the client never sees it.

### Composite fields

- `FieldObject` nests `Fields` (a sub-form).
- `FieldArray` repeats `Item` (a list of values/objects; `MinItems`/`MaxItems`/`AddLabel`).
- `FieldMap` is repeatable key/value rows whose value type is `Item`
  (`KeyLabel`/`KeyPlaceholder`).

## Workspace layout

`Layout` arranges the connection workspace:

| Layout               | Use for                                                    |
| -------------------- | ---------------------------------------------------------- |
| `LayoutTabs`         | A flat tab bar, one `Panel` at a time (most plugins).      |
| `LayoutSidebarTree`  | A resource `Tree` on the left + a detail pane.             |
| `LayoutDashboard`    | A grid of panels (from `Tabs`) shown at once.              |
| `LayoutSingle`       | One full-bleed panel (a terminal/desktop/file screen).     |

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

| `PanelType`           | Config type           | Renders                              |
| --------------------- | --------------------- | ------------------------------------ |
| `PanelTable`          | `TableConfig`         | A data grid (optionally editable).   |
| `PanelForm`           | `FormPanelConfig`     | A submit form.                       |
| `PanelTerminal`       | `TerminalConfig`      | An xterm terminal (WS route).        |
| `PanelLogStream`      | -                     | A live log tail (WS route).          |
| `PanelQueryEditor`    | `QueryEditorConfig`   | A SQL/query editor + results.        |
| `PanelFileBrowser`    | `FileBrowserConfig`   | A file manager.                      |
| `PanelCodeEditor`     | `CodeEditorConfig`    | A Monaco editor.                     |
| `PanelMetrics`        | `MetricsConfig`       | KPI cards, gauges, time-series.      |
| `PanelGraph`          | `GraphConfig`         | A node/edge graph.                   |
| `PanelTrace`          | `TraceConfig`         | A distributed-trace view.            |
| `PanelKV`             | `KVConfig`            | A key/value browser.                 |
| `PanelHTTPClient`     | `HTTPClientConfig`    | A REST client.                       |
| `PanelRemoteDesktop`  | `RemoteDesktopConfig` | A VNC/RDP screen.                    |
| `PanelDocument`       | -                     | Rendered document/markdown.          |
| `PanelDashboard`      | `DashboardConfig`     | A grid of nested panels (`Cells`).   |
| `PanelEnroll`         | -                     | The agent-enrollment screen.         |

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
chosen value(s) in a handler with `rc.Param("<param>")` / `rc.ParamList`.

## Transports

`SupportedTransports` lists what the connection form offers - `TransportDirect`
(the gateway reaches the target) and/or `TransportAgent` (tunnel through an
enrolled agent; requires `Agent *AgentProfile`). Your handler code is identical
either way. See [agents.md](agents.md).

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
