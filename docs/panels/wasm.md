# PanelWasm

Use `PanelWasm` for isolated browser-side programs that need their own runtime:
existing Go/Rust/Flutter apps, heavy visualizers, simulators, games, protocol
tools that are already compiled to WASM, or sandboxed raw JavaScript/framework
apps that cannot be expressed with the standard manifest panels.

Do not use it just to get custom UI. If a standard panel can represent the
workflow, prefer the standard panel. The core renderer gives standard panels
accessibility, loading states, actions, exports, storage pairing, and theming for
free.

The core owns the iframe sandbox, asset loading, CSP, auth, route proxying, and
stream restrictions. The plugin only declares the WASM panel, the files it needs,
and the explicit bridge routes/streams the app may call.

## Minimal Go WASM panel

Go WASM usually needs `wasm_exec.js` plus the `.wasm` binary.

```go
plugin.Panel{
    Key:   "wasm",
    Label: "WASM Console",
    Icon:  plugin.Icon{Type: plugin.IconLucide, Value: "component"},
    Type:  plugin.PanelWasm,
    Config: plugin.WasmConfig{
        Entry:     "app.wasm",
        Runtime:   plugin.WasmRuntimeGo,
        Boot:      plugin.WasmBoot{Scripts: []string{"wasm_exec.js"}},
        ScaleMode: plugin.WasmScaleScroll,
        Capabilities: plugin.WasmCapabilities{
            Keyboard:   true,
            Pointer:    true,
            Fullscreen: true,
        },
        Assets: []plugin.WasmAsset{
            wasmAsset("wasm_exec.js", "text/javascript"),
            wasmAsset("app.wasm", "application/wasm"),
        },
        Bridge: plugin.WasmBridge{
            Routes: []plugin.WasmBridgeRoute{
                {RouteID: "myplugin.state", Method: plugin.MethodGet},
                {RouteID: "myplugin.score", Method: plugin.MethodPost},
            },
            Streams: []plugin.WasmBridgeStream{
                {RouteID: "myplugin.events"},
            },
        },
        AriaLabel:    "WASM demo console",
        Instructions: "Use the embedded controls and keyboard shortcuts inside the sandbox.",
    },
}
```

Declare every file in `Assets`. The renderer only serves assets listed in the
manifest projection.

```go
func wasmAsset(name, mime string) plugin.WasmAsset {
    return plugin.WasmAsset{
        Path: name,
        MIME: mime,
        Source: plugin.DataSource{
            RouteID: "myplugin.asset",
            Params:  map[string]string{"path": name},
        },
    }
}
```

## Route-backed assets

Serve WASM files through normal plugin routes. This keeps permissions, audit,
and content type handling inside the ShellCN route wrapper.

```go
//go:embed assets/*
var assets embed.FS

func assetRoute(rc *plugin.RequestContext) (any, error) {
    name := path.Clean(strings.TrimPrefix(rc.Param("path"), "/"))
    if name == "." || strings.Contains(name, "..") {
        return nil, plugin.ErrInvalidInput
    }

    data, err := assets.ReadFile("assets/" + name)
    if err != nil {
        return nil, fmt.Errorf("%w: asset not found", plugin.ErrNotFound)
    }

    mime := "application/octet-stream"
    switch path.Ext(name) {
    case ".js":
        mime = "text/javascript"
    case ".wasm":
        mime = "application/wasm"
    case ".json":
        mime = "application/json"
    }

    return &plugin.Download{
        Name:    name,
        MIME:    mime,
        Size:    int64(len(data)),
        Inline:  true,
        Body:    io.NopCloser(bytes.NewReader(data)),
        ModTime: time.Now(),
    }, nil
}
```

Register that route like any other route:

```go
plugin.Route{
    ID: "myplugin.asset", Method: plugin.MethodGet, Path: "/asset",
    Permission: "myplugin.read", Risk: plugin.RiskSafe, AuditEvent: "myplugin.asset",
    Handle: assetRoute,
}
```

Keep the path fixed behind a route param such as `path`. Never pass arbitrary
filesystem paths to `ReadFile`, never allow `..`, and keep the assets embedded or
otherwise immutable.

## Generic runtimes

