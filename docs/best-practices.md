# Best practices

These conventions keep a ShellCN plugin idiomatic, reviewable, and consistent
with the renderer and gateway contracts. They are written for external plugin
authors: depend on the SDK, keep behavior manifest-driven, and avoid assumptions
about gateway internals.

## Project layout

The plugin is a normal Go program. Start small, then split by responsibility as
the protocol grows:

| File          | Holds                                                       |
| ------------- | ----------------------------------------------------------- |
| `main.go`     | `func main() { sdk.Serve(...) }` - nothing else.            |
| `manifest.go` | The plugin type, `Manifest()`, `Routes()`, `Connect()`.     |
| `session.go`  | The `Session` struct, its methods, and the route handlers.  |
| `config.go`   | The `Schema` and option parsing/validation (once it grows). |

Keep manifest helpers (`icon()`, schema builders, column definitions, action
lists) as package functions near `Manifest()` until they are large enough to
deserve their own file.

## The plugin is a stateless singleton

Expose a zero-value plugin type and put **all** per-connection state in the
`Session`. One plugin value serves every connection concurrently, so it must hold
no per-connection data.

```go
type Plugin struct{}

func New() *Plugin { return &Plugin{} }
```

(This starter uses a value receiver, `Starter{}`, which is equivalent - pick one
and keep the plugin field-free.)

## Naming

The catalog is consistent because everyone follows the same scheme:

- **Plugin `Name`** - lowercase, short, stable (`mydb`, `myssh`, `mycluster`).
  It must match `[a-z][a-z0-9_-]*`; no dots, spaces, slashes, uppercase, or
  leading digits. Never change it after release; it's stored on every connection.
- **Route `ID`** - `"{name}.{entity}.{action}"` (`mydb.table.row.insert`,
  `myssh.shell`, `myruntime.container.logs`).
- **`Permission`** - `"{name}.{resource}.{verb}"` (`myruntime.containers.read`,
  `mycache.keys.delete`).
- **`AuditEvent`** - set it equal to the route `ID`, so the audit log filters by
  operation cleanly.
- **`Risk`** - `RiskSafe` for reads, `RiskWrite` for create/update,
  `RiskDestructive` for delete/truncate, `RiskPrivileged` for shell/exec/raw
  socket. The gateway enforces it; be honest.

The prefix is not cosmetic. The SDK validator rejects any route whose `ID` does
not start with `Manifest.Name + "."`. If your plugin name is `myplugin`, route
IDs must look like `myplugin.list`, `myplugin.entry.create`, and
`myplugin.shell`.

## Connect: eager validate, lazy sub-clients

Two patterns are useful:

- **Eager**: open the main client in `Connect`, then call your own
  `HealthCheck` and return the error if it fails. The user gets an immediate,
  clear connect error.
- **Lazy**: store the parsed config and `cfg.Net`, then open sub-clients on
  first use behind a mutex. Use this when a connection can target many databases,
  namespaces, projects, or child endpoints.

```go
func (s *session) clientFor(ctx context.Context, name string) (*Client, error) {
    s.mu.Lock()
    defer s.mu.Unlock()
    if c := s.clients[name]; c != nil {
        return c, nil
    }
    c, err := dial(ctx, s.net, name) // egress via the gateway
    if err != nil {
        return nil, err
    }
    s.clients[name] = c
    return c, nil
}
```

Guard shared session state with a mutex - a session serves concurrent requests.
`Close()` must tear down everything (cancel running ops, close pools/clients).

## Egress: always through `cfg.Net`, never your own dialer

A plugin must not open sockets itself - route everything through the transport
the gateway hands you, so direct and agent connections share one code path and
the gateway stays the audited choke point. The wiring depends on the layer:

**L4** - give your driver the gateway's dialer:

```go
// database/sql-style pools, redis, etc.
opts.Dialer = func(ctx context.Context, network, addr string) (net.Conn, error) {
    return cfg.Net.DialContext(ctx, network, addr)
}
// or a single dial for line protocols:
conn, err := cfg.Net.DialContext(ctx, "tcp", net.JoinHostPort(host, port))
```

