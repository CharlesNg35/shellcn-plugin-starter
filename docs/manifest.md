# Manifest

The `Manifest` is your plugin's single declarative contract. The gateway reads
it once at load and the frontend renders whatever it declares — so most of
building a plugin is writing a good manifest.

```go
func (Starter) Manifest() plugin.Manifest {
    return plugin.Manifest{
        APIVersion:          plugin.CurrentAPIVersion,
        Name:                "starter",
        Version:             "0.1.0",
        Title:               "Starter",
        Description:         "A template plugin.",
        Icon:                plugin.Icon{Type: plugin.IconLucide, Value: "box"},
        Category:            plugin.CategoryOther,
        Layout:              plugin.LayoutTabs,
        SupportedTransports: []plugin.Transport{plugin.TransportDirect},
        Config:              plugin.Schema{ /* connection-form fields */ },
        Tabs:                []plugin.Panel{ /* the workspace */ },
        Actions:             []plugin.Action{ /* buttons → routes */ },
    }
}
```

## Identity

| Field         | Purpose                                                              |
| ------------- | ------------------------------------------------------------------- |
| `APIVersion`  | Always `plugin.CurrentAPIVersion`. The gateway refuses a mismatch.  |
| `Name`        | Unique, lowercase id (e.g. `redis`). Stored on every connection.    |
| `Version`     | Your plugin's own version string.                                   |
| `Title`       | Human label shown in the catalog and workspace.                     |
| `Description` | One line shown in the protocol picker.                              |
| `Icon`        | See below.                                                          |
| `Category`    | Groups the protocol in the picker (see below).                      |

### Icon

```go
plugin.Icon{Type: plugin.IconLucide, Value: "database"}
```

`Type` is one of `IconLucide` (a [Lucide](https://lucide.dev) name, kebab-case),
`IconURL`, `IconBase64` (data URI), `IconEmoji`, or `IconSVG` (inline markup,
sanitized before render).

### Category

One of: `CategoryShell`, `CategoryFiles`, `CategoryContainers`,
`CategoryOrchestration`, `CategoryVirtualization`, `CategoryRemoteDesktop`,
`CategoryDatabases`, `CategorySearch`, `CategoryObservability`,
`CategoryMessaging`, `CategoryNetwork`, `CategoryCloud`, `CategoryDevOps`,
`CategorySecurity`, `CategoryLookup`, `CategoryOther`.

## Connection form: `Config`

`Config` is a `Schema` whose fields become the connection-form inputs (host,
port, credentials, options). The same schema validates the saved config. Leave
it empty if your plugin needs no configuration.

```go
Config: plugin.Schema{Groups: []plugin.Group{{
    Name: "Connection",
    Fields: []plugin.Field{
        {Key: "host", Label: "Host", Type: plugin.FieldText, Required: true},
        {Key: "port", Label: "Port", Type: plugin.FieldNumber, Default: 6379},
        {Key: "password", Label: "Password", Type: plugin.FieldPassword, Secret: true},
        {Key: "tls", Label: "Use TLS", Type: plugin.FieldToggle},
    },
}}},
```

Field types include `FieldText`, `FieldTextarea`, `FieldPassword`, `FieldNumber`,
`FieldToggle`, `FieldSelect`, `FieldMultiSelect`, `FieldRadio`, `FieldEmail`,
`FieldURL`, `FieldDuration`, `FieldFile`, and `FieldCredentialRef` (reusable
credentials). `Secret: true` encrypts the value at rest and never returns it to
the client. Fields can show conditionally via `VisibleWhen` and carry
`Validators` (min/max/regex/oneOf). The schema is also the input contract for
routes — see [routes.md](routes.md).

At runtime your `Connect` receives the decrypted values in `cfg.Config` (read
them with `cfg.String("host")`, `cfg.Int("port")`).

## Workspace layout

`Layout` picks how the connection workspace is arranged:

| Layout               | Use for                                                  |
| -------------------- | -------------------------------------------------------- |
| `LayoutTabs`         | A flat tab bar, one panel at a time (most plugins).      |
| `LayoutSidebarTree`  | A resource tree on the left + a detail pane.             |
| `LayoutDashboard`    | A grid of panels shown at once.                          |
| `LayoutSingle`       | One full-bleed panel (a terminal/desktop/file screen).   |

## Panels (`Tabs`)

Each `Panel` is a screen in the workspace. A panel has a `Type` and a `Config`
shaped for that type, and most are fed by a route via `Source`.

```go
Tabs: []plugin.Panel{{
    Key:    "entries",
    Label:  "Entries",
    Icon:   plugin.Icon{Type: plugin.IconLucide, Value: "list"},
    Type:   plugin.PanelTable,
    Source: &plugin.DataSource{RouteID: "starter.list"},
    Config: plugin.TableConfig{
        Columns:      []plugin.Column{{Key: "key", Label: "Key", Sortable: true}, {Key: "value", Label: "Value"}},
        ActionIDs:    []string{"starter.set"},    // toolbar buttons
        RowActionIDs: []string{"starter.delete"}, // per-row buttons
    },
}},
```

Panel types include `PanelTable`, `PanelForm`, `PanelTerminal`, `PanelLog`,
`PanelQuery`, `PanelFile`, `PanelMetrics`, `PanelGraph`, `PanelDocument`,
`PanelCode`, `PanelRemote` (desktop), `PanelDashboard`, and `PanelEnroll` (agent
setup). The column `Key`s must match the JSON field names your list route
returns.

## Actions

An `Action` binds a button to a route. With an `Input` schema on the route it
opens a form; otherwise it fires immediately. `Params` templating pulls values
in — `${resource.uid}` is the selected row's id.

```go
Actions: []plugin.Action{
    {ID: "starter.set", Label: "Set entry", Icon: icon("plus"), RouteID: "starter.set"},
    {
        ID: "starter.delete", Label: "Delete", Icon: icon("trash-2"), RouteID: "starter.delete",
        Params:      map[string]string{"key": "${resource.uid}"},
        Confirm:     true,
        ConfirmText: "Delete this entry?",
    },
},
```

Reference actions from a panel's `ActionIDs`/`RowActionIDs`, or from
`HeaderActions` to place them in the workspace header.

## Transports

`SupportedTransports` lists what the connection form offers:

- `TransportDirect` — the gateway reaches the target itself.
- `TransportAgent` — the gateway tunnels through an enrolled agent. Requires an
  `Agent *AgentProfile`. See [agents.md](agents.md).

Your handler code is identical either way; the gateway hands the right transport
to your session.

## Streaming & recording

If your plugin serves interactive screens (terminal, logs, desktop), declare
each one in `Streams` and, optionally, what can be recorded in `Recording`. See
[streaming.md](streaming.md).

## Validation

Manifests are validated at load. A malformed manifest is rejected before the
plugin can serve a request, with a clear operator-facing error — the same gate a
built-in passes through.
