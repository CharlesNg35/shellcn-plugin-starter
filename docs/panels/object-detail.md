# PanelObjectDetail

Use `PanelObjectDetail` for structured read-only properties: identity, status,
placement, version, capacity, limits, metadata, and copyable ids. Prefer it over
dumping raw JSON into `PanelDocument`.

```go
plugin.Panel{
    Key: "overview", Label: "Overview", Icon: icon("info"),
    Type:   plugin.PanelObjectDetail,
    Source: &plugin.DataSource{RouteID: "myplugin.item.read", Params: map[string]string{"id": "${resource.uid}"}},
    Config: plugin.ObjectDetailConfig{
        Sections: []plugin.ObjectDetailSection{{
            Title: "Identity",
            Fields: []plugin.ObjectDetailField{
                {Key: "name", Label: "Name", Copy: true},
                {Key: "state", Label: "State", Type: plugin.ColumnBadge,
                    Severities: map[string]plugin.Severity{"ready": plugin.SeveritySuccess}},
                {Key: "memoryPct", Label: "Memory usage", Type: plugin.ColumnPercent,
                    Usage: &plugin.UsageSpec{
                        PercentKey: "memoryPct",
                        UsedKey: "memoryUsed", TotalKey: "memoryTotal",
                        UsedType: plugin.ColumnBytes, TotalType: plugin.ColumnBytes,
                        WarnAt: 80, CriticalAt: 95,
                    }},
                {Key: "token", Label: "Token", Redacted: true},
            },
        }},
        RawToggle: true,
    },
}
```

## Source route

Return a JSON object. Keys in `ObjectDetailField.Key` are read from that object.
If `Sections` is empty, the renderer can still show a generic object view, but a
real plugin should declare important fields and labels.

## Field types

`ObjectDetailField.Type` uses the same cell renderers as table columns:
`ColumnText`, `ColumnBadge`, `ColumnBytes`, `ColumnDateTime`,
`ColumnRelativeTime`, `ColumnNumber`, `ColumnPercent`, `ColumnBool`,
`ColumnJSON`, and `ColumnIcon`.

Use `Copy` for ids, names, addresses, URLs, and commands. Use `Redacted` for
values that should be present but not exposed directly.

## Usage fields

Use `ObjectDetailField.Usage` for capacity and consumption values such as CPU,
memory, disk, quota, queue depth, or pool utilization. It renders a compact
summary with a progress bar and optional warning thresholds.

```go
{Key: "cpuPct", Label: "CPU usage", Type: plugin.ColumnPercent,
    Usage: &plugin.UsageSpec{
        PercentKey: "cpuPct",
        TotalKey: "cpuTotal", TotalType: plugin.ColumnNumber,
        TotalLabel: "of", Unit: "CPU(s)",
        WarnAt: 75, CriticalAt: 90,
    }},
{Key: "memPct", Label: "Memory usage", Type: plugin.ColumnPercent,
    Usage: &plugin.UsageSpec{
        PercentKey: "memPct",
        UsedKey: "memUsed", TotalKey: "memTotal",
        UsedType: plugin.ColumnBytes, TotalType: plugin.ColumnBytes,
        WarnAt: 80, CriticalAt: 95,
    }},
```

Prefer a usage field when the operator needs to compare used vs available. Keep
plain fields for identity, placement, version, status, and configuration.

## Raw toggle

`RawToggle` is useful for diagnostics, but it should not be the only useful view.
Show the fields operators need first, then allow raw JSON as a fallback.