**L7** - build an `http.Client` on the gateway's transport:

```go
base, rt, ok := cfg.Net.HTTP()
if !ok {
    return nil, fmt.Errorf("%w: no L7 transport", plugin.ErrUnavailable)
}
client := &http.Client{Transport: rt} // requests go to base + path
```

Some SDKs only accept a `DialContext` instead of a `RoundTripper`. In that case,
build an `http.Transport{DialContext: cfg.Net.DialContext}` and give that
transport to the SDK's HTTP client.

## Credentials: read the resolved secret, never store one

Prefer a `FieldCredentialRef` over inline secret fields. The gateway decrypts the
chosen credential and attaches it to `cfg.Credentials`; read it with the
credential accessors - your plugin never sees ciphertext or persists a secret:

```go
cred, err := cfg.RequiredCredentialFor(plugin.CredentialRefField, plugin.CredentialKindDBPassword)
if err != nil {
    return nil, err
}
user, err := cred.RequiredValue("username")
if err != nil {
    return nil, err
}
pass, err := cred.RequiredValue("password")
if err != nil {
    return nil, err
}
```

If inline config and a reusable credential can both carry the same key, such as
`username`, keep them separate and choose the source explicitly from the selected
auth mode. Do not merge credential values into `cfg.Config`.

## Plugin storage: keep scope simple

Use `rc.Storage` only for small plugin-owned user objects, such as snippets,
saved queries, saved request templates, or per-plugin preferences. Do not use it
as a cache for live infrastructure state; list/watch routes should read the
target system directly.

Create records in a collection. Core stores the current plugin, authenticated
user, and current connection automatically:

```go
_, err := rc.Storage.Put(rc.Ctx, "snippets", plugin.StorageItem{
    Key:   id,
    Value: payload,
})
```

Read, list, and delete with a scope. The scope is a filter, not a persisted
field:

```go
rows, err := rc.Storage.List(rc.Ctx, plugin.UserStorage("snippets"), nil)
```

- `plugin.StorageScope{Collection: "snippets"}` or
  `plugin.ConnectionStorage("snippets")` filters to the current connection and
  current user.
- `plugin.UserStorage("snippets")` filters to the current user across this
  plugin's connections.
- `Collection` separates logical record groups inside the plugin. Use a stable,
  lowercase plural name (`snippets`, `saved_queries`, `profiles`).
- `Key` is the record identifier inside that collection. If you need hierarchy
  for list routes, use a stable key prefix and `StorageListFilter.KeyPrefix`.
- For `plugin.UserStorage(...)`, make keys unique for that user/plugin/collection
  across connections. Use generated IDs for user-owned records; keyed get/delete
  operations fail with a conflict if the same key exists in multiple
  connections.

Core resolves the security context and write timestamps. Do not duplicate plugin
ID, owner ID, connection ID, `CreatedAt`, or `UpdatedAt` in your stored JSON
payload.

Store opaque `Value` bytes with a `ContentType`, plus lightweight `Metadata` for
labels or local sorting. Keep secrets out of plugin storage - use credentials
for secrets.

## Operator views: show comparisons explicitly

Operators read status screens by comparing current use against capacity. For
resources such as CPU, memory, disk, quota, connection pools, and queues, prefer
`ObjectDetailField.Usage` over separate loose fields like `used`, `total`, and
`percent`. The renderer shows the percentage, used/total text, thresholds, and a
progress bar consistently across object overview panels and live metrics panels.

Keep usage rows generic. Do not encode plugin names or product-specific visual
rules into the contract; use `UsageSpec` keys, types, labels, units, and
thresholds to describe the data.

## Reading config safely

`cfg.String(key)` returns `""` if absent/non-string; `cfg.Int(key)` returns
`(0, false)`. **Schema `Default`s are UI hints, not runtime defaults** - apply
fallbacks in code, and validate. For action forms, string defaults may use
`${resource.uid}` or `${resource.name}` to prefill from the selected row, but the
route must still validate the submitted value:

