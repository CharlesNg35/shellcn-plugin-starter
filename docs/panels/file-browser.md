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

The list route returns a paged directory listing. Download routes return
`*plugin.Download`; upload routes read `rc.Uploads(UploadFieldName)`.

Always normalize and confine paths. Never trust browser-supplied paths.
