# Panel theming

ShellCN supports light and dark workspace themes. Most standard panels are fully
core-rendered, so plugins do not need to do anything. The renderer themes tables,
forms, editors, graphs, timelines, object details, metrics, and other generic
panels from the ShellCN design tokens.

Plugins only need to handle theme explicitly when they draw or render their own
surface:

- `PanelCanvas`: the stream receives `theme` in ready/resize events.
- `PanelWasm`: the sandbox receives `window.shellcn.theme`,
  `window.shellcn.colors`, and live `window.shellcn.onTheme` updates.
- `PanelTerminal`: resize/control frames include `theme` for terminal programs
  that can adapt colors.

The value is always `"light"` or `"dark"`.

## Theme is a visual hint

Theme must not affect permissions, route behavior, validation, storage keys, or
target-side state. Treat it like viewport size: useful for presentation, never
part of authorization or protocol correctness.

Good uses:

- choose canvas colors with enough contrast
- set a WASM app `data-theme` attribute
- update chart grid/axis colors
- choose terminal application color palette

Bad uses:

- changing route permissions
- changing which target system is queried
- using theme as a storage namespace
- hiding destructive actions only in one theme

## Canvas theme

Canvas receives the current theme in typed `ReadyEvent` and `ResizeEvent`
payloads:

```go
func (s *scene) Handle(ev canvas.Event) bool {
    switch e := ev.(type) {
    case *canvas.ReadyEvent:
        s.theme = e.Theme
        s.width, s.height = e.Width, e.Height
        return true
    case *canvas.ResizeEvent:
        s.theme = e.Theme
        s.width, s.height = e.Width, e.Height
        return true
    }
    return false
}
```

Use `plugin.PanelThemeLight` and `plugin.PanelThemeDark` constants when branching
in Go.

```go
func (s *scene) palette() palette {
    if s.theme == plugin.PanelThemeLight {
        return palette{
            Background: "#f8fafc",
            Surface:    "#ffffff",
            Text:       "#0f172a",
            Border:     "#cbd5e1",
            Accent:     "#0284c7",
        }
    }
    return palette{
        Background: "#020617",
        Surface:    "#0f172a",
        Text:       "#e2e8f0",
        Border:     "#334155",
        Accent:     "#38bdf8",
    }
}
```

Theme changes cause the renderer to resize/re-emit a resize event while the
stream is open. Redraw on the next dirty frame.

## WASM theme

WASM panels get the current theme and ShellCN color tokens through the bridge:

```js
const initialTheme = window.shellcn.theme; // "light" or "dark"
const initialColors = window.shellcn.colors;
const unsubscribe = window.shellcn.onTheme((theme, colors) => {
  document.body.dataset.theme = theme;
  document.documentElement.style.setProperty("--accent", colors.primary500);
});
```

Apply it before first paint when possible:

```js
document.body.dataset.theme = window.shellcn.theme || "dark";
document.documentElement.style.setProperty(
  "--accent",
  window.shellcn.colors?.primary500 || "#38bdf8",
);
window.shellcn.onTheme((theme, colors) => {
  document.body.dataset.theme = theme;
  document.documentElement.style.setProperty("--accent", colors.primary500);
});
```

Then use CSS variables inside the sandbox:

```css
:root {
  color-scheme: dark;
  --bg: #020617;
  --surface: #0f172a;
  --text: #e2e8f0;
  --muted: #94a3b8;
  --accent: #38bdf8;
}

body[data-theme="light"] {
  color-scheme: light;
  --bg: #f8fafc;
  --surface: #ffffff;
  --text: #0f172a;
  --muted: #64748b;
  --accent: #0284c7;
}
```

Keep the WASM app self-contained. It cannot read ShellCN Tailwind classes or
parent CSS variables directly because it runs inside a sandboxed iframe.
Use `window.shellcn.colors` for ShellCN primary and surface tokens.

## Terminal theme

Terminal panels send in-band control frames for resize events. The SDK helper
`plugin.CopyTerminalInput` consumes resize frames automatically and applies the
size to channels that implement `plugin.Resizer`.

The control payload also includes the theme:

```json
{ "type": "resize", "cols": 120, "rows": 32, "theme": "dark" }
```

If your terminal program needs theme information, read the control frames
yourself or wrap the channel before calling `CopyTerminalInput`. Most shells and
remote PTYs do not need this; xterm itself is already themed by the core.

## Testing checklist

- Canvas scenes update colors after a `ResizeEvent` with a different theme.
- WASM apps apply `window.shellcn.theme` and `window.shellcn.colors` before first
  paint and react to `onTheme(theme, colors)`.
- Theme-dependent colors have contrast in both light and dark modes.
- Theme changes do not change route behavior, storage keys, or authorization.