```go
host := strings.TrimSpace(cfg.String("host"))
if host == "" {
    return opts, fmt.Errorf("%w: host is required", plugin.ErrInvalidInput)
}
port, ok := cfg.Int("port")
if !ok || port == 0 {
    port = defaultPort
}
```

JSON numbers arrive as `float64` - `cfg.Int` already handles that; don't assert
`.(int)` yourself.

## Validate input in two layers

Untrusted input is checked twice, and you wire both:

- **The route's `Input` schema** is validated by the core wrapper **before** your
  handler runs, and the form uses it for instant feedback. Attach `Validators` to
  fields so bad input never reaches you:

  ```go
  {Key: "port", Label: "Port", Type: plugin.FieldNumber, Required: true,
   Validators: []plugin.Validator{
       {Type: plugin.ValidatorMin, Value: 1}, {Type: plugin.ValidatorMax, Value: 65535},
   }},
  ```

  The validators are `ValidatorMin`, `ValidatorMax`, `ValidatorRegex`, and
  `ValidatorOneOf`.

- **`rc.Bind(&dst)`** in the handler decodes the body into a typed struct and runs
  its `validate:"..."` struct tags, returning `ErrInvalidInput` on a bad payload:

  ```go
  var req struct {
      Name        string `json:"name" validate:"required"`
      IfNotExists bool   `json:"if_not_exists"`
  }
  if err := rc.Bind(&req); err != nil {
      return nil, err
  }
  ```

