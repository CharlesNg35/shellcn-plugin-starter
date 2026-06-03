# Explorer & database plugins

Most non-trivial plugins are **explorers**: a tree of things on the left
(databases, namespaces, topics), a list in the middle, and a detail view on
click. Databases add a query editor and editable grids. This chapter collects
the patterns the built-ins use for that shape (postgresql, mongodb, docker,
kubernetes, redis). The field reference for each type is in
[manifest.md](manifest.md); this is how to put them together well.

The frame is always:

```go
Layout:    plugin.LayoutSidebarTree,
Tree:      []plugin.TreeGroup{ /* lazy roots: databases, namespaces, ... */ },
Resources: []plugin.ResourceType{ /* a managed object type: list + detail */ },
Scope:     []plugin.ScopeFilter{ /* global selectors injected into reads */ },
```

## Scope filters (the global picker)

A `ScopeFilter` is a selector shown above everything (a database, namespace, or
region picker). Its chosen value is injected into **every read and stream
route's params**, so you don't thread it through each panel by hand. Populate the
choices from a route, and read the value in handlers with `rc.Param(...)`.

```go
// redis: a "Database" picker (0-15) loaded from a route.
func databaseScope() plugin.ScopeFilter {
    return plugin.ScopeFilter{
        Param:         "database",
        Label:         "Database",
        Icon:          icon("database"),
        OptionsSource: &plugin.DataSource{RouteID: "redis.databases.list"},
        ValueField:    "value", // which field of each option row is the value
        LabelField:    "label",
        DefaultValue:  "0",
    }
}

// kubernetes: a namespace picker with an "All namespaces" entry.
plugin.ScopeFilter{
    Param: "namespace", Label: "Namespace",
    OptionsSource: &plugin.DataSource{RouteID: "kubernetes.resource.list",
        Params: map[string]string{"kind": "namespace"}},
    ValueField: "name",
    AllLabel:   "All namespaces", // empty value = no filter
}
```

In a handler: `rc.Param("namespace")`. For a multi-select scope, read it with
`rc.ParamList("param", plugin.ScopeSeparator)`. `Control` picks the widget
(`ScopeSelect` default, `ScopeMultiSelect`, `ScopeSearch`, `ScopeToggle`).

## Sortable columns - and sort on the server

Mark a column `Sortable: true` and set the table's `DefaultSort`. The UI sends
the chosen sort to your list route; **apply it server-side** via `rc.Page()`, so
sorting is correct across pages (never sort just the current page).

```go
Columns: []plugin.Column{
    {Key: "name",  Label: "Name",  Sortable: true},
    {Key: "size",  Label: "Size",  Type: plugin.ColumnBytes, Sortable: true},
    {Key: "state", Label: "State", Type: plugin.ColumnBadge, Sortable: true,
     Severities: map[string]plugin.Severity{"running": plugin.SeveritySuccess}},
},
DefaultSort: &plugin.SortKey{Field: "name"},
```

```go
// In the list handler - SQL example (validate the field; never interpolate raw):
req, _ := rc.Page()
if len(req.Sort) > 0 {
    col, err := sqldb.SafeIdentifier(req.Sort[0].Field) // whitelist/quote
    if err != nil {
        return nil, err
    }
    order := col
    if req.Sort[0].Desc {
        order += " DESC"
    }
    // ... ORDER BY <order> ...
}
// For in-memory rows, sort the slice by req.Sort[0].Field before paging.
```

`Column.Type` also drives the cell renderer: `ColumnBytes`, `ColumnDateTime`,
`ColumnNumber`, `ColumnPercent`, `ColumnBadge` (+`Severities`), `ColumnJSON`.

## Selectable rows & bulk actions

A `ResourceType` groups its actions by **where they render**:

```go
Actions: plugin.ResourceActions{
    Toolbar: []string{"docker.container.create", "docker.container.prune"}, // no row context
    Row:     []string{"docker.container.remove"},                           // bulk over the selection
    Detail:  []string{"docker.container.start", "docker.container.stop",    // the one open resource
                      "docker.container.restart", "docker.container.remove"},
},
```

- **`Toolbar`** - list-level actions with no row context (create, prune).
- **`Row`** - act on the **selected rows** (checkboxes appear automatically;
  declaring `Row` implies `Selectable`). Keep this bar **lean** - typically just
  `delete`/`remove`. It's a bulk bar, not a menu.
- **`Detail`** - the per-item lifecycle/edit actions (start, stop, rename,
  delete) live in the open resource's detail header, **not** the row bar.

The rule the built-ins follow: **bulk-destructive only in the row bar; everything
single-item goes in the detail view.** It keeps the selection bar uncluttered and
avoids two delete buttons. For a plain browse table (selection but no row bar),
set `Selectable: true` on the `TableConfig`.

## Editable grids (the database "data" tab)

For a spreadsheet-style editable table, set `Editable` plus the mutation routes.
The grid sends each changed row as JSON to `Insert`/`Update`/`Delete`.

```go
plugin.TableConfig{
    Editable:      true,
    StagedEdits:   true, // batch edits until the user commits/discards
    Exportable:    true,
    ColumnsSource: &plugin.DataSource{RouteID: "pg.table.columns", Params: tableParams()},
    Insert:        &plugin.DataSource{RouteID: "pg.table.row.insert", Method: plugin.MethodPost,   Params: tableParams()},
    Update:        &plugin.DataSource{RouteID: "pg.table.row.update", Method: plugin.MethodPatch,  Params: tableParams()},
    Delete:        &plugin.DataSource{RouteID: "pg.table.row.delete", Method: plugin.MethodDelete, Params: tableParams()},
}
```

Two things make editing work:

- **`ColumnsSource`** - for tables whose columns are only known at runtime (a
  real DB table), return the column list from a route instead of hard-coding it.
- **Row keys** - each row your list route returns must carry its primary-key
  values so the grid can address it on update/delete. The postgres plugin tags
  every row with its PK map (`attachRowKeys`); your update/delete handlers then
  validate that key before mutating. Never trust a client-supplied row identity
  blindly - re-check it against the real key.

`${resource.scope}` / `${resource.namespace}` / `${resource.name}` in the params
carry the selected database/schema/table down to every route.

## Query editor

Pair a `PanelQueryEditor` with a `QueryEditorConfig` for a SQL/console panel:

```go
plugin.QueryEditorConfig{
    Language:          "sql",
    ExecuteLabel:      "Run query",
    CancelLabel:       "Cancel query",
    EmptyText:         "Run a query to see results.",
    CancelRouteID:     "pg.query.cancel",      // cancel a running statement
    CompletionRouteID: "pg.query.complete",    // optional autocomplete
    Exportable:        true,
}
```

Run the query over a **WS route** (results stream back as they arrive) and have
the handler report each statement through `rc.Audit(...)` so long-running console
work is recorded. Honor `rc.Ctx` cancellation so the Cancel button actually stops
the statement.

## Putting it together

A database plugin is usually: `LayoutSidebarTree` + a `databases` tree + a
`table` `ResourceType` whose `DetailView` has a **data** tab (editable grid), a
**structure** tab (columns/indexes tables), and a **query** tab (query editor),
with a `Scope` database picker on top. Read postgresql for the full version; it
uses every pattern here.
