# PanelDocument

Use `PanelDocument` for read-only documents or raw structured output: JSON,
YAML, mapping/settings documents, rendered markdown, or backend definitions.

```go
plugin.Panel{
    Key: "definition", Label: "Definition", Icon: icon("braces"),
    Type:   plugin.PanelDocument,
    Source: &plugin.DataSource{RouteID: "demo.definition.read", Params: map[string]string{"id": "${resource.uid}"}},
}
```

## Source route

Return a value that can be JSON-encoded. Common shapes:

```go
return map[string]any{"ddl": ddl}, nil
return rawJSONDocument, nil
return []byte(yamlText), nil
```

If the user should edit the value, use `PanelCodeEditor`. If the value is a
resource overview, use `PanelObjectDetail` with `RawToggle`.

## Good uses

- Database table DDL or view definitions.
- Elasticsearch/OpenSearch mappings and settings.
- Kubernetes raw YAML when no editor is intended.
- Read-only JSON/BSON-like documents.

## Avoid

Do not make `PanelDocument` the primary UI for complex resources. Expose
important properties, lists, metrics, and actions through structured panels.
