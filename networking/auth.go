package networking

import (
	"context"

	"github.com/asalih/go-box-sdk/schemas"
)

// Authentication is implemented by every auth method (CCG, JWT, OAuth2,
// Developer Token). It mirrors the Authentication interface in
// src/networking/auth.ts, with Promise<T> mapped to (T, error) and the
// cancellation token mapped to context.Context.
type Authentication interface {
	// RetrieveToken returns a valid access token, fetching one if necessary.
	RetrieveToken(ctx context.Context, session *NetworkSession) (*schemas.AccessToken, error)
	// RefreshToken forces a new access token to be fetched.
	RefreshToken(ctx context.Context, session *NetworkSession) (*schemas.AccessToken, error)
	// RetrieveAuthorizationHeader returns the value for the Authorization header.
	RetrieveAuthorizationHeader(ctx context.Context, session *NetworkSession) (string, error)
	// RevokeToken revokes the current access token.
	RevokeToken(ctx context.Context, session *NetworkSession) error
	// DownscopeToken exchanges the current token for one with reduced scopes.
	DownscopeToken(ctx context.Context, scopes []string, resource, sharedLink string, session *NetworkSession) (*schemas.AccessToken, error)
}
