package box

import (
	"github.com/asalih/go-box-sdk/managers"
	"github.com/asalih/go-box-sdk/networking"
)

// BoxClient is the slim entry point that wires the focused set of managers onto
// a shared auth and NetworkSession. It mirrors the relevant subset of BoxClient
// in src/client.ts (authorization, files, folders, uploads, chunked uploads,
// downloads, plus the With* configuration helpers).
type BoxClient struct {
	Auth           networking.Authentication
	NetworkSession *networking.NetworkSession

	Authorization  *managers.AuthorizationManager
	Files          *managers.FilesManager
	Folders        *managers.FoldersManager
	Uploads        *managers.UploadsManager
	ChunkedUploads *managers.ChunkedUploadsManager
	Downloads      *managers.DownloadsManager
}

// NewBoxClient builds a client for the given auth, using default base URLs and
// network settings. Pass nil session to use the SDK defaults.
func NewBoxClient(auth networking.Authentication, session *networking.NetworkSession) *BoxClient {
	if session == nil {
		session = networking.NewNetworkSession()
	}
	return &BoxClient{
		Auth:           auth,
		NetworkSession: session,
		Authorization:  managers.NewAuthorizationManager(auth, session),
		Files:          managers.NewFilesManager(auth, session),
		Folders:        managers.NewFoldersManager(auth, session),
		Uploads:        managers.NewUploadsManager(auth, session),
		ChunkedUploads: managers.NewChunkedUploadsManager(auth, session),
		Downloads:      managers.NewDownloadsManager(auth, session),
	}
}

// WithAsUserHeader returns a new client that impersonates the given user via the
// As-User header. It mirrors withAsUserHeader.
func (c *BoxClient) WithAsUserHeader(userID string) *BoxClient {
	return NewBoxClient(c.Auth, c.NetworkSession.WithAdditionalHeaders(map[string]string{"As-User": userID}))
}

// WithSuppressedNotifications returns a new client that suppresses email and
// webhook notifications. It mirrors withSuppressedNotifications.
func (c *BoxClient) WithSuppressedNotifications() *BoxClient {
	return NewBoxClient(c.Auth, c.NetworkSession.WithAdditionalHeaders(map[string]string{"Box-Notifications": "off"}))
}

// WithExtraHeaders returns a new client that adds the given headers to every
// request. It mirrors withExtraHeaders.
func (c *BoxClient) WithExtraHeaders(extraHeaders map[string]string) *BoxClient {
	return NewBoxClient(c.Auth, c.NetworkSession.WithAdditionalHeaders(extraHeaders))
}

// WithCustomBaseURLs returns a new client that targets the given base URLs. It
// mirrors withCustomBaseUrls.
func (c *BoxClient) WithCustomBaseURLs(baseURLs *networking.BaseURLs) *BoxClient {
	return NewBoxClient(c.Auth, c.NetworkSession.WithCustomBaseURLs(baseURLs))
}

// WithProxy returns a new client that routes requests through the given proxy.
// It mirrors withProxy.
func (c *BoxClient) WithProxy(config *networking.ProxyConfig) *BoxClient {
	return NewBoxClient(c.Auth, c.NetworkSession.WithProxy(config))
}

// WithTimeouts returns a new client that applies the given per-request timeout.
// It mirrors withTimeouts.
func (c *BoxClient) WithTimeouts(config *networking.TimeoutConfig) *BoxClient {
	return NewBoxClient(c.Auth, c.NetworkSession.WithTimeoutConfig(config))
}

// WithInterceptors returns a new client with the given interceptors appended. It
// mirrors withInterceptors.
func (c *BoxClient) WithInterceptors(interceptors ...networking.Interceptor) *BoxClient {
	return NewBoxClient(c.Auth, c.NetworkSession.WithInterceptors(interceptors...))
}
