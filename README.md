# ShellCN plugin starter

A **GitHub template repository** for building an **external (out-of-tree) protocol
plugin** for [ShellCN](https://github.com/CharlesNg35/shellcn). Press
**Use this template** to generate your own plugin repo from it.

ShellCN ships ~40 built-in protocols. When you need one that isn't built in, you
don't touch the gateway - you write a small Go program against the **plugin SDK**,
compile it, and drop the binary into the gateway's plugin directory. The gateway
runs it as a subprocess and renders it through the same universal UI, auth,
audit, and policy as a built-in. No gateway changes, no frontend code.

This repo is a working example (a tiny in-memory key/value store) you copy and
make your own.

## Quick start

Generate your repo with **Use this template** (above), then clone _your_ repo and
build - the plugin SDK is pulled from the Go proxy like any other dependency (you
don't need the gateway's source):

```sh
git clone https://github.com/<you>/<your-plugin>.git
cd <your-plugin>
go build -o starter .     # produces the plugin binary
```

Then load it into a running gateway - use a release binary or Docker, you don't
need the gateway's source (see the [ShellCN README](https://github.com/CharlesNg35/shellcn)):

1. Copy `starter` into the gateway's plugin directory (`plugins.dir`, default
   `plugins.d/` - see [docs/build-and-install.md](docs/build-and-install.md)).
2. Restart the gateway.
3. As an admin, open **Settings → Protocols** and confirm it's enabled.

It now appears in the connection catalog like any built-in protocol.

## Make it yours

1. Rename the module in `go.mod` and the plugin `Name`/`Title` in `manifest.go`.
2. Replace the key/value example in `manifest.go` + `session.go` with your
   protocol's manifest, routes, and session logic.
3. Tag a release - `.github/workflows/release.yml` cross-compiles the binary for
   every gateway platform and attaches it (with checksums) to a GitHub Release.

## Where things live

| File          | What it is                                              |
| ------------- | ------------------------------------------------------- |
| `main.go`     | The entry point - hands the plugin to `sdk.Serve`.      |
| `manifest.go` | What the gateway sees: manifest, routes, `Connect`.     |
| `session.go`  | The per-connection runtime and the route handlers.      |
| `docs/`       | The full authoring guide - read this to go beyond CRUD. |

## Learn the model

[**docs/**](docs/) walks through every part in depth:

- [Overview](docs/README.md) - how a plugin and the gateway fit together.
- [Manifest](docs/manifest.md) - describing your plugin and its UI.
- [Routes](docs/routes.md) - endpoints, input, validation, errors.
- [Sessions](docs/sessions.md) - per-connection state and reaching the target.
- [Streaming](docs/streaming.md) - terminals, logs, channels, recording.
- [Explorer & database plugins](docs/explorer.md) - trees, scope filters, sorting, editable grids, query editors.
- [File browser plugins](docs/file-browser.md) - the file-manager UI: listing, range downloads, uploads.
- [Metrics & dashboards](docs/metrics.md) - live stat cards, gauges, charts, and overview grids.
- [Web proxy](docs/web-proxy.md) - embed a target's web UI through the gateway.
- [Agents](docs/agents.md) - tunnelling into a private network.
- [Build & install](docs/build-and-install.md) - compile, ship, load, version.
- [Best practices](docs/best-practices.md) - conventions from the built-in plugins.

## License

See the main [ShellCN](https://github.com/CharlesNg35/shellcn) repository.
