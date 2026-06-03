package main

import (
	"context"

	"github.com/charlesng35/shellcn/sdk/plugin"
)

// Starter is the plugin singleton. It is stateless: it only *describes* the
// plugin (Manifest), lists its endpoints (Routes), and opens a Session when the
// gateway connects. All per-connection state lives in the Session, never here -
// one Starter value serves every connection concurrently.
type Starter struct{}

// icon is a small helper for Lucide icons (https://lucide.dev). The UI also
// accepts URLs, emoji, base64 data URIs, and inline SVG - see docs/manifest.md.
func icon(name string) plugin.Icon {
	return plugin.Icon{Type: plugin.IconLucide, Value: name}
}

// Manifest is the plugin's single declarative contract. The gateway reads it
// once at load time and the frontend renders whatever it declares - there is no
// per-plugin UI code. This example declares one table panel (the key/value
// list), a "Set" action that opens a create form, and a per-row "Delete".
//
// See docs/manifest.md for every field, panel type, and layout.
func (Starter) Manifest() plugin.Manifest {
	return plugin.Manifest{
		APIVersion:  plugin.CurrentAPIVersion,
		Name:        "starter", // unique id; lowercase. Rename for your protocol.
		Version:     "0.1.0",
		Title:       "Starter",
		Description: "A template plugin: an in-memory key/value store.",
		Icon:        icon("box"),
		Category:    plugin.CategoryOther,
		Layout:      plugin.LayoutTabs,

		// Transports the connection form offers. "direct" means the gateway
		// reaches the target itself. Add plugin.TransportAgent (and an
		// AgentProfile) to tunnel through an enrolled agent - see docs/agents.md.
		SupportedTransports: []plugin.Transport{plugin.TransportDirect},

		// Connection-form fields. This example needs no configuration, so the
		// schema is empty. A real protocol declares host/port/credentials here;
		// see docs/manifest.md (Config schema) and docs/routes.md (validation).
		Config: plugin.Schema{},

		// Tabs render as the connection workspace. One table, fed by the
		// "starter.list" route, with a toolbar action and a row action.
		Tabs: []plugin.Panel{{
			Key:    "entries",
			Label:  "Entries",
			Icon:   icon("list"),
			Type:   plugin.PanelTable,
			Source: &plugin.DataSource{RouteID: "starter.list"},
			Config: plugin.TableConfig{
				Columns: []plugin.Column{
					{Key: "key", Label: "Key", Sortable: true},
					{Key: "value", Label: "Value"},
				},
				ActionIDs:    []string{"starter.set"},    // toolbar buttons
				RowActionIDs: []string{"starter.delete"}, // per-row buttons
			},
		}},

		// Actions bind a button to a route. An action with an Input schema opens
		// a form; ${resource.uid} pulls the row's id into the route params.
		Actions: []plugin.Action{
			{ID: "starter.set", Label: "Set entry", Icon: icon("plus"), RouteID: "starter.set"},
			{
				ID: "starter.delete", Label: "Delete", Icon: icon("trash-2"), RouteID: "starter.delete",
				Params:      map[string]string{"key": "${resource.uid}"},
				Confirm:     true,
				ConfirmText: "Delete this entry?",
			},
		},
	}
}

// Routes are the plugin's endpoints. Each carries the metadata the gateway
// enforces *before* your handler runs: Permission + Risk feed RBAC, AuditEvent
// names the audit-log entry, and Input is validated against the schema. Your
// handler is pure logic - it never sees HTTP, headers, or auth.
//
// Risk levels: safe (read), write (create/update), destructive (delete),
// privileged (shell/exec/raw socket). See docs/routes.md.
func (Starter) Routes() []plugin.Route {
	return []plugin.Route{
		{
			ID: "starter.list", Method: plugin.MethodGet, Path: "/entries",
			Permission: "starter.read", Risk: plugin.RiskSafe, AuditEvent: "starter.list",
			Handle: list,
		},
		{
			ID: "starter.set", Method: plugin.MethodPost, Path: "/entries",
			Permission: "starter.write", Risk: plugin.RiskWrite, AuditEvent: "starter.set",
			Input: setSchema(), Handle: set,
		},
		{
			ID: "starter.delete", Method: plugin.MethodDelete, Path: "/entries/{key}",
			Permission: "starter.delete", Risk: plugin.RiskDestructive, AuditEvent: "starter.delete",
			Handle: del,
		},
	}
}

// Connect opens a live Session for one connection. The gateway calls it the
// first time a connection is used and reuses the Session until it idles out.
// cfg carries the decrypted connection config and a core-built network
// transport (cfg.Net) for reaching the target - see docs/sessions.md.
func (Starter) Connect(_ context.Context, _ plugin.ConnectConfig) (plugin.Session, error) {
	return newSession(), nil
}

// setSchema describes the "Set entry" form. The same schema validates the
// request on the server before set() runs, so the handler can trust its input.
func setSchema() *plugin.Schema {
	return &plugin.Schema{Groups: []plugin.Group{{
		Name: "Entry",
		Fields: []plugin.Field{
			{Key: "key", Label: "Key", Type: plugin.FieldText, Required: true},
			{Key: "value", Label: "Value", Type: plugin.FieldTextarea},
		},
	}}}
}