Schema validators and struct tags catch shape errors; they do **not** make a value
safe to interpolate. Re-check anything security-sensitive yourself - validate an
identifier against a whitelist before it touches a query (see
[explorer.md](explorer.md#build-sql-safely)).

## Schema UX: use the most specific control

The frontend is generic, so the plugin schema is the UX. Pick field types that
match the user's decision:

- Use `FieldSelect` or `FieldRadio` for closed vocabularies. Examples:
  authentication mode, TLS mode, restart policy, backup compression, power
  action, constraint type.
- Use `FieldMultiSelect` with `Options` or `OptionsSource` when the user chooses
  several known values. Examples: SQL columns for an index, privileges, namespace
  filters.
- Use `FieldAutocomplete` when there are common suggestions but custom values
  are still valid. Examples: SQL column types, S3 regions for custom endpoints,
  container network names, plugin driver names.
- Use `FieldText` only for genuinely open values. Examples: resource names,
  paths, expressions, usernames, socket paths, scopes, glob patterns.
- Use `FieldToggle`, `FieldStepper`, `FieldNumber`, `FieldDuration`,
  `FieldTextarea`, `FieldJSON`, `FieldArray`, `FieldObject`, and `FieldMap`
  where they describe the shape better than text.

Static `Options` are validated by the gateway for `select`, `radio`, and
`multiselect`. Do not use a closed control for extensible backend concepts
unless you really want to reject custom values. For runtime choices, prefer
`OptionsSource` pointing at a safe read route, so forms show the target's current
databases, schemas, columns, namespaces, containers, or buckets.

The manifest UX linter enforces the same idea at release time:

- Route IDs must be owned by the plugin namespace. If the plugin is named
  `myplugin`, use IDs such as `myplugin.list`, `myplugin.entry.create`, and
  `myplugin.events`; never borrow another plugin's prefix or use unprefixed IDs.
- Destructive and privileged actions must set `Confirm` with consequence-focused
  `ConfirmText`.
- `OpenDock` is for long-lived interactive panels only: terminal, desktop, logs,
  metrics, or task progress. Short forms/details should open as dialogs or
  ordinary views.
- Stream panel kind and manifest `Stream.Kind` must match. Do not declare logs,
  metrics, tasks, or one-way watches as terminal streams.
- Tables should declare useful column types, empty states, and a default sort
  where it helps scanning. Live data should declare `Watch` or
  `RefreshIntervalMs`.
- Actions should have labels, icons, conditions when state-dependent, and
  `OnSuccess` behavior when the next UI state is predictable.

## Use the right panel for the job

Do not shortcut the manifest by dumping everything into `PanelDocument` or a
single raw JSON view. A plugin should expose the target in the way an operator
expects to inspect and act on that target:

- Use `PanelObjectDetail` for a structured overview/property sheet: identity,
  status, placement, resource limits, metadata, badges, copyable IDs, and a raw
  toggle when useful.
- Use `PanelTable` for collections and child objects, with typed columns,
  meaningful empty states, default sort, and `Watch` or `RefreshIntervalMs` for
  live objects.
- For table rows that open a resource detail, return a `ref` object on each row.
  `${resource.uid}`, `${resource.name}`, `${resource.namespace}`, and
  `${resource.scope}` resolve from this reference:

  ```go
  type Row struct {
      Ref   plugin.ResourceIdentity `json:"ref"`
      Name  string             `json:"name"`
      State string             `json:"state"`
  }

  row := Row{
      Ref: plugin.ResourceIdentity{
          Kind: "container",
          Name: "web",
          UID:  "7f4c...",
      },
      Name:  "web",
      State: "running",
  }
  ```

- For plain rows that do not navigate to a resource detail, do not add a fake
  `ref`. Row actions and row-opened forms can use the selected row data through
  `${record.*}`, such as `map[string]string{"id": "${record.id}"}`.

- Keep the sidebar tree for navigation, not data. Do not add
  `TreeGroup.Source` or `TreeNode.ChildrenSource` just to expand every pod,
  container, task, backup, metric, table row, or message into the sidebar. If the
  children can grow large or need paging/search/filtering, make the tree node a
  leaf with `ResourceKind` so it opens a `PanelTable`. Use expandable tree
  sources only for bounded navigation, such as categories, databases, schemas, or
  a short list of child collections.
- Use `PanelTimeline` for events, tasks, audit trails, Kubernetes events,
  background jobs, and lifecycle history.
- Use `PanelMetrics` for live CPU, memory, throughput, latency, queue depth, or
  capacity signals. If the backend metrics API is absent, degrade gracefully and
  still show static request/limit/capacity data where available.
- Use `PanelCodeEditor` for editable text/config/manifests. It already gives the
  user a local diff before saving.
- Use `PanelDiff` only when the plugin can produce two meaningful versions
  server-side, such as planned vs current, dry-run vs current, or backup vs live.
- Use `PanelTaskProgress` for long-running operations instead of returning a
  synchronous "started" result with no follow-up.
- Use terminal/log/desktop stream panels only for real long-lived interactive or
  streaming channels.
- Use `PanelCanvas` only for custom visual or interactive surfaces that cannot
  be represented by the standard panels. It is appropriate for games, topology
  canvases, protocol visualizers, custom graph editors, or whiteboard-like
  tools. Use the typed canvas SDK package
  (`github.com/charlesng35/shellcn/sdk/plugin/canvas`) and structs such as
  `canvas.Frame`, `canvas.Rect`, and `canvas.PointerEvent` instead of hand-built
  `map[string]any` draw or input payloads. The browser wire is JSON, but plugin
  code should stay compiler-checked. It is not a shortcut around `PanelTable`,
  `PanelForm`, `PanelObjectDetail`, `PanelTimeline`, or `PanelTaskProgress`;
  those panels remain the professional default for operations UI because they
  preserve accessibility, keyboard behavior, validation, export, theming, and
  generic renderer consistency.
- Use the richer typed canvas helpers before inventing raw command payloads:
  `canvas.Radii` for per-corner cards, partial `canvas.Clear` for dirty regions,
  `canvas.TextBox` for padded/ellipsized labels, `canvas.FillText` or
  `canvas.StrokeText` when text rendering must be explicit, `canvas.Cursor` for
  global cursor changes, `canvas.FocusRegion` and `canvas.Announce` for
  accessible canvas-driven controls, and region helpers such as
  `canvas.RectRegion` for hit targets. For image opacity, use
  `canvas.Image{Paint: canvas.Paint{Alpha: ...}}`.
- Choose `CanvasConfig.ScaleMode` intentionally. Use `CanvasScaleResize` when the
  plugin can redraw for the available viewport. Use `CanvasScaleFit` with
  `Width` and `Height` for fixed logical coordinate systems that should scale
  into the panel while preserving pointer coordinates. Use `CanvasScaleScroll`
  for dense maps, whiteboards, timelines, dependency graphs, and linked-node
  diagrams that need a larger stable surface and normal panel scrolling. Set
  `MaxScale` when a fixed logical surface should not grow beyond its designed
  size.
- Use `CanvasConfig.WheelMode` deliberately. `CanvasWheelModified` is usually the
  best choice for zoomable maps and editors because ordinary mouse-wheel
  scrolling still works; reserve `CanvasWheelCapture` for surfaces where wheel
  gestures must always belong to the canvas.
- Pick canvas sizing by app shape, not by plugin name. Responsive dashboards and
  games usually use `CanvasScaleResize`; fixed previews and report artboards use
  `CanvasScaleFit`; large worlds such as maps, timelines, whiteboards,
  dependency graphs, and spreadsheet-like canvases use `CanvasScaleScroll`.
  Wheel mode follows the same rule: `CanvasWheelNone` for no wheel behavior,
  `CanvasWheelModified` for optional zoom/pan, and `CanvasWheelCapture` only when
  wheel gestures are core to the canvas.
- Use `PanelWasm` only for isolated browser-side programs that genuinely need
  WASM: games, heavy simulations, portable visualizers, or existing WASM
  libraries. It is still manifest-driven. Declare every asset in
  `WasmConfig.Assets`, every boot script in `WasmConfig.Boot`, and every
  callable route or stream in `WasmConfig.Bridge`. The app receives
  `window.shellcn.route`, `window.shellcn.stream`, and `window.shellcn.asset`
  inside a sandboxed iframe; undeclared access is rejected by the renderer. Do
  read `window.shellcn.theme` and `window.shellcn.colors`, then subscribe with
  `window.shellcn.onTheme(fn)` so custom rendering respects light and dark mode.
  Do not use WASM as a shortcut
  around `PanelTable`, `PanelForm`,
  `PanelObjectDetail`, `PanelTimeline`, `PanelGraph`, or `PanelCanvas`.
- For generic WASM toolchains, expect a small JavaScript loader. Rust frameworks
  such as Leptos/Yew and wasm-bindgen usually emit `app.js` plus `app_bg.wasm`
  or require a `boot.js` wrapper. Declare those files in the manifest and load
  the real entry bytes through `window.shellcn.asset(window.shellcn.entry)`;
  avoid placeholder entries, relative fetches, and same-origin assumptions
  because the iframe is intentionally sandboxed.

Cover the important features of the domain, not just the minimum route that
works. A Kubernetes Pod overview should show scheduling, status, requests,
limits, live memory/CPU when available, logs, shell, YAML, and events. A database
table should expose rows, columns, indexes, relationships, and SQL. A container
should expose state, logs, shell, inspect/config, environment, ports, and
lifecycle actions. This keeps plugins professional and predictable without any
plugin-specific frontend code.

The rule of thumb: raw JSON/YAML is a fallback and an escape hatch, not the
primary UX. First choose the panel that matches the user's task, then keep the
raw view available where it helps diagnostics.

## Errors: wrap a sentinel, never return it bare

The gateway maps `plugin.Err*` to HTTP status codes. Always add context with
`%w`:

```go
return nil, fmt.Errorf("%w: dial target: %v", plugin.ErrUnavailable, err)
```

Sentinels: `ErrInvalidInput` (400), `ErrNotFound` (404), `ErrUnauthorized`
(401), `ErrForbidden` (403), `ErrConflict`/`ErrAlreadyExists` (409),
`ErrUnavailable` (503), `ErrNotSupported`. A small `mapError` that translates
your backend's errors to sentinels keeps handlers clean:

```go
func mapError(err error) error {
    switch {
    case err == nil:           return nil
    case isNotFound(err):      return plugin.ErrNotFound
    case isPermission(err):    return plugin.ErrForbidden
    default:                   return fmt.Errorf("%w: %v", plugin.ErrUnavailable, err)
    }
}
```

## Lists: return `Page[T]`, honor `rc.Page()`

Read cursor/limit/filter/sort once and return a `plugin.Page`:

```go
req, err := rc.Page()
if err != nil {
    return nil, err
}
term := req.Search() // the grid's free-text box ("q")
// ...query with req.Limit / req.Cursor / req.Sort...
return plugin.Page[Row]{Items: rows, NextCursor: next, Total: &total}, nil
```

Encode an opaque cursor, for example a base64-encoded offset or backend cursor.
`rc.Page()` already clamps the limit to `MaxPageLimit`, so do not dump unbounded
result sets.

When the data is **already in memory** (a fixed list you fetched whole), do not
hand-roll filter/sort/paging. The SDK has generic helpers:
`plugin.FilterRows(rows, req.Search())` for the free-text box and
`plugin.SortRows(rows, req.Sort)` for the column sort, then slice by the cursor.
A handful of generic helpers cover most list handlers; reach for them before
writing your own loop.

## Many object types? Parameterize routes by kind

If your protocol has many object types, do not write near-identical routes for
each. Declare **one** set of routes keyed by a `{kind}` path param, and resolve
it against a catalog in the handler:

```go
{ID: "myplugin.resource.list",   Method: plugin.MethodGet,    Path: "/resources/{kind}",        Handle: ListResource},
{ID: "myplugin.resource.delete", Method: plugin.MethodDelete, Path: "/resources/{kind}/delete", Handle: DeleteResource},
{ID: "myplugin.resource.watch",  Method: plugin.MethodWS,     Path: "/resources/{kind}/watch",  Stream: WatchResource},

func ListResource(rc *plugin.RequestContext) (any, error) {
    k, err := resolveKind(s, rc.Param("kind")) // look up in a catalog (+ runtime CRDs)
    if err != nil {
        return nil, err
    }
    // ...one generic implementation drives every kind...
}
```

Keep the catalog (kind -> columns, actions, detail tabs) as data, so adding a
kind is a data change, not new routes. Permissions still apply per-route, so
group kinds that share a risk level.

## Streaming: declare the right kind, watch the client, tear down

For a terminal/exec, open an upstream channel and pump both ways, exiting on
client disconnect:

```go
func shell(rc *plugin.RequestContext, client plugin.ClientStream) error {
    ch, err := rc.Session.OpenChannel(rc.Ctx, plugin.ChannelRequest{Kind: plugin.StreamTerminal})
    if err != nil {
        return err
    }
    defer ch.Close()
    errc := make(chan error, 2)
    go func() { _, e := io.Copy(client, ch); errc <- e }()      // upstream → browser
    go func() { errc <- plugin.CopyTerminalInput(ch, client) }() // browser → upstream (handles resize)
    select {
    case <-client.Context().Done():
        return nil
    case err := <-errc:
        if err == io.EOF { return nil }
        return err
    }
}
```

`defer ch.Close()`, always `select` on `client.Context().Done()`, and treat
`io.EOF` as a clean close. `plugin.CopyTerminalInput` handles the terminal's
in-band resize frames for you - just implement `plugin.Resizer` (`Resize(cols,
rows int) error`) on your channel. See [streaming.md](streaming.md) for the
details and recording.

Declare stream kinds by browser behavior:

- `StreamTerminal` and `StreamDesktop` mean interactive streams with a continuous
  browser-to-upstream read loop. The gateway may apply WebSocket keepalive policy
  to these because pong frames are processed by that reader.
- `StreamLogs`, `StreamMetrics`, `StreamFile`, and `StreamTask` are
  server-to-browser streams. Do not label logs, watches, metrics, or
  long-running operations as terminal streams just because they use WebSockets;
  that can cause false idle timeouts.
- `StreamQuery` is for query editors. Its handler reads browser request frames
  and writes result frames on the same socket, so it must not be modeled as
  `StreamLogs`.

If a future stream shape is bidirectional, only treat it like terminal/desktop
when the handler continuously reads from the browser for the life of the stream.

For shell protocols, prefer `PanelTerminalGrid` when split panes are part of the
operator workflow. The manifest still declares a single `StreamTerminal` route;
the renderer opens one channel per pane. Keep every stream open independent, avoid
global session state for PTYs, and use `PanelTerminal` instead when mandatory
recording must remain available for that connection.

## Test the manifest and the handlers

Every plugin should keep a unit test that validates the manifest. The starter
ships this test by default; keep it when you rename the plugin. It is the first
line of defense for CI and local development because `go test` prints exact
manifest and UX contract failures before a bad plugin can be loaded by the
gateway.

```go
func TestManifestValidates(t *testing.T) {
    plugintest.ValidatePlugin(t, Starter{})
}
```

Import it with:

```go
import "github.com/charlesng35/shellcn/sdk/plugintest"
```

`ValidatePlugin` checks the core manifest contract, release-blocking UX rules,
projection generation, and every panel config against the SDK's
`PanelConfigSchemas`. That catches renderer-breaking mistakes such as unsupported
nested config keys, stream-kind/panel mismatches, destructive actions without
confirmation, `OpenURL` actions with required body fields, and runtime-only data
leaking into `Panel.Config`.

Plugin tests should call `plugintest.ValidatePlugin`, not duplicate lower-level
checks. The SDK keeps structural validation and renderer UX lint separate
internally, but the test helper runs both and reports the exact failing contract.

Keep panel config defaults intentional. For graphs, nil/omitted/null
`GraphConfig.Exportable` means client-side PNG/JPEG/SVG export is available; set
it to a pointer containing `false` only when the graph is sensitive enough that
the export menu should be hidden.

Use diff intentionally. `PanelCodeEditor` already gives writable documents a
generic changed-buffer diff, so do not add a separate diff tab just to compare a
user's local edits. Use `PanelDiff` for route-backed previews where your plugin
can return both sides: live config vs dry-run result, current document vs
proposed replacement, current spec vs rollback target, or generated DDL before
and after a change. Keep current-state inspection in `PanelObjectDetail`.

When the UX linter rejects a plugin, the failure is shown in the terminal running
`go test`, for example:

```text
action clickhouse.user.grant: privileged action must require confirmation
```

The same release-blocking errors are also rejected by the gateway at external
plugin registration time, so operators see a server log instead of a broken
browser workflow.

Add handler tests using the fake transports in `sdk/plugintest`.

For tests that need the projected browser contract, use the same helper path:

```go
proj := plugintest.Projection(t, p)
plugintest.ValidateProjectionPanelConfigs(t, proj)
```

- `plugintest.DirectTransport()` - a real OS dialer for L4 tests.
- `plugintest.HTTPTransport(baseURL, rt)` - point L7 clients at an `httptest.Server`.
- `plugintest.TransportFunc(dial)` - a custom dialer (agent-style).

Build a session, then drive a handler with `plugin.NewRequestContext(ctx, user,
sess, params, query, body)` and assert the returned value. Run `go test -race`.

Test the ShellCN panel payload, not only the upstream API payload. For example:

- `PanelQueryEditor` must use a `StreamQuery` source and sends
  `{ "query": "...", "confirm": false }`.
- `PanelCodeEditor` sends `{ "content": "..." }` unless `SaveBodyKey` is set.
- `PanelDiff` reads an object with the configured original/modified fields.

If your handler forwards requests to another API, add a test that uses the same
shape the gateway sends. That catches manifest and route contract mistakes before
release.

## A few don'ts

- Don't dial the target yourself - use `cfg.Net`.
- Don't put state on the plugin struct - use the `Session`.
- Don't return bare sentinels or raw backend errors - wrap with `%w`.
- Don't block a stream without watching `client.Context().Done()`.
- Don't import `github.com/charlesng35/shellcn/internal/...` or assume the
  gateway's `plugins/shared/...` packages are yours - depend only on the **SDK**.
  If you need a helper, implement it in your plugin or use a public dependency.
- Don't change `Manifest.Name` after release - connections are keyed by it.
