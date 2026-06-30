# PanelTable

Use `PanelTable` for collections: databases, tables, containers, topics,
documents, users, tasks, labels, query results, and any row-oriented result.

```go
plugin.Panel{
    Key:   "items",
    Label: "Items",
    Icon:  plugin.Icon{Type: plugin.IconLucide, Value: "table"},
    Type:  plugin.PanelTable,
    Source: &plugin.DataSource{RouteID: "myplugin.items.list"},
    Config: plugin.TableConfig{
        Columns: []plugin.Column{
            {Key: "name", Label: "Name", Sortable: true},
            {
                Key:   "state",
                Label: "State",
                Type:  plugin.ColumnBadge,
                Severities: map[string]plugin.Severity{
                    "running": plugin.SeveritySuccess,
                    "failed":  plugin.SeverityDanger,
                },
            },
        },
        DefaultSort: &plugin.SortKey{Field: "name"},
        EmptyText:   "No items found.",
        Exportable:  true,
        RowClick:    plugin.RowClickNavigate,
    },
}
```

## Source route

The source route is normally `GET` and returns `plugin.Page[T]`.

```go
func listItems(rc *plugin.RequestContext) (any, error) {
    req, err := rc.Page()
    if err != nil {
        return nil, err
    }

    rows, next, err := loadRows(rc.Ctx, req)
    if err != nil {
        return nil, err
    }
    return plugin.Page[itemRow]{Items: rows, NextCursor: next}, nil
}
```

Always honor `rc.Page()` for limit, cursor, search, and sort. Sorting only the
current page is incorrect. Sort in the backend query, or sort the full in-memory
slice before slicing.

## Row identity

When rows open a resource detail, return a `ref` field:

```go
type itemRow struct {
    Ref   plugin.ResourceIdentity `json:"ref"`
    Name  string             `json:"name"`
    State string             `json:"state"`
}
```

Action params such as `${resource.uid}` resolve from this `ref`, not from an
arbitrary visible column. Keep `ref.Kind` consistent with the manifest resource
type.

Do not add a fake `ref` just to make a row action work. Plain rows can feed row
actions and forms through `${record.*}`:

```go
plugin.Action{
    ID: "myplugin.delete", Label: "Delete", RouteID: "myplugin.delete",
    Params: map[string]string{"key": "${record.key}"},
    Confirm: true,
}
```

Use `${resource.*}` for navigable resource rows; use `${record.*}` for the
selected row's data fields. Nested paths such as `${record.metadata.name}` are
valid.

## Columns

Use explicit `Columns` for stable infrastructure objects. Use `ColumnsSource`
when columns are discovered at runtime, such as database tables or document
collections.

For dynamic table rows, prefer the SDK alias `plugin.TableRow` over local
`map[string]any` aliases:

```go
rows := []plugin.TableRow{
    {"key": "alpha", "status": "ready"},
}
return plugin.Page[plugin.TableRow]{Items: rows}, nil
```

```go
plugin.TableConfig{
    ColumnsSource: &plugin.DataSource{
        RouteID: "myplugin.table.columns",
        Params:  map[string]string{"table": "${resource.uid}"},
    },
}
```

Column fields:

- `Key`: JSON field name in each row.
- `Label`: human-readable column title.
- `Type`: renderer hint such as `ColumnText`, `ColumnBadge`, `ColumnBytes`,
  `ColumnDateTime`, `ColumnRelativeTime`, `ColumnNumber`, `ColumnPercent`,
  `ColumnBool`, `ColumnJSON`, or `ColumnIcon`.
- `Sortable`: tells the renderer it can request sort by that field.
- `Width`: optional CSS width hint.
- `Editable`: opts this column into add-row/update editing. Table-level
  `Editable` does not make every column writable.
- `Editor`: required when `Editable` is true. Use `ColumnEditorText`,
  `ColumnEditorTextarea`, `ColumnEditorNumber`, `ColumnEditorToggle`,
  `ColumnEditorSelect`, or `ColumnEditorJSON`.
- `Options`: required for `ColumnEditorSelect`.
- `ReadOnly`: marks display-only server-managed values.
- `Nullable`: lets editable cells clear to null.
- `Precision`: fixes fraction digits for number/percent cells.
- `Severities`: maps lower-cased badge values to semantic colors.

Use `HiddenColumns` when columns are inferred from dynamic rows but helper fields
such as `ref`, internal ids, or raw metadata should not render.

## Actions

For connection-level tables, use `ActionIDs` for toolbar actions and
`RowActionIDs` for selected-row actions. Declaring `RowActionIDs` implies row
selection.

```go
plugin.TableConfig{
    ActionIDs:    []string{"myplugin.refresh", "myplugin.create"},
    RowActionIDs: []string{"myplugin.rename", "myplugin.delete"},
    Selectable:   true,
}
```

