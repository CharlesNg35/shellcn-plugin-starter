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

The gateway mounts a per-connection path (today
`/api/connections/{id}/proxy/{your-sub-path...}`) - but the mount is
**core-owned URL space: never hardcode it**. The gateway hands it to you where
you need it: `rc.ProxyURL(...)` in route handlers and
`plugin.RequestProxyPrefix(r)` inside `ServeHTTPProxy`.

Every request under the mount is authenticated against the connection, then
handed to your session's `ServeHTTPProxy`. The wildcard (everything after the
mount) arrives as `r.URL.Path` - you decide what it means (which port, which
service, which file). If your session doesn't implement the interface, the
gateway returns `ErrNotSupported`.

## Implementing it

Use the SDK helper for real web apps:

```go
import "github.com/charlesng35/shellcn/sdk/plugin/webproxy"
```

Reverse-proxy the request to the target's web service **through `cfg.Net`** so
egress stays on the gateway transport:

```go
func (s *Session) ServeHTTPProxy(w http.ResponseWriter, r *http.Request) {
    base, upstreamPath, ok := s.resolveTarget(r.URL.Path) // your routing
    if !ok {
        http.Error(w, "unsupported proxy target", http.StatusBadRequest)
        return
    }
    webproxy.Serve(w, r, webproxy.Options{
        Base:         base, // scheme://host
        Transport:    s.transport,
        UpstreamPath: upstreamPath,
        PublicPrefix: plugin.RequestProxyPrefix(r),
    })
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
    return map[string]any{"url": rc.ProxyURL(rc.Param("port"))}, nil
}
```

`rc.ProxyURL(segments...)` joins the core-supplied mount with path-escaped
segments and a trailing slash; `rc.ProxyPrefix()` returns the bare mount. Bind
the route to an `Action` with `Open: plugin.OpenURL`.

## Embed as a panel

Use `PanelWebProxy` when the proxied surface should live inside the workspace
instead of opening a separate browser tab:

```go
plugin.Panel{
    Key:   "web",
    Label: "Web",
    Type:  plugin.PanelWebProxy,
    Config: plugin.WebProxyConfig{
        Path: "/",
        Capabilities: []plugin.WebProxyCapability{
            plugin.WebProxyCapabilityClipboard,
            plugin.WebProxyCapabilityFullscreen,
        },
    },
}
```

`Path` is relative to the connection proxy mount and must start with `/`. Do not
put an absolute URL in the manifest; the gateway builds the per-connection URL.
The panel renders the proxied surface in a sandboxed iframe and only enables
extra browser privileges through named capabilities:

- `Clipboard` allows clipboard read/write.
- `Downloads` allows browser downloads.
- `Fullscreen` allows fullscreen requests.
- `Popups` allows popup windows.
- `SameOrigin` removes opaque-origin isolation. Use it only for trusted,
  connection-owned surfaces that cannot work without browser origin storage.

Set `OpenExternal: true` only when the embedded app needs a larger separate tab.
Inline rendering should be the default.

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

Use `sdk/plugin/webproxy` for the common rewrite path. If a target app has
unusual runtime behavior, add the smallest plugin-local handling around that
helper and test the target UI through the gateway prefix.

## Notes

- **Auth and audit still apply** - the gateway authenticates every proxied request
  against the connection before calling you; you don't re-check.
- **Set `FlushInterval: -1`** so streamed responses (logs, server-sent events)
  aren't buffered.
- **Preserve percent-encoding** when the sub-path can contain encoded characters:
  read `r.URL.EscapedPath()` for the raw form and set `req.URL.RawPath`.
