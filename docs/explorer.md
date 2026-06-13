# Explorer & database plugins

Most non-trivial plugins are **explorers**: a tree of things on the left
(databases, namespaces, topics), a list in the middle, and a detail view on
click. Databases add a query editor and editable grids. This chapter explains
the manifest patterns for that shape. The field reference for each type is in
[manifest.md](manifest.md).

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
keeps first paint bounded for deep hierarchies such as cluster -> node -> guest
or category -> namespace -> object.

```go
// Roots (manifest). Source provides the first level; ResourceKind says what a
// node opens into.
Tree: []plugin.TreeGroup{
    {Key: "nodes", Label: "Nodes", Icon: icon("server"),
     Source: plugin.DataSource{RouteID: "myplugin.tree.nodes"}, ResourceKind: "node"},
    {Key: "storage", Label: "Storage", Icon: icon("database"),
     Source: plugin.DataSource{RouteID: "myplugin.tree.storage"}, ResourceKind: "storage"},
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

## Stop the tree at the collection

The sidebar tree is for **navigation**, not for data. Expand it down to a
**collection** (Pods, Deployments, Volumes) and stop there: make that node a
**leaf** that opens the collection's **list** in the main panel, where it can
paginate, sort, filter, and stream. Never expand a collection's members into the
tree as individual nodes - a namespace with 5,000 pods would build a 5,000-node
tree the user has to scroll, with no paging and no search.

For example, a `Workloads` branch can expand to `Pods`, `Deployments`, and other
collections, and each of those should be a leaf that opens the list view. The
individual items live in the list, never in the tree.

```go
// A collection node: a leaf that opens the kind's LIST (not its members).
plugin.TreeNode{
    Key: "kind:pod", Label: "Pods", Icon: icon("box"),
    Leaf:         true,
    ResourceKind: "pod",                            // click -> open the pod list
    ListParams:   map[string]string{"node": name}, // optional scoping
}
```

Reach for `ChildrenSource` (inline expansion) only for **small, bounded** fan-out
that is itself navigation - categories, sub-groups, a handful of cluster nodes -
not for a data collection. The test: if a node's children could number in the
hundreds or thousands, it's a list, so use `ResourceKind`; if it's a short, fixed
menu, `ChildrenSource` is fine. Clicking a row in the opened list is what reaches
a single item's [detail view](#resource-detail-views) - that is where one pod's
tabs (logs, YAML, events) live, not the tree.

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
         Source: &plugin.DataSource{RouteID: "myplugin.guest.metrics", Method: plugin.MethodWS, Params: guestParams()}, Config: cpuMemMetrics()},
        {Key: "console", Label: "Console", Type: plugin.PanelRemoteDesktop,
         Source: &plugin.DataSource{RouteID: "myplugin.guest.console", Method: plugin.MethodWS, Params: guestParams()},
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
// A "Database" picker (0-15) loaded from a route.
func databaseScope() plugin.ScopeFilter {
    return plugin.ScopeFilter{
        Param:         "database",
        Label:         "Database",
        Icon:          icon("database"),
        OptionsSource: &plugin.DataSource{RouteID: "myplugin.databases.list"},
        ValueField:    "value", // which field of each option row is the value
        LabelField:    "label",
        DefaultValue:  "0",
    }
}

// A namespace picker with an "All namespaces" entry.
plugin.ScopeFilter{
    Param: "namespace", Label: "Namespace",
    OptionsSource: &plugin.DataSource{RouteID: "myplugin.resource.list",
        Params: map[string]string{"kind": "namespace"}},
    ValueField: "name",
    AllLabel:   "All namespaces", // empty value = no filter
}
```

In a handler: `rc.Param("namespace")`. For a multi-select scope, read it with
`rc.ParamList("param", plugin.ScopeSeparator)`. `Control` picks the widget
(`ScopeSelect` default, `ScopeMultiSelect`, `ScopeSearch`, `ScopeToggle`).

Prefer a **narrow `DefaultValue`** so the first paint is bounded, for example
database `0` instead of all databases. Only default to an "all" scope (via
`AllLabel`) when the list behind it is paginated server-side and genuinely cheap
to span; otherwise the opening view fetches everything at once.

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

## Status badges and clear states

A status column should read at a glance. Map the status values to colors **once**
and reuse that map on both the list column and the detail header, so a "running"
guest looks identical in the table and on its detail page:

```go
// Define the value -> color map once; keys are matched lower-cased.
var stateSeverities = map[string]plugin.Severity{
    "running": plugin.SeveritySuccess,
    "paused":  plugin.SeverityWarn,
    "exited":  plugin.SeveritySecondary,
    "dead":    plugin.SeverityDanger,
}

// On the list column...
{Key: "state", Label: "State", Type: plugin.ColumnBadge, Sortable: true, Severities: stateSeverities},
// ...and the detail header, the same map:
Header: plugin.HeaderSpec{Title: "${resource.name}", StatusField: "state", Severities: stateSeverities},
```

