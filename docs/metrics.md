# Metrics & dashboards

When your protocol has live numbers (CPU, memory, container counts, queue
depth), you can render them as **KPI cards**, **gauges**, and **time-series
charts**, and group several panels into one **dashboard**. The gateway draws all
of it from a `MetricsConfig`; your job is to stream frames.

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
        Gauges: []plugin.MetricGauge{                // radial gauges (value vs Max)
            {Key: "cpu", Label: "CPU", Unit: "%", Max: 100},
            {Key: "mem", Label: "Memory", Unit: "%", Max: 100},
        },
        Series: []plugin.MetricSeries{               // lines on a time chart
            {Key: "cpu", Label: "CPU", Unit: "%"},
            {Key: "mem", Label: "Memory", Unit: "%"},
        },
        History: 60,                                 // points kept per line
    },
}
```

- **`Stats`** - a scalar shown as a number card. Good for counts.
- **`Gauges`** - a radial gauge of the current value against `Max`. `Max: 0`
  means a percentage (0-100). Set `Unit` for the label (`%`, `MB`).
- **`Series`** - one line on a shared time chart; new frames append a point and
  `History` bounds how many are kept.
- A key can appear in **both** `Gauges` and `Series`; the same frame value feeds
  both.

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
        "cpu":        round1(cpuPercent),
        "mem":        round1(memPercent),
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
