# Metrics & dashboards

When your protocol has live numbers (CPU, memory, container counts, queue
depth), you can render them as **KPI cards**, **usage rows**, **gauges**, and
**time-series charts**, and group several panels into one **dashboard**. The
gateway draws all of it from a `MetricsConfig`; your job is to stream frames.

## The metrics panel

A `PanelMetrics` panel names the keys it wants and points at a **WebSocket**
route that streams values:

```go
plugin.Panel{
    Key: "stats", Label: "Environment",
    Type:   plugin.PanelMetrics,
    Source: &plugin.DataSource{RouteID: "myplugin.overview.metrics", Method: plugin.MethodWS},
    Config: plugin.MetricsConfig{
        Stats: []plugin.MetricStat{                  // KPI number cards
            {Key: "containers", Label: "Containers"},
            {Key: "running",    Label: "Running"},
        },
        Usage: []plugin.MetricUsage{           // used/capacity rows
            {Key: "cpuPct", Label: "CPU usage", Type: plugin.ColumnPercent,
             Usage: &plugin.UsageSpec{PercentKey: "cpuPct", UsedKey: "cpuUsed", TotalKey: "cpuTotal",
                 UsedType: plugin.ColumnNumber, TotalType: plugin.ColumnNumber, TotalLabel: "of", Unit: "core(s)",
                 WarnAt: 75, CriticalAt: 90}},
            {Key: "memPct", Label: "Memory usage", Type: plugin.ColumnPercent,
             Usage: &plugin.UsageSpec{PercentKey: "memPct", UsedKey: "memUsed", TotalKey: "memTotal",
                 UsedType: plugin.ColumnBytes, TotalType: plugin.ColumnBytes, WarnAt: 80, CriticalAt: 95}},
        },
        Series: []plugin.MetricSeries{               // lines on a time chart
            {Key: "cpuPct", Label: "CPU", Unit: "%"},
            {Key: "memPct", Label: "Memory", Unit: "%"},
        },
        History: 60,                                 // points kept per line
    },
}
```

- **`Stats`** - a scalar shown as a number card. Good for counts.
  `Unit: "bytes"` renders a human-readable byte value, and `Unit: "bytes/s"`
  renders a human-readable byte rate. Other units are shown with a separating
  space, such as `0.003 cores`.
- **`Gauges`** - a radial gauge of the current value against `Max`. Use these
  sparingly for standalone scores. Do not also declare a usage row for the same
  value.
- **`Usage`** - the same usage rows as `PanelObjectDetail`, useful when a live
  metric should read as used/capacity instead of just a chart. Prefer this for
  CPU, memory, disk, quota, pool, and queue capacity. Declare each row with
  `MetricUsage{Usage: &plugin.UsageSpec{...}}`. Do not use it for soft
  baselines such as Kubernetes pod requests when a stat card already presents
  the current and requested values clearly.
- **`Series`** - one line on a shared time chart; new frames append a point and
  `History` bounds how many are kept.
- A key can appear in **both** `Usage` and `Series`; the same frame value feeds a
  current usage row and a trend line. Avoid declaring the same key in `Gauges`
  and `Usage`.

Keep the panel simple: pick the smallest set of widgets that answers the
operator's question. Use stat cards for standalone values, usage rows for real
used/total capacity, gauges only for standalone scores, and series for trend
history. Do not show the same value twice unless the second view adds different
information.

## The frame contract

The WS route streams JSON objects keyed by the metric `Key`. Each object is one
snapshot; the panel reads the keys it declared and ignores the rest. Stream on a
fixed cadence until the client disconnects:

```go
func metrics(rc *plugin.RequestContext, stream plugin.ClientStream) error {
    enc := json.NewEncoder(stream)
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()
    for {
        if err := enc.Encode(frame(rc.Ctx)); err != nil {
            return nil // client went away
        }
        select {
        case <-stream.Context().Done():
            return nil
        case <-rc.Ctx.Done():
            return nil
        case <-ticker.C:
        }
    }
}

// One snapshot. Keys match the MetricStat/Gauge/Series keys above.
func frame(ctx context.Context) map[string]any {
    return map[string]any{
        "containers": 12,
        "running":    9,
        "cpuPct":     round1(cpuPercent),
        "cpuUsed":    round1(cpuUsed),
        "cpuTotal":   round1(cpuTotal),
        "memPct":     round1(memPercent),
        "memUsed":    memUsedBytes,
        "memTotal":   memTotalBytes,
    }
}
```

Encode the **first frame immediately** (before the first tick) so the panel isn't
blank on open. If a backend call fails mid-stream, return the keys you do have
rather than killing the stream - a momentary gap beats a dropped panel.

## Dashboards: several panels in one grid

A `PanelDashboard` lays out multiple panels in one responsive grid via
`DashboardConfig.Cells`. Use it for an overview screen that combines a metrics
panel with a couple of tables:

```go
dash := plugin.DashboardConfig{Cells: []plugin.Panel{
    {Key: "stats", Label: "Environment", Type: plugin.PanelMetrics, Span: 2,
     Source: &plugin.DataSource{RouteID: "myplugin.overview.metrics", Method: plugin.MethodWS},
     Config: overviewMetrics()},
    {Key: "containers", Label: "Containers", Type: plugin.PanelTable, Span: 2,
     Source: &plugin.DataSource{RouteID: "myplugin.containers.list"},
     Config: plugin.TableConfig{Columns: containerColumns()}},
}}
```

`Span` is a sizing hint (how many grid columns a cell takes). A dashboard is just
a panel, so it usually lives as the **Overview** tab of a detail view:

```go
Detail: plugin.DetailView{
    Header: plugin.HeaderSpec{Title: "Overview"},
    Tabs:   []plugin.Panel{{Key: "dashboard", Label: "Overview",
        Type: plugin.PanelDashboard, Config: dash}},
}
```

## The overview pattern

For connection-level overviews, use a **single-row** `ResourceType` whose one row
is "Overview", reached from a tree `Ref`. Clicking it opens the dashboard tab
above. A live overview is: one tree node -> one resource -> a dashboard of a
metrics panel plus tables, fed by one WS route.
