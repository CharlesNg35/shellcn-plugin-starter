# Web proxy (embed a target's web UI)

Some targets have their own web interface - a container's exposed port, a
Kubernetes Service, a database admin console. ShellCN can **embed that UI** inside
a panel, served through the gateway so it inherits the connection's auth and
audit and works for both direct and agent connections. You opt in by implementing
one optional method on your `Session`.

This is **not** agent-specific - it's a session capability that works on any
transport.

## The capability

A `Session` can implement `plugin.HTTPProxy`:

```go
type HTTPProxy interface {
    ServeHTTPProxy(w http.ResponseWriter, r *http.Request)
}
```

The gateway mounts a per-connection path:

```
/api/connections/{id}/proxy/{your-sub-path...}
```

Every request under it is authenticated against the connection, then handed to
your session's `ServeHTTPProxy`. The wildcard (everything after `/proxy/`) arrives
as `r.URL.Path` - you decide what it means (which port, which service, which file).
If your session doesn't implement the interface, the gateway returns
`ErrNotSupported`.

## Implementing it

Reverse-proxy the request to the target's web service **through `cfg.Net`** (so
egress stays on the gateway transport), and rewrite the response so the app's
absolute URLs resolve back under the proxy prefix:

```go
func (s *Session) ServeHTTPProxy(w http.ResponseWriter, r *http.Request) {
    host, upstreamPath, ok := s.resolveTarget(r.URL.Path) // your routing
    if !ok {
        http.Error(w, "unsupported proxy target", http.StatusBadRequest)
        return
    }
    prefix := "/api/connections/" + s.connID + "/proxy"
    proxy := &httputil.ReverseProxy{
        Director: func(req *http.Request) {
            req.URL.Scheme = "http"
            req.URL.Host = host
            req.URL.Path = upstreamPath
            req.Host = host
        },
        Transport:      s.transport, // an http.Transport whose DialContext is cfg.Net.DialContext
        FlushInterval:  -1,           // stream responses, don't buffer
        ModifyResponse: rewriteUnderPrefix(prefix),
    }
    proxy.ServeHTTP(w, r)
}
```

Build `s.transport` once in `Connect` from the gateway dialer, so the proxy can't
reach anything the connection can't:

```go
s.transport = &http.Transport{
    DialContext: cfg.Net.DialContext,
}
```

## Expose an "Open in browser" link

The browser needs the proxy URL. Return it from a normal route so an action or
detail button can open it:

```go
func proxyURL(rc *plugin.RequestContext) (any, error) {
    s, err := sess(rc)
    if err != nil {
        return nil, err
    }
    u := "/api/connections/" + s.connID + "/proxy/" + rc.Param("port") + "/"
    return map[string]any{"url": u}, nil
}
```

## Rewriting is the hard part

Proxying a static page is trivial; proxying a real single-page app is not. The
app emits root-absolute URLs (`/assets/app.js`), redirects, and cookies that all
assume it owns the origin - under a `/proxy/...` prefix they break. A correct
proxy must rewrite, in the response:

- `Location` redirects and `Set-Cookie` `Path=` back under the prefix,
- root-absolute URLs in HTML (`href`/`src`/`action`/`srcset`), CSS `url(...)`, and
  `<meta refresh>`,
- framing/CSP headers (relax `X-Frame-Options` / `Content-Security-Policy` so the
  page embeds),
- and inject a small runtime shim plus a service worker so the app's _dynamic_
  `fetch`/`WebSocket`/navigation requests stay under the prefix too.

The built-ins do all of this in a gateway-internal `webproxy` package
(`plugins/shared/webproxy`) that external plugins **cannot import**. For a simple
internal tool, `httputil.ReverseProxy` with the `Director`/`ModifyResponse` above
is enough. For a full SPA, read that package and the kubernetes/docker proxy
handlers (`proxy_http.go`) and port the rewriting you need into your plugin.

## Notes

- **Auth and audit still apply** - the gateway authenticates every proxied request
  against the connection before calling you; you don't re-check.
- **Set `FlushInterval: -1`** so streamed responses (logs, server-sent events)
  aren't buffered.
- **Preserve percent-encoding** when the sub-path can contain encoded characters:
  read `r.URL.EscapedPath()` for the raw form and set `req.URL.RawPath`, as the
  kubernetes handler does for route-group chunk names.
