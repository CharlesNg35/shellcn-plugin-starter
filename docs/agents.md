# Agent transport

Some targets aren't reachable directly from the gateway — they live in a private
network, behind NAT, or on a host you can only reach from the inside. ShellCN's
**agent transport** solves this: a small agent binary runs *next to the target*
and the gateway tunnels through it. A plugin opts in declaratively; the dialing
code stays identical.

This template is direct-only. Here's how to add agent support.

## How it works

```
Browser → gateway → (agent tunnel) → agent on target host → target service
```

The gateway owns the tunnel, enrollment, and the agent binary. Your plugin only:

1. declares `TransportAgent` and an `AgentProfile`, and
2. dials through `cfg.Net` as usual — **the same code as direct**.

Whether `cfg.Net` routes directly or through the agent is invisible to your
handler. That's the point: you write the dial once.

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
            {Label: "Docker", Kind: "docker", Template: "docker run ... {{.ConnectURL}} {{.Token}}"},
        },
    },
}
```

When the user picks the **agent** transport in the connection form, the gateway
renders an enrollment panel from `Install` (no UI code from you), mints a
one-time token, and waits for the agent to connect back.

### ProxyTarget

| Field       | Purpose                                                            |
| ----------- | ------------------------------------------------------------------ |
| `Mode`      | `AgentTCP`, `AgentUnix`, `AgentHTTP` (L7 proxy), `AgentHostMonitor`. |
| `Address`   | Default target address from the agent's vantage point.             |
| `Risk`      | Risk of opening the tunnel (usually `RiskPrivileged`).             |
| `Forward`   | Allow per-stream target addresses instead of only `Address`.       |
| `TokenFile` / `CAFile` | Optional credential/CA paths on the agent host.         |

### InstallArtifact

Each artifact is a launch recipe shown to the operator (a `docker run` line, a
systemd unit, a script). `Template` is filled with `{{.ConnectURL}}` and
`{{.Token}}`. For large or sensitive payloads use `Delivery: DeliveryURL` and put
the body in `Content` — the gateway serves it from a single-use signed URL
instead of inlining the token.

## Dialing — unchanged

```go
// Works for direct AND agent connections; cfg.Net is wired by the gateway.
conn, err := cfg.Net.DialContext(ctx, "tcp", cfg.String("host")+":"+port)
```

For L7 backends, `cfg.Net.HTTP()` returns a RoundTripper that routes through the
agent's HTTP proxy when the connection is agent-mode.

## Trust

An agent opens a path *into* the operator's network, so it's a privileged
capability. Enrollment, the tunnel, and the agent binary are all gateway-owned;
your plugin only declares the profile. Operators control which protocols are
available at all from **Settings → Protocols**.