Use `WasmRuntimeGeneric` for Rust, wasm-bindgen, Emscripten, Flutter, raw
JavaScript, browser framework bundles, and custom loaders. Put generated
JavaScript loaders in both `Boot.Scripts` and `Assets`.

With `Runtime: plugin.WasmRuntimeGeneric`, boot scripts own startup. If boot
scripts are present, the renderer loads the bridge, loads those scripts, and
does not auto-instantiate `Entry`. That is what makes generic runtimes work:
Rust glue, Flutter loaders, Emscripten output, React/Vue/Svelte bundles, or
plain browser JavaScript can decide how and when to load their own assets.

```go
plugin.WasmConfig{
    Entry:     "app_bg.wasm",
    Runtime:   plugin.WasmRuntimeGeneric,
    ScaleMode: plugin.WasmScaleScroll,
    Boot:      plugin.WasmBoot{Scripts: []string{"app.js", "boot.js"}},
    Assets: []plugin.WasmAsset{
        wasmAsset("app.js", "text/javascript"),
        wasmAsset("app_bg.wasm", "application/wasm"),
        wasmAsset("boot.js", "text/javascript"),
    },
    Bridge: plugin.WasmBridge{Routes: []plugin.WasmBridgeRoute{
        {RouteID: "myplugin.todos.list", Method: plugin.MethodGet},
        {RouteID: "myplugin.todos.save", Method: plugin.MethodPost},
        {RouteID: "myplugin.todos.delete", Method: plugin.MethodDelete},
    }},
    Capabilities: plugin.WasmCapabilities{Keyboard: true, Pointer: true},
}
```

### Raw JavaScript or framework bundles

`PanelWasm` can also host a sandboxed browser app that has no `.wasm` file. This
is useful for a complex visual tool that needs a framework runtime but must stay
isolated from the ShellCN frontend. Use this sparingly; if `PanelCanvas`,
`PanelGraph`, `PanelDashboard`, or another standard panel can model the UI, use
the standard panel.

For a pure JavaScript bundle, set `Runtime` to `WasmRuntimeGeneric`, declare the
main app script as `Entry`, include it in `Boot.Scripts`, and list every JS, CSS,
font, image, and data file in `Assets`.

```go
plugin.WasmConfig{
    Entry:     "app.js",
    Runtime:   plugin.WasmRuntimeGeneric,
    ScaleMode: plugin.WasmScaleScroll,
    Boot:      plugin.WasmBoot{Scripts: []string{"vendor.js", "app.js"}},
    Assets: []plugin.WasmAsset{
        wasmAsset("vendor.js", "text/javascript"),
        wasmAsset("app.js", "text/javascript"),
        wasmAsset("styles.css", "text/css"),
        wasmAsset("icons.svg", "image/svg+xml"),
    },
    Bridge: plugin.WasmBridge{
        Routes: []plugin.WasmBridgeRoute{
            {RouteID: "myplugin.state", Method: plugin.MethodGet},
            {RouteID: "myplugin.save", Method: plugin.MethodPost},
        },
        Streams: []plugin.WasmBridgeStream{
            {RouteID: "myplugin.events"},
        },
    },
    Capabilities: plugin.WasmCapabilities{Keyboard: true, Pointer: true},
    AriaLabel:    "Custom visual operations app",
}
```

Inside `app.js`, mount your framework normally and use `window.shellcn` for all
ShellCN access:

```js
const root =
  document.getElementById("app") ||
  document.body.appendChild(document.createElement("div"));
root.id = "app";

const state = await window.shellcn.route(
  "myplugin.state",
  {},
  { method: "GET" },
);

window.shellcn.onTheme((theme, colors) => {
  document.body.dataset.theme = theme;
  document.documentElement.style.setProperty(
    "--accent",
    colors.primary500 || "#38bdf8",
  );
});

renderApp(root, {
  state,
  save: (body) =>
    window.shellcn.route("myplugin.save", body, { method: "POST" }),
  events: () => window.shellcn.stream("myplugin.events"),
});
```

Do not use ambient `fetch("/...")`, cookies, `localStorage`, or parent-window
DOM access. The iframe has an opaque origin and a restrictive CSP. Load declared
files with `window.shellcn.asset()` or `window.shellcn.assetURL()`, and call
backend behavior through allowlisted bridge routes and streams.

