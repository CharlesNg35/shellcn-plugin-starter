# Sessions

The plugin value (`Starter{}`) is a **stateless singleton** - one instance
serves every connection. All per-connection state lives in a `Session`, which
`Connect` returns.

```go
func (Starter) Connect(ctx context.Context, cfg plugin.ConnectConfig) (plugin.Session, error) {
    // open clients/sockets here using cfg; return a Session that holds them.
    return newSession(), nil
}
```

The gateway calls `Connect` the first time a connection is used, reuses the
`Session` across requests, and `Close`s it when the connection idles out.

## The Session interface

```go
type Session interface {
    HealthCheck(ctx context.Context) error
    OpenChannel(ctx context.Context, req ChannelRequest) (Channel, error)
    Close() error
}
```

- **`HealthCheck`** - the gateway probes liveness before reusing an idle
  session. Ping your backend; return an error if it's gone.
- **`OpenChannel`** - open a tracked byte-stream to the upstream (terminals,
  exec, port-forwards). Return `plugin.ErrNotSupported` if you have none. See
  [streaming.md](streaming.md).
- **`Close`** - release everything the connection holds.

Handlers reach the session by type-asserting `rc.Session`:

```go
func list(rc *plugin.RequestContext) (any, error) {
    s := rc.Session.(*session)
    ...
}
```

Guard shared session state - a session can serve concurrent requests.

## ConnectConfig

`Connect` receives the decrypted connection config and the network transport:

```go
type ConnectConfig struct {
    ConnectionID string
    Transport    Transport       // "direct" or "agent"
    Config       map[string]any  // decrypted form values
    Net          NetTransport    // reach the target through here
}
```

Read config values with the typed helpers: `cfg.String("host")`,
`cfg.Int("port")`. Secret fields are already decrypted.

## Reaching the target: `cfg.Net`

**A plugin never dials the target itself.** The gateway owns network egress so
it stays the single audited choke point and so the *same* code works whether the
connection is direct or tunnelled through an [agent](agents.md). Use the
transport it hands you:

```go
type NetTransport interface {
    DialContext(ctx context.Context, network, addr string) (net.Conn, error) // L4
    HTTP() (baseURL string, rt http.RoundTripper, ok bool)                   // L7
}
```

### L4 (sockets / TCP-based protocols)

```go
conn, err := cfg.Net.DialContext(ctx, "tcp", cfg.String("host")+":"+port)
// speak your protocol over conn; the gateway routes it (direct or agent).
```

### L7 (HTTP/REST backends)

```go
base, rt, ok := cfg.Net.HTTP()
if !ok {
    return nil, fmt.Errorf("%w: no L7 transport", plugin.ErrUnavailable)
}
client := &http.Client{Transport: rt}
resp, err := client.Get(base + "/v1/info")
```

Store the transport on your session in `Connect` and reuse it from handlers.

## A typical Connect

```go
func (Starter) Connect(ctx context.Context, cfg plugin.ConnectConfig) (plugin.Session, error) {
    conn, err := cfg.Net.DialContext(ctx, "tcp", cfg.String("host")+":"+strconv.Itoa(port))
    if err != nil {
        return nil, err // propagates to the client as a clear connect error
    }
    return &session{client: newClient(conn), net: cfg.Net}, nil
}
```

If your backend is lazily opened, store `cfg.Net` and dial on first use (guard
with a mutex) - that keeps `Connect` fast and surfaces backend errors on the
request that needs them.

## Open in browser (HTTPProxy)

If your target serves a web UI (a dashboard, a container's exposed port, a DB
admin page), your Session can surface it in the browser. Implement the optional
`plugin.HTTPProxy` interface and reverse-proxy to the upstream through `cfg.Net`:

```go
type HTTPProxy interface {
    ServeHTTPProxy(w http.ResponseWriter, r *http.Request)
}

func (s *session) ServeHTTPProxy(w http.ResponseWriter, r *http.Request) {
    u, err := url.Parse(s.upstream)
    if err != nil {
        http.Error(w, "no upstream", http.StatusBadGateway)
        return
    }
    rp := httputil.NewSingleHostReverseProxy(u)
    rp.Transport = &http.Transport{DialContext: s.net.DialContext} // egress via the gateway
    rp.ServeHTTP(w, r)
}
```

The gateway authenticates and authorizes the request, then serves your proxy
under `/api/connections/{id}/proxy/...`, so redirects, assets, and WebSocket
upgrades pass through. To give the user a link, return that proxy URL from a
route and bind it to an `Action` with `Open: plugin.OpenURL`. The built-ins
(kubernetes, docker) use the gateway's `webproxy` helper to also rewrite HTML and
asset URLs onto the proxy prefix; read those for a full example.
