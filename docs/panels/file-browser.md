# PanelFileBrowser

The detailed guide lives in [../file-browser.md](../file-browser.md). This page
summarizes the panel contract.

Use `PanelFileBrowser` for filesystems and object stores: SFTP, FTP, SMB, NFS,
WebDAV, S3-compatible storage, and similar protocols.

```go
plugin.Panel{
    Key: "files", Label: "Files", Icon: icon("folder"),
    Type:   plugin.PanelFileBrowser,
    Source: &plugin.DataSource{RouteID: "myplugin.files.list", Params: map[string]string{"path": "."}},
    Config: plugin.FileBrowserConfig{
        PathParam: "path",
        Routes: plugin.FileBrowserRoutes{
            Read:     "myplugin.files.read",
            Download: "myplugin.files.download",
            Mkdir:    "myplugin.files.mkdir",
            Rename:   "myplugin.files.rename",
            Delete:   "myplugin.files.delete",
            Move:     "myplugin.files.move",
            Copy:     "myplugin.files.copy",
        },
        Upload: plugin.FileUploadConfig{RouteID: "myplugin.files.upload", FieldName: "files", Multiple: true},
        Writable: true,
    },
}
```

The list route returns a paged directory listing:

```json
{
  "path": "/var/log",
  "items": [
    {
      "name": "syslog",
      "path": "/var/log/syslog",
      "isDir": false,
      "size": 4096
    }
  ]
}
```

Read routes return preview content:

```json
{
  "path": "/var/log/syslog",
  "encoding": "utf8",
  "content": "...",
  "truncated": false
}
```

Download routes return `*plugin.Download`; upload routes read
`rc.Uploads(config.Upload.FieldName)`.

Operation route calls use these methods and bodies:

| Operation        | Method                | Body                                             |
| ---------------- | --------------------- | ------------------------------------------------ |
| Save file        | `PUT`                 | `{ "content": "..." }`                           |
| Create directory | default action method | `{ "name": "new-dir" }`                          |
| Rename           | `PATCH`               | `{ "name": "new-name" }`                         |
| Delete           | `DELETE`              | `{ "path": "/path/to/item" }`                    |
| Move             | `POST`                | `{ "paths": ["..."], "destination": "/target" }` |
| Copy             | `POST`                | `{ "paths": ["..."], "destination": "/target" }` |
| Chmod            | default action method | `{ "paths": ["..."], "mode": "0644" }`           |
| Archive          | `POST`                | `{ "paths": ["..."] }` and returns a download    |

Each operation also receives the selected path through `PathParam`.

Always normalize and confine paths. Never trust browser-supplied paths.

## Controls

`FileBrowserConfig.Controls` is the same `[]plugin.StreamControl` used by the
[log](log-stream.md) and [terminal](terminal.md) panels. Here the selected value
is merged into **every** file operation's route params (and the listing reloads) —
e.g. a picker that browses a different container's filesystem on a multi-container
pod:

```go
Config: plugin.FileBrowserConfig{
    PathParam: "path",
    Routes:    plugin.FileBrowserRoutes{ /* ... */ },
    Controls: []plugin.StreamControl{{
        Param: "container", Label: "Container",
        OptionsSource: &plugin.DataSource{RouteID: "myplugin.pod.containers"},
    }},
}
```

The selected value reaches each operation handler via `rc.Param("container")`,
alongside the path. The picker is hidden when there is only one option.
