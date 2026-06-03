# Best practices

These conventions are distilled from ShellCN's 40 built-in plugins, which use
the **same SDK** an external plugin does. Following them keeps your plugin
idiomatic, reviewable, and consistent with the rest of the catalog.

## Project layout

The plugin is a normal Go program; split it the way the built-ins do rather than
one big file:

| File          | Holds                                                                |
| ------------- | -------------------------------------------------------------------- |
| `main.go`     | `func main() { sdk.Serve(...) }` - nothing else.                     |
| `manifest.go` | The plugin type, `Manifest()`, `Routes()`, `Connect()`.              |
| `session.go`  | The `Session` struct, its methods, and the route handlers.           |
| `config.go`   | The `Schema` and option parsing/validation (once it grows).          |

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

Encode an opaque cursor (the built-ins base64 an offset). Don't dump unbounded
result sets - the limit is clamped for a reason.

## Streaming: bridge, watch the client, tear down

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

## Test the manifest and the handlers

Every built-in has a unit test that validates the manifest, plus handler tests
using the fake transports in `sdk/plugintest`:

```go
func TestManifestValidates(t *testing.T) {
    p := New()
    if err := plugin.Validate(p.Manifest(), p.Routes()); err != nil {
        t.Fatalf("invalid manifest: %v", err)
    }
}
```

- `plugintest.DirectTransport()` - a real OS dialer for L4 tests.
- `plugintest.HTTPTransport(baseURL, rt)` - point L7 clients at an `httptest.Server`.
- `plugintest.TransportFunc(dial)` - a custom dialer (agent-style).

Build a session, then drive a handler with `plugin.NewRequestContext(ctx, user,
sess, params, query, body)` and assert the returned value. Run `go test -race`.

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
