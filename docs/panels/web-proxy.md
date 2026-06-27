# PanelWebProxy

Use `PanelWebProxy` when a connection exposes a browser-based tool that should
render inside the ShellCN workspace: an admin console, web terminal, notebook,
developer IDE, dashboard, or service UI already served by the target.

`PanelWebProxy` does not let a plugin ship frontend code. The plugin implements
the optional `plugin.HTTPProxy` session capability, and the core renderer embeds
the connection-scoped proxy URL in a sandboxed iframe.

## Minimal panel

```go
plugin.Panel{
    Key:   "web",
    Label: "Web",
    Icon:  plugin.Icon{Type: plugin.IconLucide, Value: "panel-top"},
    Type:  plugin.PanelWebProxy,
    Config: plugin.WebProxyConfig{
        Path: "/",
        Capabilities: []plugin.WebProxyCapability{
            plugin.WebProxyCapabilityClipboard,
            plugin.WebProxyCapabilityFullscreen,
        },
        AriaLabel:    "Target web interface",
        Instructions: "Use the embedded target web interface.",
    },
}
```

`Path` is relative to the connection proxy mount. It must start with `/` and
must not be an absolute URL. The gateway builds the real browser URL from the
active connection ID.

## Session proxy

A session opts in by implementing `ServeHTTPProxy`. For most web apps, use the
SDK helper:

```go
import "github.com/charlesng35/shellcn/sdk/plugin/webproxy"

type Session struct {
    transport plugin.NetTransport
    target    string // host:port reachable through cfg.Net
}

func (s *Session) ServeHTTPProxy(w http.ResponseWriter, r *http.Request) {
    upstream := &url.URL{Scheme: "http", Host: s.target}
    webproxy.Serve(w, r, webproxy.Options{
        Base:         upstream,
        Transport:    s.transport,
        UpstreamPath: r.URL.Path,
        PublicPrefix: plugin.RequestProxyPrefix(r),
    })
}
```

Always build the proxy transport from `cfg.Net`, not `http.DefaultTransport`.
That keeps outbound traffic inside the configured connection transport.

`webproxy.Serve` handles the common redirect, cookie path, root-relative asset,
CSS URL, framing header, WebSocket URL, and service-worker rewriting needed by
single-page apps. It also has opt-in `WebSocketOptions` for upstreams that reject
gateway-origin WebSocket upgrades or forwarded proxy headers. See the top-level
[web proxy guide](../web-proxy.md) for the full proxy model.

## Capabilities

The iframe starts with a restricted sandbox. Add capabilities only when the
target UI needs them:

| Capability | Browser permission |
| --- | --- |
| `WebProxyCapabilityClipboard` | Clipboard read/write. |
| `WebProxyCapabilityDownloads` | Browser downloads. |
| `WebProxyCapabilityFullscreen` | Fullscreen requests. |
| `WebProxyCapabilityPopups` | Popup windows. |
| `WebProxyCapabilitySameOrigin` | Non-opaque iframe origin. |

`SameOrigin` is the sensitive one. Use it only for trusted, connection-owned
surfaces that need browser origin storage or strict same-origin behavior. It is
not a protection boundary against malicious plugin code.

## Open in a separate tab

Keep inline rendering as the default. Set `OpenExternal: true` only when the
target UI benefits from a larger browser tab:

```go
plugin.WebProxyConfig{
    Path:         "/",
    OpenExternal: true,
}
```

The new tab still uses the ShellCN connection proxy and still goes through
gateway authentication and authorization.

## Security checklist

- Prefer `TransportAgent` for host-local targets on shared gateways.
- Do not proxy arbitrary user-provided hosts or paths.
- Do not hardcode `/api/connections/.../proxy`; use
  `plugin.RequestProxyPrefix(r)` inside `ServeHTTPProxy` and `rc.ProxyURL(...)`
  in normal route handlers.
- Strip or rewrite upstream framing and CSP headers only for the proxied
  response.
- Keep upstream editor/admin-console authentication disabled only when ShellCN is
  the exclusive access path. Never expose that upstream port publicly.
- Treat browser IDEs and terminals as privileged because they can execute code on
  whichever machine runs the upstream process.
