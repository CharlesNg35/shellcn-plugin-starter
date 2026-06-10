# PanelQueryEditor

Use `PanelQueryEditor` for consoles where the user submits a query or command
and receives result frames: SQL, PromQL, LogQL, SurrealQL, search DSLs, database
commands, or similar protocols.

```go
plugin.Panel{
    Key:   "query",
    Label: "SQL",
    Icon:  plugin.Icon{Type: plugin.IconLucide, Value: "square-terminal"},
    Type:  plugin.PanelQueryEditor,
    Source: &plugin.DataSource{
        RouteID: "demo.query",
        Method:  plugin.MethodWS,
    },
    Config: plugin.QueryEditorConfig{
        Language:     "sql",
        InitialQuery: "SELECT 1;",
        ExecuteLabel: "Run",
        RunningLabel: "Running",
        CancelLabel:  "Cancel",
        EmptyText:    "Run a query to see results.",
        Exportable:   true,
    },
}
```

## Stream route

The source route is `MethodWS`. The panel sends execution requests to the stream
as JSON:

```json
{ "query": "SELECT 1;", "confirm": false }
```

When the user accepts a confirmation prompt, the panel resends the same query
with `confirm: true`:

```json
{ "query": "DROP TABLE demo;", "confirm": true }
```

Handlers should tolerate unknown fields, but the current panel only depends on
`query` and `confirm`. Validate both server-side.

```go
type queryRequest struct {
    Query   string `json:"query"`
    Confirm bool   `json:"confirm,omitempty"`
}

func queryStream(rc *plugin.RequestContext, client plugin.ClientStream) error {
    dec := json.NewDecoder(client)
    enc := json.NewEncoder(client)

    for {
        var req queryRequest
        if err := dec.Decode(&req); err != nil {
            if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
                return nil
            }
            return err
        }
        req.Query = strings.TrimSpace(req.Query)
        if req.Query == "" {
            if err := enc.Encode(map[string]any{
                "error": "query is required",
            }); err != nil {
                return err
            }
            continue
        }
        if needsConfirmation(req.Query) && !req.Confirm {
            if err := enc.Encode(map[string]any{
                "confirmRequired": true,
                "confirmMessage":  "This query changes data. Run it anyway?",
            }); err != nil {
                return err
            }
            continue
        }

        params := map[string]string{
            "bytes":     strconv.Itoa(len(req.Query)),
            "confirmed": strconv.FormatBool(req.Confirm),
        }

        result, err := executeQuery(rc.Ctx, req.Query)
        if err != nil {
            rc.Audit(plugin.AuditError, params, err)
            if sendErr := enc.Encode(map[string]any{
                "error": err.Error(),
            }); sendErr != nil {
                return sendErr
            }
            continue
        }
        rc.Audit(plugin.AuditAllowed, params, nil)
        if err := enc.Encode(result); err != nil {
            return err
        }
    }
}
```

Use the plugin session for target clients, but keep per-execution state inside
the stream handler or a request-scoped helper. One WebSocket can execute many
queries, so audit each execution, not just the route open.

## Result frames

For tabular query results, return stable JSON fields:

```json
{
  "columns": ["id", "name"],
  "rows": [[1, "alpha"], [2, "beta"]],
  "rowCount": 2,
  "elapsedMs": 17,
  "statement": "SELECT id, name FROM demo"
}
```

That maps well to SQL, SurrealQL, search DSLs, and admin command output. Keep
`columns` ordered, keep every row the same width, and prefer plain JSON values
over driver-specific types.

For non-tabular protocols, still keep the envelope stable:

```json
{
  "message": "Index compacted",
  "elapsedMs": 124
}
```

Use an error frame instead of closing the socket for normal execution errors:

```json
{ "error": "permission denied" }
```

For risky executions, return a confirmation challenge instead of running the
query:

```json
{ "confirmRequired": true, "confirmMessage": "This query changes data. Run it anyway?" }
```

Close the stream only for transport failure, context cancel, malformed protocol
state, or unrecoverable backend errors.

## Cancel and completion

`CancelRouteID` points at a normal route that cancels the current query. Use it
only when the backend supports cancellation safely.

```go
Config: plugin.QueryEditorConfig{
    Language:      "sql",
    CancelRouteID: "demo.query.cancel",
    CancelParams:  map[string]string{"database": "${resource.uid}"},
}
```

`CompletionRouteID` points at a read route for suggestions. Scope completion to
the active database, namespace, index, or resource with params.

```go
Config: plugin.QueryEditorConfig{
    Language:          "surrealql",
    CompletionRouteID: "demo.query.complete",
    CompletionParams:  map[string]string{"database": "${resource.uid}"},
}
```

Completion routes should be safe, fast, and tolerant of partial input.

## Saved queries

Saved query lists are not built into this panel. Implement normal routes backed
by `rc.Storage` and expose them through a table, action dialog, side panel, or
WASM bridge.

```go
plugin.Route{
    ID: "demo.queries.list", Method: plugin.MethodGet, Path: "/queries",
    Permission: "demo.query.read", Risk: plugin.RiskSafe,
    AuditEvent: "demo.queries.list", Handle: listSavedQueries,
}
plugin.Route{
    ID: "demo.queries.save", Method: plugin.MethodPost, Path: "/queries",
    Permission: "demo.query.write", Risk: plugin.RiskWrite,
    AuditEvent: "demo.queries.save", Handle: saveQuery,
}
```

See [plugin storage](../storage.md).

## Security and audit

- Validate the active resource/database/schema again in the handler.
- Never concatenate untrusted identifiers into backend-specific command text
  without validation or quoting.
- Use route permission and risk for the stream itself, then use `rc.Audit` for
  each executed query.
- Avoid logging full query text when it may contain secrets. Audit query size,
  statement type, target database, row count, duration, and error class instead.
- For dangerous commands, require explicit confirmation in the request and
  enforce it server-side.

## Testing checklist

- Empty query returns an error frame and keeps the stream open.
- Successful query returns ordered `columns`, `rows`, `rowCount`, and timing.
- Backend execution error returns an error frame and keeps the stream open.
- Risky execution returns `confirmRequired` until the request includes
  `confirm: true`.
- Context cancellation exits the stream.
- Each execution emits an audit event.
- Saved-query routes use `rc.Storage` and destructive deletes use
  `RiskDestructive`.
