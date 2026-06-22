# PanelForm

Use `PanelForm` for explicit submit flows inside a panel or dialog. Many actions
do not need it: if an action route has `Input`, ShellCN can render the route
schema as the action form automatically. Use `PanelForm` when the form itself is
a tab or a reusable panel.

```go
plugin.Panel{
    Key: "settings", Label: "Settings", Icon: icon("sliders-horizontal"),
    Type: plugin.PanelForm,
    Source: &plugin.DataSource{RouteID: "myplugin.settings.schema"},
    Config: plugin.FormPanelConfig{
        SubmitRouteID: "myplugin.settings.save",
        SubmitMethod:  plugin.MethodPut,
        SubmitLabel:   "Save settings",
    },
}
```

## Submit route

The panel `Source` route returns a `plugin.Schema`; that schema renders the
fields. The submit route should declare the same schema as `Input`, so the
gateway validates the request before your handler runs.

```go
plugin.Route{
    ID: "myplugin.settings.schema", Method: plugin.MethodGet, Path: "/settings/schema",
    Permission: "myplugin.settings.read", Risk: plugin.RiskSafe,
    AuditEvent: "myplugin.settings.schema", Handle: settingsSchemaRoute,
}
```

```go
plugin.Route{
    ID: "myplugin.settings.save", Method: plugin.MethodPut, Path: "/settings",
    Permission: "myplugin.settings.write", Risk: plugin.RiskWrite,
    AuditEvent: "myplugin.settings.save", Input: settingsSchema(),
    Handle: saveSettings,
}
```

```go
func settingsSchemaRoute(*plugin.RequestContext) (any, error) {
    return settingsSchema(), nil
}
```

```go
func saveSettings(rc *plugin.RequestContext) (any, error) {
    var in settingsInput
    if err := rc.Bind(&in); err != nil {
        return nil, err
    }
    return map[string]any{"ok": true}, nil
}
```

## Params

Use `FormPanelConfig.Params` for resource-scoped forms:

```go
Params: map[string]string{"name": "${resource.name}"}
```

For forms opened from a table row action, field defaults, submit params, and
`OptionsSource.Params` can also use `${record.*}` from the selected row:

```go
plugin.Field{Key: "ttl", Label: "TTL", Type: plugin.FieldNumber, Default: "${record.ttl}"}
```

Use `${resource.*}` for the active `ResourceIdentity`; use `${record.*}` for the
current row or object data.

## Save feedback

`FormPanelConfig.SaveToast` and `SaveDismiss` behave exactly as in
[code-editor](code-editor.md): set `SaveDismiss: plugin.SaveDismissClose` to close
a dialog-opened form on success, and `SaveToast` (a `*plugin.SaveToast` with
`Summary`/`Detail`/`Severity`, where `Detail` may use `${response.x}`) to confirm
via a toast once the panel is gone.

```go
Config: plugin.FormPanelConfig{
    SubmitRouteID: "myplugin.settings.save",
    SubmitMethod:  plugin.MethodPut,
    SaveDismiss:   plugin.SaveDismissClose,
    SaveToast:     &plugin.SaveToast{Summary: "Saved", Severity: plugin.SeveritySuccess},
}
```

## When not to use it

- For create/update actions, prefer an `Action` with route `Input`.
- For editable text or JSON, use `PanelCodeEditor`.
- For table row editing, use `PanelTable` editable mode.
