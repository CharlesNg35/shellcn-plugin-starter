# PanelTimeline

Use `PanelTimeline` for ordered events: Kubernetes events, Docker events, audit
history, task history, backups, lifecycle transitions, and job logs summarized as
records.

```go
plugin.Panel{
    Key: "events", Label: "Events", Icon: icon("history"),
    Type:   plugin.PanelTimeline,
    Source: &plugin.DataSource{RouteID: "myplugin.events.list", Params: map[string]string{"id": "${resource.uid}"}},
    Config: plugin.TimelineConfig{
        TimestampField:    "time",
        TitleField:        "reason",
        BodyField:         "message",
        SeverityField:     "severity",
        IconField:         "icon",
        ResourceField:     "resource",
        EmptyText:         "No events yet.",
        RefreshIntervalMs: 5000,
    },
}
```

The source route returns `plugin.Page[T]`. Each item should contain the fields
named in config. Use `RefreshIntervalMs` for polling. Use a watch stream only
when event volume and backend support justify it.
