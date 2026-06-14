# Credentials and secrets

ShellCN keeps secrets core-owned. A plugin declares the shape of credentials it
can use, but it never stores ciphertext, returns secret values to the browser, or
logs them.

Use reusable credentials when the same secret can be shared by many
connections. Use inline secret fields only for one-off connection-local secrets.

## Inline secret fields

```go
plugin.Field{
    Key:      "password",
    Label:    "Password",
    Type:     plugin.FieldPassword,
    Secret:   true,
    Required: true,
}
```

The value is encrypted before storage and decrypted into `ConnectConfig.Config`.
Read it with the normal typed helpers:

```go
password := cfg.String("password")
```

Hidden `VisibleWhen` fields are not required or validated. This is useful for
auth mode switches:

```go
{Key: "auth_mode", Label: "Auth", Type: plugin.FieldSelect, Default: "password",
    Options: []plugin.Option{{Label: "Password", Value: "password"}, {Label: "Token", Value: "token"}}},
{Key: "password", Label: "Password", Type: plugin.FieldPassword, Secret: true,
    VisibleWhen: &plugin.Condition{AllOf: []plugin.Rule{{Field: "auth_mode", Op: plugin.OpEq, Value: "password"}}}},
```

## Reusable credential references

Declare a `credential_ref` field when the user should pick a reusable credential:

```go
plugin.Field{
    Key:      plugin.CredentialIDField,
    Label:    "Credential",
    Type:     plugin.FieldCredentialRef,
    Required: true,
    Credential: &plugin.CredentialSelector{
        Kind: plugin.CredentialDBPassword,
    },
}
```

The saved connection stores only the credential id. At connect time the gateway
resolves the credential and attaches its declared fields to
`ConnectConfig.Credentials`, keyed by the credential reference field. Read
values through the helpers:

```go
cred, err := cfg.RequiredCredentialFor(plugin.CredentialIDField, plugin.CredentialDBPassword)
if err != nil {
    return nil, err
}

user, err := cred.RequiredValue("username")
if err != nil {
    return nil, err
}
password, err := cred.RequiredValue("password")
if err != nil {
    return nil, err
}
```

For a non-standard field key, pass that key:

```go
token := cfg.CredentialValueFor("api_credential", "token")
cred, ok := cfg.CredentialFor("api_credential")
```

Each `FieldCredentialRef` selector declares one credential kind. If a protocol
supports alternative stored credentials, such as a stored password and a stored
private key, expose separate fields with separate `VisibleWhen` rules. This keeps
stored config keys, labels, and credential creation predictable.

When a credential contains a field that also exists in inline config, such as
`username`, do not merge the credential into `Config`. Choose the source
explicitly in `Connect` based on the selected auth mode.

## SDK credential kinds

Common SDK kinds include:

- `CredentialSSHPrivateKey`
- `CredentialSSHPassword`
- `CredentialTLSClientCert`
- `CredentialDBPassword`
- `CredentialAPIToken`
- `CredentialCloudAccessKey`
- `CredentialBasicAuth`
- `CredentialBearerToken`

Use the closest existing kind when possible. Operators understand these labels
and ShellCN can reuse them across protocols.

## Custom credential kinds

Declare custom kinds in `Manifest.CredentialKinds`:

```go
CredentialKinds: []plugin.CredentialKindInfo{{
    Kind:  "acme_api_key",
    Label: "ACME API key",
    Fields: []plugin.Field{
        plugin.CredentialPublicField(plugin.Field{Key: "key_id", Label: "Key ID", Type: plugin.FieldText, Required: true}),
        plugin.CredentialSecretField(plugin.Field{Key: "api_key", Label: "API key", Type: plugin.FieldPassword, Required: true}),
    },
}},
```

Then reference that kind from a `CredentialSelector`.

Only create a custom kind when the secret material has protocol-specific meaning
that does not fit the SDK kinds.

Credential kind fields intentionally support a small safe subset of schema
features:

- `FieldText` for non-secret metadata such as username, subject, key id, tenant,
  account id, or profile name.
- `FieldPassword` for short secret values such as passwords, API keys, tokens,
  and access-key secrets.
- `FieldTextarea` for multi-line secret values such as PEM certificates,
  private keys, kubeconfigs, or JSON credentials.

Every credential field must be persisted as either secret material or non-secret
public metadata. Prefer `plugin.CredentialSecretField(...)` for sensitive
values and `plugin.CredentialPublicField(...)` for safe display metadata. Do
not rely on custom frontend code; the gateway renders credential fields from
this declaration.

### Examples

Stored password:

```go
Fields: []plugin.Field{
    plugin.CredentialPublicField(plugin.Field{Key: "username", Label: "Username", Type: plugin.FieldText, Required: true}),
    plugin.CredentialSecretField(plugin.Field{Key: "password", Label: "Password", Type: plugin.FieldPassword, Required: true}),
}
```

Stored token:

```go
Fields: []plugin.Field{
    plugin.CredentialPublicField(plugin.Field{Key: "subject", Label: "Token subject", Type: plugin.FieldText}),
    plugin.CredentialSecretField(plugin.Field{Key: "token", Label: "Token", Type: plugin.FieldPassword, Required: true}),
}
```

Stored client certificate:

```go
Fields: []plugin.Field{
    plugin.CredentialPublicField(plugin.Field{Key: "subject", Label: "Certificate subject", Type: plugin.FieldText}),
    plugin.CredentialSecretField(plugin.Field{Key: "certificate", Label: "Client certificate", Type: plugin.FieldTextarea, Required: true}),
    plugin.CredentialSecretField(plugin.Field{Key: "private_key", Label: "Private key", Type: plugin.FieldTextarea, Required: true}),
    plugin.CredentialSecretField(plugin.Field{Key: "passphrase", Label: "Private key passphrase", Type: plugin.FieldPassword}),
}
```

Connect-time use:

```go
cred, err := cfg.RequiredCredentialFor("client_cert_id", plugin.CredentialTLSClientCert)
if err != nil {
    return nil, err
}
cert, err := cred.RequiredValue("certificate")
if err != nil {
    return nil, err
}
key, err := cred.RequiredValue("private_key")
if err != nil {
    return nil, err
}
bundle := strings.TrimSpace(cert + "\n" + key)
```

## Transport-aware forms

When an agent supplies the target endpoint, direct-only fields such as host,
port, Unix socket, or API server URL should usually be hidden under
`$transport == "direct"`:

```go
directOnly := plugin.Condition{AllOf: []plugin.Rule{
    {Field: plugin.SchemaContextTransport, Op: plugin.OpEq, Value: string(plugin.TransportDirect)},
}}

plugin.Field{Key: "host", Label: "Host", Type: plugin.FieldText, Required: true, VisibleWhen: &directOnly}
```

Credential fields can remain visible for agent mode when the protocol still
needs upstream credentials. For `AgentHTTP` profiles with `TokenFile`/`CAFile`,
the agent may inject target-side credentials, so connection credential fields may
not be needed.

## Rules

- Do not store secrets in `rc.Storage`.
- Do not return secret config values from routes.
- Do not put tokens in route params, URLs, logs, metadata, or audit params.
- Prefer reusable credentials for passwords, API tokens, cloud keys, and client
  certificates.
- Put requiredness on the `FieldCredentialRef` field with `Required: true`, not
  on `CredentialSelector`.
- Keep public fields non-secret: username, key id, account id, or profile name
  is fine.
- Wrap backend auth failures as `plugin.ErrUnauthorized` or
  `plugin.ErrForbidden` with useful context.