The colors are `SeveritySuccess`, `SeverityWarn`, `SeverityDanger`, `SeverityInfo`,
and `SeveritySecondary`; an unmapped value renders neutral. Keep this map in one
helper so list and detail views cannot drift apart.

Give every list and result panel a **meaningful empty state** with `EmptyText`
rather than a blank grid - say what's absent or what to do next:

```go
plugin.TableConfig{Columns: cols, EmptyText: "No containers in this environment."}
plugin.QueryEditorConfig{Language: "sql", EmptyText: "Run a query to see results."}
```

Errors are handled on the handler side: wrap the right
[sentinel](best-practices.md#errors-wrap-a-sentinel-never-return-it-bare) and the
UI surfaces an actionable message (404 vs 403 vs 503) instead of a generic
failure. Success and failure feedback is rendered centrally, so you never build
per-plugin toasts.

## Selectable rows & bulk actions

A `ResourceType` groups its actions by **where they render**:

```go
Actions: plugin.ResourceActions{
    Toolbar: []string{"myplugin.container.create", "myplugin.container.prune"}, // no row context
    Row:     []string{"myplugin.container.remove"},                             // bulk over the selection
    Detail:  []string{"myplugin.container.start", "myplugin.container.stop",    // the one open resource
                      "myplugin.container.restart", "myplugin.container.remove"},
},
```

- **`Toolbar`** - list-level actions with no row context (create, prune).
- **`Row`** - act on the **selected rows** (checkboxes appear automatically;
  declaring `Row` implies `Selectable`). Keep this bar **lean** - typically just
  `delete`/`remove`. It's a bulk bar, not a menu.
- **`Detail`** - the per-item lifecycle/edit actions (start, stop, rename,
  delete) live in the open resource's detail header, **not** the row bar.

The rule: **bulk-destructive only in the row bar; everything single-item goes in
the detail view.** It keeps the selection bar uncluttered and avoids two delete
buttons. For a plain browse table (selection but no row bar), set
`Selectable: true` on the `TableConfig`.

## Guard and group actions

An action is just a button wired to a route, but a few fields make it safe and
usable:

```go
	plugin.Action{
	    ID: "act.qemu.stop", Label: "Stop", Icon: icon("square"),
	    RouteID:     "myplugin.guest.stop",
	    Confirm:     true,                                   // ask before firing
	    ConfirmText: "Force stop this guest? Unsaved state is lost.",
	    EnabledWhen: &plugin.Condition{AllOf: []plugin.Rule{ // grey out unless it applies
	        {Field: "status", Op: plugin.OpIn, Value: []string{"running"}},
	    }},
	    VisibleWhen: &plugin.Condition{AllOf: []plugin.Rule{ // hide when the resource cannot support it
	        {Field: "template", Op: plugin.OpNeq, Value: true},
	    }},
	    Group:     "Power",                                       // cluster into a dropdown
	    OnSuccess: &plugin.ActionSuccess{SelectTab: "snapshots"}, // focus a tab after
	}
```

- **`Confirm` + `ConfirmText`** on anything destructive or disruptive (stop,
  destroy, restore). Write the consequence, not "Are you sure?".
- **`EnabledWhen`** disables the button unless the active row matches - Start only
  when `stopped`, Stop only when `running`. The condition reads row fields
  (`AllOf`/`AnyOf` of `{Field, Op, Value}`; ops `OpEq`, `OpNeq`, `OpIn`, `OpNin`,
  `OpEmpty`, `OpNotEmpty`). This is UX only; the gateway still enforces the route's
  risk server-side.
- **`VisibleWhen`** hides actions that make no sense for the active row - for
  example power controls on VM templates or exec/log actions on resources that do
  not expose a runtime. Prefer hiding only when the action is conceptually
  impossible; use `EnabledWhen` when the action exists but is temporarily blocked
  by state.
- **`Group`** collects related actions (Power, Snapshots) into one labeled
  dropdown instead of a wall of buttons.
- **`OnSuccess.SelectTab`** moves the user to the relevant tab after success (take
  a snapshot, land on the Snapshots tab).

## Editable grids (the database "data" tab)

For a spreadsheet-style editable table, set `Editable` plus the mutation routes.
The grid sends each changed row as JSON to `Insert`/`Update`/`Delete`.

```go
plugin.TableConfig{
    Editable:      true,
    StagedEdits:   true, // batch edits until the user commits/discards
    Exportable:    true,
    ColumnsSource: &plugin.DataSource{RouteID: "myplugin.table.columns", Params: tableParams()},
    Insert:        &plugin.DataSource{RouteID: "myplugin.table.row.insert", Method: plugin.MethodPost,   Params: tableParams()},
    Update:        &plugin.DataSource{RouteID: "myplugin.table.row.update", Method: plugin.MethodPatch,  Params: tableParams()},
    Delete:        &plugin.DataSource{RouteID: "myplugin.table.row.delete", Method: plugin.MethodDelete, Params: tableParams()},
}
```

Two things make editing work:

- **`ColumnsSource`** - for tables whose columns are only known at runtime (a
  real DB table), return the column list from a route instead of hard-coding it.
- **Row keys** - each row your list route returns must carry its primary-key
  values so the grid can address it on update/delete. Your update/delete handlers
  then validate that key before mutating. Never trust a client-supplied row
  identity blindly - re-check it against the real key.

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
    CancelRouteID:     "myplugin.query.cancel",      // cancel a running statement
    CompletionRouteID: "myplugin.query.complete",    // optional autocomplete
    Exportable:        true,
}
```

Run the query over a **WS route** (results stream back as they arrive) and have
the handler report each statement through `rc.Audit(...)` so long-running console
work is recorded. Honor `rc.Ctx` cancellation so the Cancel button actually stops
the statement.

The query editor sends each run as a JSON frame:

```json
{ "query": "SELECT * FROM users LIMIT 100", "confirm": false }
```

Read the `query` field in your stream handler. If your backend expects JSON
instead of a query string, parse that field before forwarding it.

## Build SQL safely

User input reaches your queries two ways, and they are handled differently:

- **Values** (a cell's new content, a filter term) always go through
  **placeholders** (`$1`, `?`) as bound arguments - never concatenated into the
  SQL string.
- **Identifiers** (table, column, schema names) **cannot** be placeholders, so
  validate each against a strict whitelist and quote it.

```go
// Identifier: validate (whitelist) then quote. A common safe shape is
// ^[A-Za-z_][A-Za-z0-9_]{0,62}$; reject anything else.
col, err := safeIdentifier(req.Sort[0].Field)
if err != nil {
    return nil, err
}
order := quoteIdent(col)