For Flutter-style bundles, include the runtime files and any generated asset
tree the loader needs:

```go
plugin.WasmConfig{
    Entry:   "main.dart.wasm",
    Runtime: plugin.WasmRuntimeGeneric,
    Boot: plugin.WasmBoot{Scripts: []string{
        "shellcn_flutter_pre.js",
        "flutter.js",
        "shellcn_flutter_run.js",
    }},
    Assets: []plugin.WasmAsset{
        wasmAsset("main.dart.wasm", "application/wasm"),
        wasmAsset("flutter.js", "text/javascript"),
        wasmAsset("shellcn_flutter_pre.js", "text/javascript"),
        wasmAsset("shellcn_flutter_run.js", "text/javascript"),
        wasmAsset("AssetManifest.json", "application/json"),
        wasmAsset("FontManifest.json", "application/json"),
    },
}
```

If the generated bundle contains many files, build the `Assets` slice by walking
the embedded directory at startup. Keep the resulting `Path` values relative to
the WASM asset root and stable across releases.

## Bridge routes

The sandbox does not get raw access to plugin HTTP routes. It can only call
routes declared in `Bridge.Routes` and streams declared in `Bridge.Streams`.

```go
Bridge: plugin.WasmBridge{
    Routes: []plugin.WasmBridgeRoute{
        {RouteID: "myplugin.state", Method: plugin.MethodGet},
        {RouteID: "myplugin.save", Method: plugin.MethodPost},
    },
    Streams: []plugin.WasmBridgeStream{
        {RouteID: "myplugin.events"},
    },
}
```

The bridge allowlist is part of the security boundary:

- Match the method the app will use. A `GET` allowlist entry does not permit
  `POST`.
- Keep read/write/delete permissions and risk levels correct on the underlying
  route.
- Add `rc.Audit` inside long-lived bridge streams when one stream can perform
  more than one meaningful operation.
- Treat all data from the WASM app as user input. Bind and validate it exactly
  as you would validate a normal HTTP request.

## Bridge API inside the sandbox

The iframe gets a small `window.shellcn` object. This object is the only
supported way for WASM JavaScript glue to talk back to ShellCN.

```js
window.shellcn.entry; // primary WASM path from WasmConfig.Entry
window.shellcn.capabilities; // declared capabilities
window.shellcn.theme; // "light" or "dark"
window.shellcn.colors; // ShellCN primary/surface color tokens
window.shellcn.onTheme(fn); // subscribe to theme/color changes, returns unsubscribe
window.shellcn.route(id, body, options);
window.shellcn.asset(path);
window.shellcn.assetURL(path, mime);
window.shellcn.stream(id, params);
```

Route calls return a promise:

```js
const state = await window.shellcn.route(
  "myplugin.state",
  {},
  { method: "GET", params: { namespace: "default" } },
);
```

The route id and method must be declared in `Bridge.Routes`. Params from the
bridge declaration and params passed by the app are merged by the renderer before
calling the gateway.

Asset calls return an `ArrayBuffer`; `assetURL` returns a blob URL for loaders
that need a URL:

```js
const wasmBytes = await window.shellcn.asset(window.shellcn.entry);
const fontURL = await window.shellcn.assetURL(
  "assets/fonts/Roboto-Regular.ttf",
  "font/ttf",
);
```

Stream calls return a handle:

```js
const stream = window.shellcn.stream("myplugin.events");
const off = stream.onMessage((data) => {
  console.log("event", data);
});
stream.send({ hello: "from wasm" });
stream.close();
off();
```

`stream.send` sends strings as-is and JSON-encodes other values. Stream messages
from the backend arrive as browser WebSocket message data, usually a string.

## Theme

WASM panels receive the current ShellCN theme and token colors through the
bridge. `window.shellcn.theme` is `"light"` or `"dark"`.
`window.shellcn.colors` is a plain object containing primary and surface tokens
such as `primary500`, `surface0`, `surface900`, and `surface950`.

