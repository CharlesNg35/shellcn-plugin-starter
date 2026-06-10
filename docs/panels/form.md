# PanelForm

Use `PanelForm` for explicit submit flows inside a panel or dialog. Many actions
do not need it: if an action route has `Input`, ShellCN can render the route
schema as the action form automatically. Use `PanelForm` when the form itself is
a tab or a reusable panel.

```go
plugin.Panel{
    Key: "settings", Label: "Settings", Icon: icon("sliders-horizontal"),
    Type: plugin.PanelForm,
    Config: plugin.FormPanelConfig{
        SubmitRouteID: "demo.settings.save",
        SubmitMethod:  plugin.MethodPut,
        SubmitLabel:   "Save settings",
    },
}
```

## Submit route

The submit route should declare an `Input` schema. The same schema renders the
fields and validates the request before your handler runs.

```go
plugin.Route{
    ID: "demo.settings.save", Method: plugin.MethodPut, Path: "/settings",
    Permission: "demo.settings.write", Risk: plugin.RiskWrite,
    AuditEvent: "demo.settings.save", Input: settingsSchema(),
    Handle: saveSettings,
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

## When not to use it

- For create/update actions, prefer an `Action` with route `Input`.
- For editable text or JSON, use `PanelCodeEditor`.
- For table row editing, use `PanelTable` editable mode.
