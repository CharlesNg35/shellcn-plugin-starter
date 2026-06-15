# File browser plugins

If your protocol exposes files (SFTP, FTP, SMB, NFS, WebDAV, S3, MinIO), you get
a full file-manager UI for free: breadcrumbs, a listing, upload/download, rename,
delete, preview, and bulk operations. You declare a `PanelFileBrowser` with a
`FileBrowserConfig` that wires each operation to one of your routes, and the
gateway renders the manager.

## The panel

```go
plugin.Panel{
    Key: "files", Label: "Files", Icon: icon("folder"),
    Type:   plugin.PanelFileBrowser,
    Source: &plugin.DataSource{RouteID: "myplugin.files.list", Params: map[string]string{"path": "."}},
    Config: plugin.FileBrowserConfig{
        PathParam: "path", // route param carrying the current dir/file path
        Routes: plugin.FileBrowserRoutes{
            Read:     "myplugin.files.read",
            Download: "myplugin.files.download",
            Write:    "myplugin.files.write",
            Mkdir:    "myplugin.files.mkdir",
            Rename:   "myplugin.files.rename",
            Delete:   "myplugin.files.delete",
            Move:     "myplugin.files.move",
            Copy:     "myplugin.files.copy",
            Chmod:    "myplugin.files.chmod",
            Archive:  "myplugin.files.archive",
        },
        Upload: plugin.FileUploadConfig{
            RouteID:   "myplugin.files.upload",
            FieldName: "files", // multipart field name (matches the upload schema)
            Multiple:  true,
        },
        Writable: true,
    },
}
```

Leave a route ID empty and the UI hides that action. A read-only browser is just
`Routes.Read`/`Routes.Download` with `Writable: false`.

File browser operations are request/response routes. Keep progress/cancel needs
in a separate task surface instead of adding file-browser-specific streams.

## Listing a directory

The `Source` route returns a page of entries for the current `path`. Each entry
carries at least a **name**, whether it's a **directory**, a **size**, and a
**modified time**; the browser renders the rest. Resolve `rc.Param("path")`,
read the directory, and sort **directories first, then by name**:

```go
func list(rc *plugin.RequestContext) (any, error) {
    dir := rc.Param("path")
    infos, err := fs.ReadDir(rc.Ctx, dir)
    if err != nil {
        return nil, mapFileError(err) // os.IsNotExist -> ErrNotFound, IsPermission -> ErrForbidden
    }
    // ...build entries, sort dirs-first then name, page with rc.Page()...
    return plugin.Page[entry]{Items: entries, NextCursor: next}, nil
}
```

## Downloads (stream big files, support range)

Return a `*plugin.Download`. Set **exactly one** byte source:

- `Seeker` (an `io.ReadSeekCloser`) - the best choice: the gateway serves HTTP
  range requests, so media players and resumable downloads work.
- `OpenRange(offset, length)` - for backends that can range but not seek.
- `Body` - a plain stream when neither is possible.

```go
return &plugin.Download{
    Name:    path.Base(p),
    MIME:    mimeFor(p),
    Size:    info.Size(),
    ModTime: info.ModTime(),
    Inline:  rc.Param("inline") == "1", // preview in the browser vs. download
    Seeker:  f,                          // *sftp.File, *os.File, etc.
}, nil
```

## Uploads

Declare a `FieldFile` input matching `Upload.FieldName`, then read the parts:

```go
Input: &plugin.Schema{Groups: []plugin.Group{{Fields: []plugin.Field{
    {Key: "files", Label: "Files", Type: plugin.FieldFile, Required: true},
}}}},

func upload(rc *plugin.RequestContext) (any, error) {
    for _, up := range rc.Uploads("files") {
        f, err := up.Open()
        if err != nil { return nil, err }
        // ...stream f to dst, respecting Upload.MaxBytes...
        _ = f.Close()
    }
    return map[string]any{"ok": true}, nil
}
```

## Simple move/copy routes

Simple destination operations receive selected paths and a destination folder:

```json
{ "paths": ["/a.txt", "/logs"], "destination": "/archive" }
```

The file browser still shows a folder picker for the destination. Typing a path
is available as a fallback, not the primary flow.

## Security: never trust the path

Every file route takes a client-supplied path. **Resolve and confine it** to the
connection's root before touching the backend - reject `..` escapes and absolute
paths that climb out. A buggy or hostile client will send `../../etc/passwd`;
your resolve step is the guard. Clean and bound every path first, and map backend
errors to the right sentinel (`ErrNotFound`, `ErrForbidden`).
