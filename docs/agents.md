# Agent transport

Some targets aren't reachable directly from the gateway - they live in a private
network, behind NAT, or on a host you can only reach from the inside. ShellCN's
**agent transport** solves this: a small agent binary runs _next to the target_
and the gateway tunnels through it. A plugin opts in declaratively; the dialing
code stays identical.

This template is direct-only. Here's how to add agent support.

## How it works

```
Browser -> gateway -> (agent tunnel) -> agent on target host -> target service
```

The gateway owns the tunnel, enrollment, and the agent binary - there is **one**
shared agent (`shellcn-agent`), not a per-plugin one. Your plugin only:

1. declares `TransportAgent` and an `AgentProfile`, and
2. dials through `cfg.Net` as usual - **the same code as direct**.

Whether `cfg.Net` routes directly or through the agent is invisible to your
handler. That's the point: you write the dial once.

## The four proxy modes

`ProxyTarget.Mode` says what the agent exposes back to the gateway. Pick the one
that matches your protocol's layer:

| Mode               | The agent exposes...                       | Reach it with         |
| ------------------ | ------------------------------------------ | --------------------- |
| `AgentTCP`         | a TCP endpoint on the target side          | `cfg.Net.DialContext` |
| `AgentUnix`        | a Unix socket on the target host           | `cfg.Net.DialContext` |
| `AgentHTTP`        | an HTTP proxy to a target-side web service | `cfg.Net.HTTP()`      |
| `AgentHostMonitor` | the agent's **own** host-metrics HTTP API  | `cfg.Net.HTTP()`      |

The first three forward to a separate target service. **`AgentHostMonitor` is
different**: there's no target behind the agent - the agent _is_ the data source
(it reports the host's CPU, memory, processes). The gateway exposes the agent's
small HTTP API through `cfg.Net.HTTP()`, and your plugin talks to that.

## Declaring it

```go
Manifest{
    SupportedTransports: []plugin.Transport{plugin.TransportDirect, plugin.TransportAgent},
    Agent: &plugin.AgentProfile{
        Proxy: plugin.ProxyTarget{
            Mode:    plugin.AgentTCP,        // what the agent proxies on the target side
            Address: "127.0.0.1:5432",       // default target address, as seen by the agent
            Risk:    plugin.RiskPrivileged,  // opening a tunnel is privileged
        },
        Install: []plugin.InstallArtifact{
            {Label: "Docker", Kind: "docker-run", Template: dockerRunTemplate},
        },
    },
}
```

When the user picks the **agent** transport in the connection form, the gateway
renders an enrollment panel from `Install` (no UI code from you), mints a
one-time token, and waits for the agent to connect back.

Because the agent supplies the endpoint, hide your direct-only config fields
(host, port, socket path) under the agent transport with a `VisibleWhen`
condition on `$transport`. See the transport-conditional example in
[manifest.md](manifest.md#conditions-visiblewhen).

### ProxyTarget

| Field                  | Purpose                                                                            |
| ---------------------- | ---------------------------------------------------------------------------------- |
| `Mode`                 | One of the four modes above.                                                       |
| `Address`              | Default target address from the agent's vantage point (empty for host-monitor).    |
| `Risk`                 | Risk of opening the tunnel (usually `RiskPrivileged`; host-monitor is `RiskSafe`). |
| `Forward`              | Allow per-stream target addresses instead of only `Address`.                       |
| `TokenFile` / `CAFile` | Optional credential/CA paths on the agent host.                                    |

### Branch on the transport when you must

For most plugins the dial is identical and you branch on nothing. Host-monitor is
the exception - direct collects locally, agent talks to the agent's HTTP API:

```go
func Connect(ctx context.Context, cfg plugin.ConnectConfig) (plugin.Session, error) {
    if cfg.Transport == plugin.TransportAgent {
        base, rt, ok := cfg.Net.HTTP()
        if !ok {
            return nil, fmt.Errorf("%w: agent exposes no HTTP transport", plugin.ErrUnavailable)
        }
        return newRemote(base, &http.Client{Transport: rt}), nil
    }
    return newLocal(cfg), nil
}
```

## Install artifacts

Each `InstallArtifact` is a launch recipe shown to the operator (a `docker run`
line, a native command, a PowerShell line). The gateway renders `Template` as a
Go `text/template` with these variables and a `shellquote` function:

| Variable                 | What it is                                                        |
| ------------------------ | ----------------------------------------------------------------- |
| `.ConnectURL`            | The URL the agent dials back (localhost-rewritten if needed).     |
| `.GatewayConnectURL`     | The raw, un-rewritten connect URL.                                |
| `.Token`                 | The one-time enrollment token.                                    |
| `.Image`                 | The agent's container image.                                      |
| `.Slug`                  | A DNS-safe per-connection id (name targets without collisions).   |
| `.Insecure`              | True when the connect URL is `ws://` (not `wss://`).              |
| `.LocalhostHost`         | The host to substitute for `localhost` (see below).               |
| `.LocalhostHostRequired` | True when the connect URL pointed at localhost and was rewritten. |

Always `shellquote` interpolated values so a token or URL can't break the command:

```go
Template: "docker run --rm --name " + plugin.AgentBinary + " " +
    "{{if .LocalhostHostRequired}}--add-host={{.LocalhostHost}}:host-gateway {{end}}" +
    "-e SHELLCN_CONNECT_URL={{shellquote .ConnectURL}} " +
    "{{if .Insecure}}-e SHELLCN_INSECURE=1 {{end}}" +
    "-e SHELLCN_ENROLL_TOKEN={{shellquote .Token}} " +
    "{{shellquote .Image}}",
```

Reference the gateway's agent identifiers from the SDK rather than hard-coding
strings: `plugin.AgentBinary` (`shellcn-agent`), `plugin.AgentImageLatest` (the
published image). Ship a few artifacts so operators on Docker, a bare host, and
Windows each have a copy-paste line - the server-monitor plugin ships Docker,
native-binary, and PowerShell variants.

**`LocalhostHost`** handles the common "gateway on `localhost`" case: when the
connect URL points at localhost, an agent in a container can't reach it, so the
gateway rewrites it to `LocalhostHost` (e.g. `host.docker.internal`) and sets
`LocalhostHostRequired`, which your template uses to add the matching host entry.

### Large or sensitive payloads

For a long systemd unit or a script you don't want inlined with the token, set
`Delivery: plugin.DeliveryURL` and put the body in `Content`. The gateway serves
it from a **single-use signed URL** (available as `.ArtifactURL`) instead of
inlining the token into the command. The default, `DeliveryInline`, just renders
`Template`.

## Enrollment flow

1. User picks **Agent** in the connection form and opens the enrollment panel.
2. The gateway mints a one-time token and renders your `Install` artifacts.
3. The operator runs one on the target host; the agent dials `.ConnectURL` and
   presents the token.
4. The gateway verifies it, establishes the mutually-authenticated tunnel, and
   marks the connection ready. The token is now spent.

From then on, `Connect` and every dial go through that tunnel via `cfg.Net`.

## Dialing - unchanged

```go
// Works for direct AND agent connections; cfg.Net is wired by the gateway.
conn, err := cfg.Net.DialContext(ctx, "tcp", cfg.String("host")+":"+port)
```

For L7 backends, `cfg.Net.HTTP()` returns a RoundTripper that routes through the
agent's HTTP proxy when the connection is agent-mode.

## Trust

An agent opens a path _into_ the operator's network, so it's a privileged
capability. Enrollment, the tunnel, and the agent binary are all gateway-owned;
your plugin only declares the profile. Operators control which protocols are
available at all from **Settings -> Protocols**.
