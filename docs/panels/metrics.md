# PanelMetrics

The detailed guide lives in [../metrics.md](../metrics.md). This page summarizes
the panel contract.

Use `PanelMetrics` for live numeric status: CPU, memory, throughput, latency,
queue depth, object counts, capacity, and health.

```go
plugin.Panel{
    Key: "metrics", Label: "Metrics", Icon: icon("activity"),
    Type:   plugin.PanelMetrics,
    Source: &plugin.DataSource{RouteID: "myplugin.metrics", Method: plugin.MethodWS},
    Config: plugin.MetricsConfig{
        Stats:  []plugin.MetricStat{{Key: "requests", Label: "Requests"}},
        Gauges: []plugin.MetricGauge{{Key: "cpu", Label: "CPU", Unit: "%", Max: 100}},
        Series: []plugin.MetricSeries{{Key: "cpu", Label: "CPU", Unit: "%"}},
        History: 60,
    },
}
```

Declare the stream route in `Streams()` as well:

```go
plugin.Stream{ID: "myplugin.metrics", Kind: plugin.StreamMetrics, RouteID: "myplugin.metrics"}
```

The stream writes JSON snapshots:

```json
{ "requests": 42, "cpu": 17.4 }
```

Write the first frame immediately, then tick. Prefer partial frames over killing
the stream for one missing backend metric.
