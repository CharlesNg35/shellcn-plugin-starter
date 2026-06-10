# PanelFileBrowser

The detailed guide lives in [../file-browser.md](../file-browser.md). This page
summarizes the panel contract.

Use `PanelFileBrowser` for filesystems and object stores: SFTP, FTP, SMB, NFS,
WebDAV, S3-compatible storage, and similar protocols.

```go
plugin.Panel{
    Key: "files", Label: "Files", Icon: icon("folder"),
    Type:   plugin.PanelFileBrowser,
    Source: &plugin.DataSource{RouteID: "demo.files.list", Params: map[string]string{"path": "."}},
    Config: plugin.FileBrowserConfig{
        PathParam:       "path",
        ReadRouteID:     "demo.files.read",
        DownloadRouteID: "demo.files.download",
        UploadRouteID:   "demo.files.upload",
        MkdirRouteID:    "demo.files.mkdir",
        RenameRouteID:   "demo.files.rename",
        DeleteRouteID:   "demo.files.delete",
        Writable:        true,
        MultipleUpload:  true,
        UploadFieldName: "files",
    },
}
```

The list route returns a paged directory listing:

```json
{
  "path": "/var/log",
  "items": [
    { "name": "syslog", "path": "/var/log/syslog", "isDir": false, "size": 4096 }
  ]
}
```

Read routes return preview content:

```json
{ "path": "/var/log/syslog", "encoding": "utf8", "content": "...", "truncated": false }
```

Download routes return `*plugin.Download`; upload routes read
`rc.Uploads(UploadFieldName)`.

Operation route calls use these methods and bodies:

| Operation | Method | Body |
| --- | --- | --- |
| Save file | `PUT` | `{ "content": "..." }` |
| Create directory | default action method | `{ "name": "new-dir" }` |
| Rename | `PATCH` | `{ "name": "new-name" }` |
| Delete | `DELETE` | `{ "path": "/path/to/item" }` |
| Move/copy | default action method | `{ "paths": ["..."], "dest": "/target" }` |
| Chmod | default action method | `{ "paths": ["..."], "mode": "0644" }` |
| Archive | `POST` | `{ "paths": ["..."] }` and returns a download |

Each operation also receives the selected path through `PathParam`.

Always normalize and confine paths. Never trust browser-supplied paths.
