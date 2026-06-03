# File browser plugins

If your protocol exposes files (SFTP, FTP, SMB, NFS, WebDAV, S3, MinIO), you get
a full file-manager UI for free: breadcrumbs, a listing, upload/download, rename,
delete, preview, and bulk operations. You declare a `PanelFileBrowser` with a
`FileBrowserConfig` that wires each operation to one of your routes, and the
gateway renders the manager. This is how all ~10 file-transfer built-ins work,
through the shared `filesystem` helper.

## The panel

```go
plugin.Panel{
    Key: "files", Label: "Files", Icon: icon("folder"),
    Type:   plugin.PanelFileBrowser,
    Source: &plugin.DataSource{RouteID: "fs.files.list", Params: map[string]string{"path": "."}},
    Config: plugin.FileBrowserConfig{
        PathParam:       "path",                // the route param carrying the current dir
        ReadRouteID:     "fs.files.read",        // preview a file's contents
        DownloadRouteID: "fs.files.download",
        WriteRouteID:    "fs.files.write",       // save an edited text file
        UploadRouteID:   "fs.files.upload",
        MkdirRouteID:    "fs.files.mkdir",
        RenameRouteID:   "fs.files.rename",
        DeleteRouteID:   "fs.files.delete",
        // Optional bulk ops over a multi-selection:
        MoveRouteID:     "fs.files.move",
        CopyRouteID:     "fs.files.copy",
        ChmodRouteID:    "fs.files.chmod",
        ArchiveRouteID:  "fs.files.archive",
        Writable:        true,
        MultipleUpload:  true,
        MaxUploadBytes:  50 << 20,
        UploadFieldName: "files",               // multipart field name (matches the upload schema)
    },
}
```

Leave a `*RouteID` empty and the UI hides that action - a read-only browser is
just `ReadRouteID`/`DownloadRouteID` with `Writable: false`.

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

Declare a `FieldFile` input matching `UploadFieldName`, then read the parts:

```go
Input: &plugin.Schema{Groups: []plugin.Group{{Fields: []plugin.Field{
    {Key: "files", Label: "Files", Type: plugin.FieldFile, Required: true},
}}}},

func upload(rc *plugin.RequestContext) (any, error) {
    for _, up := range rc.Uploads("files") {
        f, err := up.Open()
        if err != nil { return nil, err }
        // ...stream f to dst, respecting MaxUploadBytes...
        _ = f.Close()
    }
    return map[string]any{"ok": true}, nil
}
```

## Security: never trust the path

Every file route takes a client-supplied path. **Resolve and confine it** to the
connection's root before touching the backend - reject `..` escapes and absolute
paths that climb out. A buggy or hostile client will send `../../etc/passwd`;
your resolve step is the guard. The shared `filesystem` helper does this for the
built-ins; if you write your own, clean and bound every path first, and map
backend errors to the right sentinel (`ErrNotFound`, `ErrForbidden`).

## Reuse

The 10 built-in file plugins share one implementation (`plugins/shared/filesystem`
+ protocol adapters like `ftpfs`, `s3compat`, `sshsftp`). You can't import those
(they're gateway-internal), but they're the reference for path handling, bulk
ops, and range downloads - read them, copy the patterns into your plugin.
