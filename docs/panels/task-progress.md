# PanelTaskProgress

Use `PanelTaskProgress` for long-running operations: imports, backups, restores,
cluster upgrades, index rebuilds, migrations, and bulk deletes.

```go
plugin.Panel{
    Key: "task", Label: "Task", Icon: icon("loader"),
    Type:   plugin.PanelTaskProgress,
    Source: &plugin.DataSource{RouteID: "demo.task.watch", Method: plugin.MethodWS, Params: map[string]string{"id": "${resource.uid}"}},
    Config: plugin.TaskProgressConfig{
        Title:         "Import",
        CancelRouteID: "demo.task.cancel",
        RetryRouteID:  "demo.task.retry",
    },
}
```

Declare the watch route in `Streams()`:

```go
plugin.Stream{ID: "demo.task.watch", Kind: plugin.StreamTask, RouteID: "demo.task.watch"}
```

The source route is `MethodWS` and streams task status/progress frames as JSON.
Keep the frame shape stable and test it. Common fields are `status`, `message`,
`progress`, `current`, `total`, `startedAt`, and `finishedAt`.

Cancel and retry routes are normal mutating routes. They should enforce
permissions, risk, audit, and backend state checks.
