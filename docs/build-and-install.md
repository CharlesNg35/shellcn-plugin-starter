# Build, install & version

A plugin is a normal Go executable. It's **OS- and arch-specific** (like the
gateway binary itself) - there's no universal artifact. Build the target that
matches your gateway, drop it in the plugin directory, restart.

## Depending on the SDK

The plugin SDK is a published Go module, pulled from the proxy like any other
dependency - you do **not** need the gateway's source:

```sh
go get github.com/charlesng35/shellcn/sdk@latest
go build -o starter .
```

`go.mod` already pins a version (`github.com/charlesng35/shellcn/sdk v0.1.0`);
bump it with `go get ...@<newer>` when a new SDK is released.

## Prefer pure Go

Keep the plugin **CGO-free** (`CGO_ENABLED=0`). Pure-Go plugins cross-compile to
every platform with one toolchain and have no runtime library dependencies. The
gateway's own stack is pure Go for the same reason.

## Build one target

```sh
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o starter .
```

## Cross-compile the matrix

A release should cover every platform a gateway might run on:

```sh
for t in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do
  goos=${t%/*}; goarch=${t#*/}; ext=""; [ "$goos" = windows ] && ext=.exe
  CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch \
    go build -trimpath -ldflags "-s -w" -o "dist/starter-$goos-$goarch$ext" .
done
```

`.github/workflows/release.yml` does exactly this on a `v*` tag and attaches the
binaries (with `checksums.txt`) to a GitHub Release. A wrong-arch binary fails
the gateway handshake cleanly, so shipping the whole matrix is safe.

## Install into a gateway

1. Find the gateway's plugin directory: config key `plugins.dir` (env
   `SHELLCN_PLUGINS_DIR`), default `plugins.d/` relative to the server's working
   directory.
2. Copy the matching binary there and make it executable:
   ```sh
   cp dist/starter-linux-amd64 /path/to/plugins.d/starter
   chmod +x /path/to/plugins.d/starter
   ```
3. Restart the gateway. It scans the directory at startup, spawns each binary,
   and registers it.
4. As an admin, open **Settings → Protocols**. Your plugin appears in the
   **External** tab with its version and health. Set its availability
   (enabled / admins-only / disabled) there.

The protocol now shows in the connection catalog like any built-in.

## Updating & removing

- **Update:** replace the binary and restart. If the gateway is configured to
  verify checksums, ship the matching `.sha256` too (operator-side option).
- **Disable:** set the protocol to *disabled* in Settings → Protocols - it's
  hidden and can't open sessions, but stays loaded (re-enable without a restart).
- **Remove:** delete the binary from the plugin directory and restart.

## Versioning & compatibility

- `Manifest.APIVersion` must equal `plugin.CurrentAPIVersion`; the gateway
  refuses a mismatch with a clear error.
- The handshake also pins a wire `ProtocolVersion`; an incompatible gateway/SDK
  pair is rejected at load, never half-works.
- Bump your own `Manifest.Version` per release so operators see what they're
  running.

## Trust model

An installed plugin is operator-chosen code that receives the decrypted
credentials for the connections it serves - treat it like a Terraform provider
or an IDE extension. Out-of-process execution gives isolation (separate address
space, killable, can't read other plugins' memory) but not unconditional safety:
install plugins you trust, from sources you trust. The gateway keeps the guard
rails - authn/authz, audit, and network egress all stay core-owned.
