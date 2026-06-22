# go-box-sdk

A focused Go port of the official [Box Platform SDK](https://github.com/box/box-node-sdk),
covering the pieces needed to use Box as a file/evidence repository:
authentication, the networking/serialization core, and the
**files / folders / uploads / chunked-uploads / downloads** managers.

It is transformed from the auto-generated Box TypeScript SDK (`box-node-sdk`),
cross-checked against the Python SDK, and preserves the same wire behavior
(request shapes, retry rules, chunked-upload SHA1 verification, error mapping).

> Scope: this is not a 1:1 port of the entire Box API. Only the managers and
> schemas required for repository-style usage are included.

## Requirements

- Go 1.20+

## Installation

```bash
go get github.com/asalih/go-box-sdk
```

## Authentication

All four Box authentication methods are supported. Each implements the
`networking.Authentication` interface and can be passed to `box.NewBoxClient`.

`box.NewBoxClient` takes an `Authentication` and an optional `*networking.NetworkSession`
(pass `nil` for SDK defaults).

### Client Credentials Grant (CCG)

```go
auth := box.NewBoxCcgAuth(box.CcgConfig{
    ClientID:     "<client-id>",
    ClientSecret: "<client-secret>",
    EnterpriseID: "<enterprise-id>",
})
client := box.NewBoxClient(auth, nil)
```

### JWT (server auth)

```go
cfg, err := box.JwtConfigFromConfigFile("config.json", nil, nil)
if err != nil {
    log.Fatal(err)
}
auth := box.NewBoxJwtAuth(*cfg)
client := box.NewBoxClient(auth, nil)
```

### OAuth 2.0

```go
oauth := box.NewBoxOAuth(box.OAuthConfig{
    ClientID:     "<client-id>",
    ClientSecret: "<client-secret>",
})
url := oauth.GetAuthorizeURL(box.GetAuthorizeURLOptions{RedirectURI: "https://example.com/callback"})
// ... redirect the user, then exchange the authorization code:
_, err := oauth.GetTokensAuthorizationCodeGrant(ctx, "<auth-code>", nil)
client := box.NewBoxClient(oauth, nil)
```

### Developer Token

```go
auth := box.NewBoxDeveloperTokenAuth("<developer-token>", box.DeveloperTokenConfig{})
client := box.NewBoxClient(auth, nil)
```

## Usage

Upload a small file:

```go
f, _ := os.Open("evidence.bin")
defer f.Close()

result, err := client.Uploads.UploadFile(ctx, &managers.UploadFileRequestBody{
    Attributes: managers.UploadFileAttributes{
        Name:   "evidence.bin",
        Parent: managers.UploadFileAttributesParent{ID: "0"},
    },
    File:         f,
    FileFileName: "evidence.bin",
}, nil)
```

Upload a large file in chunks:

```go
file, _ := os.Open("large-evidence.bin")
info, _ := file.Stat()
uploaded, err := client.ChunkedUploads.UploadBigFile(ctx, file, "large-evidence.bin", info.Size(), "0")
```

## Testing

```bash
go test ./...                 # unit tests (no credentials required)
go test -tags integration ./... # integration tests (requires Box credentials)
```

Integration tests read credentials from environment variables (e.g.
`JWT_CONFIG_BASE_64`, `CLIENT_ID`, `CLIENT_SECRET`, `USER_ID`, `ENTERPRISE_ID`)
and skip automatically when they are absent.

## License

See the upstream Box SDK license for the original generated sources under
`3rdparty/`.
