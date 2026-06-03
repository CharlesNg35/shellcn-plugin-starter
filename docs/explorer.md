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

## Lazy resource trees

The sidebar is built from `TreeGroup` roots that **expand on demand** - you never
ship the whole tree, you serve each level from a route as the user opens it. This
is how proxmox does cluster -> node -> guest and kubernetes does category ->
namespace -> object without loading everything up front.

```go
// Roots (manifest). Source provides the first level; ResourceKind says what a
// node opens into.
Tree: []plugin.TreeGroup{
    {Key: "nodes", Label: "Nodes", Icon: icon("server"),
     Source: plugin.DataSource{RouteID: "proxmox.tree.nodes"}, ResourceKind: "node"},
    {Key: "storage", Label: "Storage", Icon: icon("database"),
     Source: plugin.DataSource{RouteID: "proxmox.tree.storage"}, ResourceKind: "storage"},
},
```

A tree route returns `plugin.Page[plugin.TreeNode]`. Each `TreeNode` chooses what
happens on click/expand:

```go
plugin.TreeNode{
    Key: "node:" + name, Label: name, Icon: icon("server"),
    ResourceKind: "guest",                              // expanding opens a *list* of this kind...
    ListParams:   map[string]string{"node": name},      //   with these params merged in
    Leaf:         true,
}
// or, to open a single resource's detail:
plugin.TreeNode{Key: "...", Label: name,
    Ref: &plugin.ResourceRef{Kind: "storage", Namespace: node, Name: name, UID: name}, Leaf: true}
// or, to expand deeper lazily:
plugin.TreeNode{Key: "...", Label: name,
    ChildrenSource: &plugin.DataSource{RouteID: "x.tree.children", Params: map[string]string{"parent": name}}}
```

So a node is one of: a **leaf**, a thing that **opens a resource list**
(`ResourceKind` + `ListParams`), a thing that **opens a detail view** (`Ref`), or
a branch that **loads more children** (`ChildrenSource`). Return `Leaf: true` when
there's nothing under it, so the UI doesn't show an expander.

## Resource detail views

A `ResourceType` declares what opens when a row (or `Ref` node) is clicked: a
header with a status badge, and tabbed panels.

```go
Detail: plugin.DetailView{
    Header: plugin.HeaderSpec{
        Title:       "${resource.name}",
        StatusField: "status",          // a row field rendered as a badge
        Severities:  statusSeverities,  // value -> color (running=success, stopped=danger)
    },
    Tabs: []plugin.Panel{
        {Key: "overview", Label: "Overview", Type: plugin.PanelMetrics,
         Source: &plugin.DataSource{RouteID: "proxmox.qemu.metrics", Method: plugin.MethodWS, Params: guestParams()}, Config: cpuMemMetrics()},
        {Key: "console", Label: "Console", Type: plugin.PanelRemoteDesktop,
         Source: &plugin.DataSource{RouteID: "proxmox.qemu.console", Method: plugin.MethodWS, Params: guestParams()},
         Config: plugin.RemoteDesktopConfig{Resize: true, Clipboard: true}},
    },
}
```

Detail tabs are where the per-item screens live: a live metrics graph, a console,
an editable data grid, a YAML editor, logs. `${resource.scope/namespace/name/uid}`
carry the clicked object's identity into each tab's route params.

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

## Config / YAML editing

For editing a document (a Kubernetes object's YAML, a config blob), use a
`PanelCodeEditor` with a `CodeEditorConfig` that loads current content from its
`Source` route and applies edits to a save route:

```go
plugin.CodeEditorConfig{
    Language:    "yaml",
    SaveRouteID: "kubernetes.resource.apply", // POST the edited document here
    SaveMethod:  plugin.MethodPost,
}
```

The panel's `Source` loads the current document; Save sends the buffer to
`SaveRouteID`. Validate and apply server-side in the handler, returning a clear
`plugin.ErrInvalidInput` on a bad document so the editor can surface it.

## Putting it together

A database plugin is usually: `LayoutSidebarTree` + a `databases` tree + a
`table` `ResourceType` whose `DetailView` has a **data** tab (editable grid), a
**structure** tab (columns/indexes tables), and a **query** tab (query editor),
with a `Scope` database picker on top. Read postgresql for the full version; it
uses every pattern here.
