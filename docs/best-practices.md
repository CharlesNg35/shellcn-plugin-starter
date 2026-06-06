# Best practices

These conventions are distilled from ShellCN's 40 built-in plugins, which use
the **same SDK** an external plugin does. Following them keeps your plugin
idiomatic, reviewable, and consistent with the rest of the catalog.

## Project layout

The plugin is a normal Go program; split it the way the built-ins do rather than
one big file:

| File          | Holds                                                       |
| ------------- | ----------------------------------------------------------- |
| `main.go`     | `func main() { sdk.Serve(...) }` - nothing else.            |
| `manifest.go` | The plugin type, `Manifest()`, `Routes()`, `Connect()`.     |
| `session.go`  | The `Session` struct, its methods, and the route handlers.  |
| `config.go`   | The `Schema` and option parsing/validation (once it grows). |

Built-ins range from a single ~60-line file (`plugins/s3`) to a directory of
domain files (`plugins/kubernetes`). Start small; split by concern as it grows.
Keep manifest helpers (`icon()`, schema builders) as package functions right
after `Manifest()`.

## The plugin is a stateless singleton

The built-ins expose `func New() *Plugin` returning a zero-value struct and put
**all** state in the `Session`. One plugin value serves every connection
concurrently, so it must hold no per-connection data.

```go
type Plugin struct{}

func New() *Plugin { return &Plugin{} }
```

(This starter uses a value receiver, `Starter{}`, which is equivalent - pick one
and keep the plugin field-free.)

## Naming

The catalog is consistent because everyone follows the same scheme:

- **Plugin `Name`** - lowercase, short, stable (`postgresql`, `ssh`, `kubernetes`).
  Never change it after release; it's stored on every connection.
- **Route `ID`** - `"{name}.{entity}.{action}"` (`postgresql.table.row.insert`,
  `ssh.shell`, `docker.container.logs`).
- **`Permission`** - `"{name}.{resource}.{verb}"` (`docker.containers.read`,
  `redis.keys.delete`).
- **`AuditEvent`** - set it equal to the route `ID`, so the audit log filters by
  operation cleanly.
- **`Risk`** - `RiskSafe` for reads, `RiskWrite` for create/update,
  `RiskDestructive` for delete/truncate, `RiskPrivileged` for shell/exec/raw
  socket. The gateway enforces it; be honest.

## Connect: eager validate, lazy sub-clients

Two patterns, both used in-tree:

- **Eager** (Redis, MongoDB): open the client in `Connect`, then call your own
  `HealthCheck` and return the error if it fails - the user gets an immediate,
  clear connect error.
- **Lazy** (PostgreSQL opens a pool per database on demand): store `cfg` and
  `cfg.Net`, then open sub-clients on first use behind a mutex.

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

Some SDKs only accept a `DialContext` (not a RoundTripper) - wire
`cfg.Net.DialContext` into their `http.Transport{DialContext: ...}`. That's how
the Prometheus/Elasticsearch/S3 built-ins do L7.

## Credentials: read the resolved secret, never store one

Prefer a `FieldCredentialRef` over inline secret fields. The gateway decrypts the
chosen credential and injects it into `cfg`; read it with the accessors - your
plugin never sees ciphertext or persists a secret:

```go
user := cfg.String("username")
pass := cfg.String("password")
if id := cfg.CredentialIdentityFor(plugin.CredentialField); id != "" {
    user = id // the credential can supply the username too
}
if secret := cfg.CredentialSecretFor(plugin.CredentialField); secret != "" {
    pass = secret
}
```

## Reading config safely

`cfg.String(key)` returns `""` if absent/non-string; `cfg.Int(key)` returns
`(0, false)`. **Schema `Default`s are UI hints, not runtime defaults** - apply
fallbacks in code, and validate:

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

Encode an opaque cursor (the built-ins base64 an offset). `rc.Page()` already
clamps the limit to `MaxPageLimit`, so don't dump unbounded result sets.

When the data is **already in memory** (a fixed list you fetched whole), don't
hand-roll filter/sort/paging - the SDK has the primitives the built-ins use:
`plugin.FilterRows(rows, req.Search())` for the free-text box and
`plugin.SortRows(rows, req.Sort)` for the column sort, then slice by the cursor.
A handful of generic helpers cover most list handlers; reach for them before
writing your own loop.

## Many object types? Parameterize routes by kind

If your protocol has dozens of object types (kubernetes has pods, deployments,
services, ...), don't write near-identical routes for each. Declare **one** set of
routes keyed by a `{kind}` path param, and resolve it against a catalog in the
handler:

```go
{ID: "k8s.resource.list",   Method: plugin.MethodGet,    Path: "/resources/{kind}",        Handle: ListResource},
{ID: "k8s.resource.delete", Method: plugin.MethodDelete, Path: "/resources/{kind}/delete", Handle: DeleteResource},
{ID: "k8s.resource.watch",  Method: plugin.MethodWS,     Path: "/resources/{kind}/watch",  Stream: WatchResource},

func ListResource(rc *plugin.RequestContext) (any, error) {
    k, err := resolveKind(s, rc.Param("kind")) // look up in a catalog (+ runtime CRDs)
    if err != nil {
        return nil, err
    }
    // ...one generic implementation drives every kind...
}
```

Kubernetes serves its whole catalog (plus runtime CRDs) from ~6 routes this way.
Keep the catalog (kind -> columns, actions, detail tabs) as data, so adding a kind
is a data change, not new routes. Permissions still apply per-route, so group
kinds that share a risk level.

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
- `StreamLogs`, `StreamMetrics`, and `StreamFile` are server-to-browser streams.
  Do not label logs, watches, metrics, or long-running query results as terminal
  streams just because they use WebSockets; that can cause false idle timeouts.

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

- `PanelQueryEditor` sends `{ "query": "...", "confirm": false }`.
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
  The shared packages are the gateway's own reference implementations; read them
  for patterns, but copy what you need into your plugin.
- Don't change `Manifest.Name` after release - connections are keyed by it.