Row actions are single-row by default. This prevents bad UX such as showing
"Rename" when two columns are selected. Add `Bulk: true` to the action only when
it can safely run once for each selected row.

```go
plugin.Action{
    ID:      "myplugin.delete",
    Label:   "Delete",
    RouteID: "myplugin.delete",
    Params:  map[string]string{"table": "${resource.uid}"},
    Body:    map[string]any{"key": "${record._key}"},
    Confirm: true,
    Bulk:    true,
}
```

Use `Params` for route identity and `Body` for structured mutation payloads.
When the body value is exactly one template token, the renderer preserves the
raw value type, so `${record._key}` can become `{ "id": 7 }` instead of a string.

For resource type lists, prefer `ResourceActions.Toolbar`, `ResourceActions.Row`,
and `ResourceActions.Detail` on the resource type. Keep row bars lean. Bulk
delete/remove belongs in row actions. Lifecycle actions often belong in the
resource detail header unless they are intentionally repeatable across selected
rows.

## Row click

`RowClick` controls body clicks:

- `RowClickNavigate`: open the row's `ref` resource detail.
- `RowClickDetail`: open an inline/detail dialog for the row.
- `RowClickSelect`: toggle row selection.
- `RowClickNone`: no row-body behavior.

Use `RowClickNone` for dense editable tables where accidental navigation would
be expensive.

## Editable tables

Use editable mode for database rows and spreadsheet-like data.

```go
plugin.TableConfig{
    Editable:    true,
    StagedEdits: true,
    RowKey:      []string{"id"},
    Columns: []plugin.Column{
        {Key: "id", Label: "ID", ReadOnly: true},
        {Key: "name", Label: "Name", Editable: true, Editor: plugin.ColumnEditorText},
        {Key: "enabled", Label: "Enabled", Type: plugin.ColumnBool, Editable: true, Editor: plugin.ColumnEditorToggle},
        {Key: "metadata", Label: "Metadata", Type: plugin.ColumnJSON, Editable: true, Editor: plugin.ColumnEditorJSON, Nullable: true},
    },
    ColumnsSource: &plugin.DataSource{
        RouteID: "myplugin.columns",
        Params:  map[string]string{"table": "${resource.uid}"},
    },
    Insert: &plugin.DataSource{
        RouteID: "myplugin.row.insert",
        Method:  plugin.MethodPost,
        Params:  map[string]string{"table": "${resource.uid}"},
    },
    Update: &plugin.DataSource{
        RouteID: "myplugin.row.update",
        Method:  plugin.MethodPut,
        Params:  map[string]string{"table": "${resource.uid}"},
    },
    Delete: &plugin.DataSource{
        RouteID: "myplugin.row.delete",
        Method:  plugin.MethodDelete,
        Params:  map[string]string{"table": "${resource.uid}"},
    },
    RowActionIDs: []string{"myplugin.row.delete"},
}
```

`Editable` enables mutation affordances when `Insert`, `Update`, or `Delete`
exist; it is not a blanket "make every cell a text input" switch. Each writable
column must declare `Editable: true` and a concrete `Editor`. Keep primary keys,
generated columns, computed values, and audit fields read-only.

Mutation routes receive edited row data as JSON. Validate table names, primary
keys, column names, and writable columns again in the handler. Do not trust the
column metadata returned by `ColumnsSource` as authorization.

For runtime columns, `ColumnsSource` should return rows with stable `name` or
`key`, `label`, display `type`, plus `editable`, `editor`, `readOnly`, and
`nullable` when the table is writable. JSON/object columns should use
`ColumnEditorJSON`; the renderer shows a compact cell summary and opens a JSON
editor dialog instead of inline-editing `[object Object]`.

`StagedEdits` lets users batch local edits before sending them. Use it for
database tables where accidental single-cell commits are risky.

## Live tables

Use `Watch` for patch streams and `RefreshIntervalMs` for high-churn data where
full refetch is cheaper and simpler than event diffs.

```go
plugin.TableConfig{
    Watch: &plugin.DataSource{
        RouteID: "myplugin.items.watch",
        Method:  plugin.MethodWS,
    },
}
```

`Watch` streams emit `plugin.ResourceEvent` values. Keep the event's resource
ref aligned with row refs so the renderer can update the right row.

## Export

`Exportable` opts into generic CSV/JSON export of loaded rows. It is off by
default because export lets data leave the grid. Enable it only for data users
are allowed to extract.

## Testing checklist

- `rc.Page()` limit, cursor, search, and sort are honored.
- `ref` exists only for rows that navigate to resource detail.
- Plain row actions use `${record.*}` params instead of fake refs.
- Editable mutation handlers revalidate identifiers and writable columns.
- Destructive row actions use `RiskDestructive`.
- Dynamic column routes return stable keys, editor metadata, and read-only flags.
- Export is enabled only when the plugin intentionally permits it.
