# PanelHTTPClient

Use `PanelHTTPClient` for REST-like targets where users need to compose a
request and inspect the response through the gateway transport.

```go
plugin.Panel{
    Key: "client", Label: "Client", Icon: icon("send"),
    Type: plugin.PanelHTTPClient,
    Config: plugin.HTTPClientConfig{
        ExecuteRouteID: "demo.http.execute",
        Methods:       []string{"GET", "POST", "PUT", "DELETE"},
        DefaultMethod: "GET",
        DefaultURL:    "/api/v1/status",
        DefaultHeaders: []plugin.HeaderDefault{{Key: "Accept", Value: "application/json"}},
    },
}
```

The execute route is called with `POST`. It receives the request method,
URL/path, headers, and body from the panel:

```json
{
  "method": "GET",
  "url": "/api/v1/status",
  "headers": [{ "key": "Accept", "value": "application/json" }],
  "body": ""
}
```

Return a stable response object:

```json
{
  "status": 200,
  "statusText": "OK",
  "durationMs": 18.4,
  "headers": [{ "key": "Content-Type", "value": "application/json" }],
  "body": { "ok": true }
}
```

`headers` may also be a string map. `body` may be a string or any JSON value.

Validate allowed hosts and paths server-side. Do not let the panel become an
arbitrary SSRF primitive; route through the connection's configured target and
`cfg.Net`.

Saved request templates should be implemented with `rc.Storage`.
