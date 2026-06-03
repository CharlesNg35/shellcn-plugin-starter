// Command shellcn-plugin-starter is a template for an out-of-tree ShellCN
// protocol plugin.
//
// A plugin is a normal Go program. It does not import the gateway; it imports
// only the ShellCN plugin SDK, describes itself with a Manifest, exposes a few
// Routes, and hands back a Session when the gateway connects. The gateway runs
// the compiled binary as a subprocess and talks to it over gRPC - so the same
// universal UI, auth, audit, and policy that power the built-in protocols apply
// to your plugin with zero gateway changes.
//
// To make this your own:
//
//  1. Rename the module in go.mod and the plugin Name/Title in manifest.go.
//  2. Replace the in-memory key/value example with your protocol's logic.
//  3. Build it and drop the binary into the gateway's plugin directory
//     (see docs/build-and-install.md).
//
// The example here is deliberately tiny - an in-memory key/value store - so the
// shape of a plugin is easy to see. The docs/ folder explains every part in
// depth, including the features this starter does not use (streaming terminals,
// agent transport, the "open in browser" proxy, recording).
package main

import "github.com/charlesng35/shellcn/sdk"

// main hands the plugin to the SDK, which serves it to the gateway and blocks
// until the gateway disconnects. This is the entire main function - everything
// interesting lives in the manifest and the handlers.
func main() {
	sdk.Serve(Starter{})
}