```js
function applyTheme(theme, colors = window.shellcn.colors || {}) {
  document.body.dataset.theme = theme === "light" ? "light" : "dark";
  document.documentElement.style.setProperty(
    "--shellcn-primary",
    colors.primary500 || "#38bdf8",
  );
  document.documentElement.style.setProperty(
    "--shellcn-bg",
    colors.surface950 || "#020617",
  );
  document.documentElement.style.setProperty(
    "--shellcn-text",
    colors.surface100 || "#e2e8f0",
  );
}

applyTheme(window.shellcn.theme, window.shellcn.colors);
window.shellcn.onTheme(applyTheme);
```

Use CSS variables inside the sandbox:

```css
:root {
  color-scheme: dark;
  --bg: #020617;
  --surface: #0f172a;
  --text: #e2e8f0;
  --accent: #38bdf8;
}

body[data-theme="light"] {
  color-scheme: light;
  --bg: #f8fafc;
  --surface: #ffffff;
  --text: #0f172a;
  --accent: #0284c7;
}
```

The sandbox cannot read ShellCN parent CSS variables or Tailwind classes. Use the
bridge-provided `colors` object instead of reaching into the parent DOM. Theme is
a visual hint only; route behavior, permissions, and storage keys must not depend
on it.

## Storage-backed WASM apps

WASM apps should persist user-created records through normal bridge routes backed
by `rc.Storage`. Do not use browser `localStorage` or cookies; the iframe runs
with an opaque origin and ShellCN owns persistence.

```go
func saveTodo(rc *plugin.RequestContext) (any, error) {
    var in struct {
        Key   string `json:"key"`
        Title string `json:"title"`
        Done  bool   `json:"done"`
    }
    if err := rc.Bind(&in); err != nil {
        return nil, err
    }
    in.Title = strings.TrimSpace(in.Title)
    if in.Title == "" {
        return nil, fmt.Errorf("%w: title is required", plugin.ErrInvalidInput)
    }
    key := strings.TrimSpace(in.Key)
    if key == "" {
        key = uuid.NewString()
    }

    raw, err := json.Marshal(in)
    if err != nil {
        return nil, err
    }
    stored, err := rc.Storage.Put(rc.Ctx, "todos", plugin.StorageItem{
        Key:         key,
        Value:       raw,
        ContentType: "application/json",
        Metadata:    map[string]string{"title": in.Title},
    })
    if err != nil {
        return nil, err
    }
    return storedTodo(stored), nil
}
```

See [plugin storage](../storage.md) for scoping and test patterns.

## Scale and capabilities

`ScaleMode` controls how the sandbox is laid out:

- `WasmScaleScroll`: preserve the app's natural size and scroll when needed.
- `WasmScaleFit`: fit the app into the panel.
- `WasmScaleResize`: let the app react to the current panel size.

Declare `Width` and `Height` together. Fixed dimensions should normally use
`WasmScaleFit` or `WasmScaleScroll`; a full-panel app should leave both empty
and use resize mode/default layout.

Declare only the capabilities the app needs:

```go
Capabilities: plugin.WasmCapabilities{
    Keyboard:   true,
    Pointer:    true,
    Fullscreen: true,
    Gamepad:    true,
}
```

Capabilities are UX and permission signals for the renderer. They are not a
replacement for route authorization.

## App assumptions

WASM code runs isolated from the ShellCN frontend:

- Do not depend on parent DOM access.
- Do not depend on cookies, browser `localStorage`, or ambient relative fetches.
- Load assets through declared `WasmAsset` entries.
- Call ShellCN through declared bridge routes and streams.
- Keep bundle paths deterministic. The manifest is the contract the renderer
  uses to preload and authorize files.

## Testing checklist

- Manifest validation passes with all assets, routes, and streams declared.
- Every `WasmAsset.Source.RouteID` exists in `Routes()`.
- `Width` and `Height` are either both empty or both positive, and fixed-size
  panels declare `WasmScaleFit` or `WasmScaleScroll`.
- The asset route rejects `..`, empty paths, and missing files.
- Content types are correct for `.wasm`, `.js`, `.json`, fonts, images, and any
  framework-specific files.
- Bridge write/delete routes have non-safe risk levels and useful audit events.
- Storage-backed bridge routes have handler tests with a fake `plugin.Storage`.
- The app applies `window.shellcn.theme` on startup and responds to `onTheme`
  without requiring a page reload.