// Values: bind, don't concatenate.
rows, err := db.QueryContext(ctx,
    `SELECT * FROM `+quoteIdent(table)+` WHERE status = $1 ORDER BY `+order, status)
```

The same split applies to any query language with identifiers (CQL, N1QL). Treat
**every** name as untrusted - including a column the client got from your own
`ColumnsSource`, since it can send anything back.

## Config / YAML editing

For editing a document (a YAML manifest, a config blob), use a
`PanelCodeEditor` with a `CodeEditorConfig` that loads current content from its
`Source` route and applies edits to a save route:

```go
plugin.CodeEditorConfig{
    Language:    "yaml",
    SaveRouteID: "myplugin.resource.apply", // POST the edited document here
    SaveMethod:  plugin.MethodPost,
}
```

The panel's `Source` loads the current document; Save sends the buffer to
`SaveRouteID`. By default the request body is `{ "content": "..." }`. When the
loaded buffer changes, the generic renderer shows a **Diff** button that opens a
read-only before/after review of the loaded content and the edited content. You
do not need plugin-specific UI to get this workflow.

If your route expects parsed JSON under a named field, set `SaveBodyKey`:

```go
plugin.CodeEditorConfig{
    Language:    "json",
    SaveRouteID: "search.document.upsert",
    SaveMethod:  plugin.MethodPut,
    SaveBodyKey: "document",
}
```

With `SaveBodyKey`, the editor parses the buffer as JSON and sends
`{ "document": ... }`. Validate and apply server-side in the handler, returning a
clear `plugin.ErrInvalidInput` on a bad document so the editor can surface it.

Use `PanelDiff` when the plugin computes both sides itself, for example a
server-side dry-run result or generated DDL preview:

```go
plugin.Panel{
    Key:    "preview",
    Label:  "Preview",
    Type:   plugin.PanelDiff,
    Source: &plugin.DataSource{RouteID: "example.change.preview"},
    Config: plugin.DiffConfig{
        Language:      "yaml",
        OriginalField: "current",
        ModifiedField: "proposed",
        OriginalLabel: "Current",
        ModifiedLabel: "Proposed",
    },
}
```

The route should return an object with the configured fields. Use strings for
text/YAML/SQL, or structured values when JSON pretty-printing is acceptable.

## Putting it together

A database plugin is usually: `LayoutSidebarTree` + a `databases` tree + a
`table` `ResourceType` whose `DetailView` has a **data** tab (editable grid), a
**structure** tab (columns/indexes tables), and a **query** tab (query editor),
with a `Scope` database picker on top.
