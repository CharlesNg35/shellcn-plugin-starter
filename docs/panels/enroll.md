# PanelEnroll

`PanelEnroll` is the agent enrollment screen. External plugins usually do not
declare it in `Tabs`, `Tree`, resources, or action dialogs.

Instead, an agent-capable plugin declares:

```go
SupportedTransports: []plugin.Transport{plugin.TransportDirect, plugin.TransportAgent},
Agent: &plugin.AgentProfile{
    Proxy: plugin.ProxyTarget{
        Mode:    plugin.AgentTCP,
        Address: "127.0.0.1:5432",
        Risk:    plugin.RiskPrivileged,
    },
    Install: []plugin.InstallArtifact{{
        Label: "Docker",
        Kind:  "docker-run",
        Template: "docker run ... -e SHELLCN_CONNECT_URL={{shellquote .ConnectURL}} " +
            "-e SHELLCN_ENROLL_TOKEN={{shellquote .Token}} {{shellquote .Image}}",
    }},
}
```

For host-sensitive plugins such as Docker, Podman, Swarm, host monitoring, or
other local daemon/socket integrations, prefer agent-only:

```go
SupportedTransports: []plugin.Transport{plugin.TransportAgent},
Agent: &plugin.AgentProfile{
    Proxy: plugin.ProxyTarget{
        Mode:    plugin.AgentUnix,
        Address: "/var/run/docker.sock",
        Risk:    plugin.RiskPrivileged,
    },
    Install: []plugin.InstallArtifact{{Label: "Docker", Kind: "docker-run", Template: "docker run ..."}},
}
```

When a connection uses `TransportAgent` and no tunnel is online, the ShellCN
workspace shows the enrollment UI from the plugin's `AgentProfile.Install`
artifacts. The panel creates an enrollment token, renders install commands or
artifact URLs, polls agent state, and swaps back to the normal workspace when
the agent connects.

See [../agents.md](../agents.md) for the full agent transport contract.
