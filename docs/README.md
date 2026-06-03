# Plugin authoring guide

This guide explains how to build a ShellCN protocol plugin: the model, the
contract, and every capability - from a simple CRUD list to interactive
terminals tunnelled through an agent.

## What a plugin is

A plugin is a **standalone Go program**. It does not import the gateway and the
gateway does not import it. They share only the **plugin SDK**
(`github.com/charlesng35/shellcn/sdk`), which defines the contract between them.

At runtime the gateway launches your compiled binary as a **subprocess** and
talks to it over gRPC (via [hashicorp/go-plugin](https://github.com/hashicorp/go-plugin),
with mutually-authenticated local transport). Because it's a separate process,
a misbehaving plugin can't corrupt the gateway, and you can build and ship it on
your own schedule.

```
  Browser ──HTTP/WS──> ShellCN gateway ──gRPC subprocess──> your plugin
                         (auth, audit,                       (manifest,
                          policy, UI,                          routes,
                          transport)                           session)
```

## The three things you implement

Every plugin implements one interface - `plugin.Plugin`:

```go
type Plugin interface {
    Manifest() Manifest                                       // describe yourself
    Routes() []Route                                          // your endpoints
    Connect(ctx, ConnectConfig) (Session, error)              // open a connection
}
```

- **[Manifest](manifest.md)** - pure data describing your plugin and the UI the
  gateway should render for it (panels, forms, actions, transports). The
  frontend is a universal renderer; it draws whatever the manifest declares.
  There is no per-plugin UI code.
- **[Routes](routes.md)** - your endpoints. Each one carries the metadata the
  gateway enforces (permission, risk, audit, input schema) and a handler that is
  pure business logic - it never touches HTTP, auth, or headers.
- **[Sessions](sessions.md)** - `Connect` opens a live, per-connection runtime
  (the `Session`) that holds all state for one connection. The plugin value
  itself is stateless and shared across all connections.

## What the gateway does for you

You write protocol logic; the gateway owns everything around it:

- **Authentication & authorization** - every request is authenticated and
  checked against the route's permission/risk before your handler runs.
- **Audit** - each route names an audit event; the gateway records it.
- **Network egress** - the plugin never dials the target directly. The gateway
  hands your session a transport (`cfg.Net`) that reaches the target whether the
  connection is **direct** or tunnelled through an **[agent](agents.md)**. Egress
  stays in the gateway, so it remains the single audited choke point.
- **UI** - panels, tables, forms, trees, terminals, and the connection form are
  all rendered from your manifest.
- **Recording, sessions, transport, TLS** - all core-owned.

## Read next

1. [Manifest](manifest.md) - start here; it's most of a plugin.
2. [Routes](routes.md) - handlers, input, validation, errors.
3. [Sessions](sessions.md) - state and reaching the target.
4. [Streaming](streaming.md) - terminals, logs, channels, recording.
5. [Explorer & database plugins](explorer.md) - trees, scope filters, sorting,
   selectable rows, editable grids, query editors.
6. [Agents](agents.md) - reaching targets in a private network.
7. [Build & install](build-and-install.md) - compile, ship, load, version.
8. [Best practices](best-practices.md) - conventions distilled from the built-ins.
