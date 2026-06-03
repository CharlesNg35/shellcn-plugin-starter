package main

import (
	"context"
	"sort"
	"sync"

	"github.com/charlesng35/shellcn/sdk/plugin"
)

// entry is one row returned to the table panel. JSON field names must match the
// column Keys declared in the manifest ("key", "value").
type entry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// session is the per-connection runtime. The gateway creates one via
// Starter.Connect and hands it to every handler through rc.Session, so this is
// where you keep live clients, sockets, caches — anything tied to one
// connection. Here it is just a guarded map standing in for a real backend.
type session struct {
	mu      sync.Mutex
	entries map[string]string
}

func newSession() *session {
	return &session{entries: map[string]string{}}
}

// HealthCheck lets the gateway probe the connection (e.g. before reusing an idle
// session). Return an error to signal the upstream is gone. A real plugin would
// ping its backend here.
func (s *session) HealthCheck(context.Context) error { return nil }

// OpenChannel opens a tracked byte-stream to the upstream (used by terminals,
// exec, port-forwards). This template has no such streams, so it declines.
// See docs/streaming.md for the full story.
func (s *session) OpenChannel(context.Context, plugin.ChannelRequest) (plugin.Channel, error) {
	return nil, plugin.ErrNotSupported
}

// Close releases the connection's resources when the session ends. Nothing to
// release here.
func (s *session) Close() error { return nil }

// --- route handlers ---------------------------------------------------------
//
// A handler receives the request context and returns (value, error). The value
// is JSON-encoded for the client; the error maps to an HTTP status (see the
// plugin.Err* sentinels). Handlers never touch HTTP directly.

// list returns every entry. A table panel expects a plugin.Page; for large data
// sets read rc.Page() for cursor/limit/sort/filter — see docs/routes.md.
func list(rc *plugin.RequestContext) (any, error) {
	s := rc.Session.(*session)
	s.mu.Lock()
	defer s.mu.Unlock()

	items := make([]entry, 0, len(s.entries))
	for k, v := range s.entries {
		items = append(items, entry{Key: k, Value: v})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Key < items[j].Key })
	return plugin.Page[entry]{Items: items}, nil
}

// set creates or updates one entry. rc.Bind decodes the body into the struct and
// runs `validate` tags, so by the time it returns the input is trusted.
func set(rc *plugin.RequestContext) (any, error) {
	var in struct {
		Key   string `json:"key" validate:"required"`
		Value string `json:"value"`
	}
	if err := rc.Bind(&in); err != nil {
		return nil, err
	}

	s := rc.Session.(*session)
	s.mu.Lock()
	s.entries[in.Key] = in.Value
	s.mu.Unlock()
	return entry{Key: in.Key, Value: in.Value}, nil
}

// del removes one entry. The {key} path placeholder from the route arrives via
// rc.Param, populated from the row action's ${resource.uid}.
func del(rc *plugin.RequestContext) (any, error) {
	s := rc.Session.(*session)
	s.mu.Lock()
	delete(s.entries, rc.Param("key"))
	s.mu.Unlock()
	return map[string]any{"ok": true}, nil
}
