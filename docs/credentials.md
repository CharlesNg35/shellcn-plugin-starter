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
    Key:   plugin.CredentialField,
    Label: "Credential",
    Type:  plugin.FieldCredentialRef,
    Credential: &plugin.CredentialSelector{
        Kinds:    []plugin.CredentialKind{plugin.CredentialDBPassword},
        Required: true,
    },
}
```

The saved connection stores only the credential id. At connect time the gateway
resolves the credential and injects derived config keys. Read them through the
helpers:

```go
user := cfg.String("username")
if id := cfg.CredentialIdentityFor(plugin.CredentialField); id != "" {
    user = id
}
password := cfg.CredentialSecretFor(plugin.CredentialField)
kind := cfg.CredentialKindFor(plugin.CredentialField)
```

For a non-standard field key, pass that key:

```go
secret := cfg.CredentialSecretFor("api_credential")
```

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
    Kind:          "acme_api_key",
    Label:         "ACME API key",
    SecretLabel:   "API key",
    IdentityLabel: "Key ID",
}},
```

Then reference that kind from a `CredentialSelector`.

Only create a custom kind when the secret material has protocol-specific meaning
that does not fit the SDK kinds.

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
- Keep credential identity non-secret: username, key id, account id, or profile
  name is fine.
- Wrap backend auth failures as `plugin.ErrUnauthorized` or
  `plugin.ErrForbidden` with useful context.
