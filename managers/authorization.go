package managers

import (
	"context"
	"net/http"

	"github.com/asalih/go-box-sdk/networking"
	"github.com/asalih/go-box-sdk/schemas"
)

// AuthorizationManager exposes the OAuth2 token endpoints used by the auth
// layer. It mirrors AuthorizationManager in src/managers/authorization.ts,
// limited to requesting, refreshing, and revoking access tokens.
type AuthorizationManager struct {
	baseManager
}

// NewAuthorizationManager returns an AuthorizationManager bound to the given
// auth and session.
func NewAuthorizationManager(auth networking.Authentication, session *networking.NetworkSession) *AuthorizationManager {
	return &AuthorizationManager{baseManager{Auth: auth, NetworkSession: session}}
}

// RequestAccessToken exchanges a grant (authorization code, JWT assertion, or
// client credentials) for an access token. It posts the form-encoded body to
// the /oauth2/token endpoint.
func (m *AuthorizationManager) RequestAccessToken(ctx context.Context, requestBody *schemas.PostOAuth2Token, extraHeaders map[string]string) (*schemas.AccessToken, error) {
	resp, err := m.fetch(ctx, &networking.FetchOptions{
		URL:            m.NetworkSession.BaseURLs.BaseURL + "/oauth2/token",
		Method:         http.MethodPost,
		Headers:        prepareParams(extraHeaders),
		Data:           requestBody,
		ContentType:    networking.ContentTypeURLEncoded,
		ResponseFormat: networking.ResponseFormatJSON,
	})
	if err != nil {
		return nil, err
	}
	return schemas.Decode[*schemas.AccessToken](resp.Data)
}

// RefreshAccessToken obtains a fresh access token using a refresh token grant.
// The source uses the /oauth2/token#refresh URL; the fragment is informational
// and not transmitted on the wire.
func (m *AuthorizationManager) RefreshAccessToken(ctx context.Context, requestBody *schemas.PostOAuth2Token, extraHeaders map[string]string) (*schemas.AccessToken, error) {
	resp, err := m.fetch(ctx, &networking.FetchOptions{
		URL:            m.NetworkSession.BaseURLs.BaseURL + "/oauth2/token#refresh",
		Method:         http.MethodPost,
		Headers:        prepareParams(extraHeaders),
		Data:           requestBody,
		ContentType:    networking.ContentTypeURLEncoded,
		ResponseFormat: networking.ResponseFormatJSON,
	})
	if err != nil {
		return nil, err
	}
	return schemas.Decode[*schemas.AccessToken](resp.Data)
}

// RevokeAccessToken revokes an active access (or refresh) token.
func (m *AuthorizationManager) RevokeAccessToken(ctx context.Context, requestBody *schemas.PostOAuth2Revoke, extraHeaders map[string]string) error {
	_, err := m.fetch(ctx, &networking.FetchOptions{
		URL:            m.NetworkSession.BaseURLs.BaseURL + "/oauth2/revoke",
		Method:         http.MethodPost,
		Headers:        prepareParams(extraHeaders),
		Data:           requestBody,
		ContentType:    networking.ContentTypeURLEncoded,
		ResponseFormat: networking.ResponseFormatNoContent,
	})
	return err
}
