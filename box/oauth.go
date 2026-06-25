package box

import (
	"context"
	"strings"
	"sync"

	boxerrors "github.com/asalih/go-box-sdk/errors"
	"github.com/asalih/go-box-sdk/managers"
	"github.com/asalih/go-box-sdk/networking"
	"github.com/asalih/go-box-sdk/schemas"
	"github.com/asalih/go-box-sdk/serialization"
)

// OAuthConfig configures OAuth2 authorization-code authentication. It mirrors
// OAuthConfig in src/box/oauth.ts.
type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	TokenStorage TokenStorage
}

// GetAuthorizeURLOptions carries the optional parameters of GetAuthorizeURL.
type GetAuthorizeURLOptions struct {
	ClientID     string
	RedirectURI  string
	ResponseType string
	State        string
	Scope        string
}

// BoxOAuth authenticates using the OAuth2 authorization-code grant. It mirrors
// BoxOAuth and implements networking.Authentication. A BoxOAuth must not be
// copied after first use: it holds a mutex that serializes token refresh. Use
// it through a pointer (as the New* constructor returns).
type BoxOAuth struct {
	Config       OAuthConfig
	TokenStorage TokenStorage
	// mu serializes token refresh. This is critical for OAuth: Box rotates
	// refresh tokens (each is single-use), so unsynchronized concurrent
	// refreshes would race to consume the same refresh token and fail.
	mu sync.Mutex
}

// NewBoxOAuth builds a BoxOAuth from the given config.
func NewBoxOAuth(config OAuthConfig) *BoxOAuth {
	if config.TokenStorage == nil {
		config.TokenStorage = NewInMemoryTokenStorage()
	}
	return &BoxOAuth{Config: config, TokenStorage: config.TokenStorage}
}

// GetAuthorizeURL returns the URL to which the user is sent to authorize the
// application. It mirrors getAuthorizeUrl.
func (a *BoxOAuth) GetAuthorizeURL(options GetAuthorizeURLOptions) string {
	clientID := options.ClientID
	if clientID == "" {
		clientID = a.Config.ClientID
	}
	responseType := options.ResponseType
	if responseType == "" {
		responseType = "code"
	}
	params := map[string]any{
		"client_id":     clientID,
		"response_type": responseType,
		"redirect_uri":  options.RedirectURI,
		"state":         options.State,
		"scope":         options.Scope,
	}
	for k, v := range params {
		if v == "" {
			delete(params, k)
		}
	}
	encoded, _ := serialization.SDToURLParams(params)
	return "https://account.box.com/api/oauth2/authorize?" + encoded
}

// GetTokensAuthorizationCodeGrant exchanges an authorization code for tokens and
// stores them. It mirrors getTokensAuthorizationCodeGrant.
func (a *BoxOAuth) GetTokensAuthorizationCodeGrant(ctx context.Context, authorizationCode string, session *networking.NetworkSession) (*schemas.AccessToken, error) {
	authManager := managers.NewAuthorizationManager(nil, sessionOrDefault(session))
	token, err := authManager.RequestAccessToken(ctx, &schemas.PostOAuth2Token{
		GrantType:    schemas.GrantTypeAuthorizationCode,
		ClientID:     a.Config.ClientID,
		ClientSecret: a.Config.ClientSecret,
		Code:         authorizationCode,
	}, nil)
	if err != nil {
		return nil, err
	}
	a.TokenStorage.Store(token)
	return token, nil
}

// RetrieveToken returns the cached token. Unlike CCG/JWT, OAuth requires a
// prior authentication step, so a missing token is an error.
func (a *BoxOAuth) RetrieveToken(ctx context.Context, session *networking.NetworkSession) (*schemas.AccessToken, error) {
	token := a.TokenStorage.Get()
	if token == nil {
		return nil, boxerrors.NewBoxSDKError("Access and refresh tokens not available. Authenticate before making any API call first.")
	}
	return token, nil
}

// RefreshToken obtains a new access token using the stored refresh token and
// stores the result. It mirrors refreshToken. Concurrent calls are serialized:
// a caller that blocks while another goroutine refreshes returns that fresh
// token rather than re-using the now-consumed refresh token.
func (a *BoxOAuth) RefreshToken(ctx context.Context, session *networking.NetworkSession) (*schemas.AccessToken, error) {
	oldToken := a.TokenStorage.Get()
	before := ""
	if oldToken != nil {
		before = oldToken.AccessToken
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	// Another goroutine may have refreshed while we waited for the lock. Re-read
	// before consuming the (single-use) refresh token.
	oldToken = a.TokenStorage.Get()
	if oldToken != nil && oldToken.AccessToken != before {
		return oldToken, nil
	}
	refreshToken := ""
	if oldToken != nil {
		refreshToken = oldToken.RefreshToken
	}
	authManager := managers.NewAuthorizationManager(nil, sessionOrDefault(session))
	token, err := authManager.RequestAccessToken(ctx, &schemas.PostOAuth2Token{
		GrantType:    schemas.GrantTypeRefreshToken,
		ClientID:     a.Config.ClientID,
		ClientSecret: a.Config.ClientSecret,
		RefreshToken: refreshToken,
	}, nil)
	if err != nil {
		return nil, err
	}
	a.TokenStorage.Store(token)
	return token, nil
}

// RetrieveAuthorizationHeader returns the Bearer authorization header value.
func (a *BoxOAuth) RetrieveAuthorizationHeader(ctx context.Context, session *networking.NetworkSession) (string, error) {
	token, err := a.RetrieveToken(ctx, session)
	if err != nil {
		return "", err
	}
	return "Bearer " + token.AccessToken, nil
}

// RevokeToken revokes the stored access token. It mirrors revokeToken.
func (a *BoxOAuth) RevokeToken(ctx context.Context, session *networking.NetworkSession) error {
	token := a.TokenStorage.Get()
	if token == nil {
		return nil
	}
	authManager := managers.NewAuthorizationManager(nil, sessionOrDefault(session))
	return authManager.RevokeAccessToken(ctx, &schemas.PostOAuth2Revoke{
		ClientID:     a.Config.ClientID,
		ClientSecret: a.Config.ClientSecret,
		Token:        token.AccessToken,
	}, nil)
}

// DownscopeToken exchanges the current token for one restricted to the given
// scopes and (optionally) resource or shared link.
func (a *BoxOAuth) DownscopeToken(ctx context.Context, scopes []string, resource, sharedLink string, session *networking.NetworkSession) (*schemas.AccessToken, error) {
	token, err := a.RetrieveToken(ctx, session)
	if err != nil {
		return nil, err
	}
	if token == nil || token.AccessToken == "" {
		return nil, boxerrors.NewBoxSDKError("No access token is available.")
	}
	authManager := managers.NewAuthorizationManager(nil, sessionOrDefault(session))
	return authManager.RequestAccessToken(ctx, &schemas.PostOAuth2Token{
		GrantType:        schemas.GrantTypeTokenExchange,
		SubjectToken:     token.AccessToken,
		SubjectTokenType: schemas.SubjectTokenTypeAccessToken,
		Scope:            strings.Join(scopes, " "),
		Resource:         resource,
		BoxSharedLink:    sharedLink,
	}, nil)
}
