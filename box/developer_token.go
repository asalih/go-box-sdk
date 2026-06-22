package box

import (
	"context"
	"strings"

	boxerrors "github.com/asalih/go-box-sdk/errors"
	"github.com/asalih/go-box-sdk/managers"
	"github.com/asalih/go-box-sdk/networking"
	"github.com/asalih/go-box-sdk/schemas"
)

// DeveloperTokenConfig holds the optional client credentials used when revoking
// a developer token. It mirrors DeveloperTokenConfig in
// src/box/developerTokenAuth.ts.
type DeveloperTokenConfig struct {
	ClientID     string
	ClientSecret string
}

// BoxDeveloperTokenAuth authenticates using a fixed developer token. It mirrors
// BoxDeveloperTokenAuth and implements networking.Authentication.
type BoxDeveloperTokenAuth struct {
	Token        string
	Config       DeveloperTokenConfig
	TokenStorage TokenStorage
}

// NewBoxDeveloperTokenAuth builds a developer-token auth seeded with the token.
func NewBoxDeveloperTokenAuth(token string, config DeveloperTokenConfig) *BoxDeveloperTokenAuth {
	return &BoxDeveloperTokenAuth{
		Token:        token,
		Config:       config,
		TokenStorage: NewInMemoryTokenStorageWithToken(&schemas.AccessToken{AccessToken: token}),
	}
}

// RetrieveToken returns the stored developer token.
func (a *BoxDeveloperTokenAuth) RetrieveToken(ctx context.Context, session *networking.NetworkSession) (*schemas.AccessToken, error) {
	token := a.TokenStorage.Get()
	if token == nil {
		return nil, boxerrors.NewBoxSDKError("No access token is available.")
	}
	return token, nil
}

// RefreshToken always fails: developer tokens cannot be refreshed.
func (a *BoxDeveloperTokenAuth) RefreshToken(ctx context.Context, session *networking.NetworkSession) (*schemas.AccessToken, error) {
	return nil, boxerrors.NewBoxSDKError("Developer token has expired. Please provide a new one.")
}

// RetrieveAuthorizationHeader returns the Bearer authorization header value.
func (a *BoxDeveloperTokenAuth) RetrieveAuthorizationHeader(ctx context.Context, session *networking.NetworkSession) (string, error) {
	token, err := a.RetrieveToken(ctx, session)
	if err != nil {
		return "", err
	}
	return "Bearer " + token.AccessToken, nil
}

// RevokeToken revokes the developer token and clears it from storage.
func (a *BoxDeveloperTokenAuth) RevokeToken(ctx context.Context, session *networking.NetworkSession) error {
	token := a.TokenStorage.Get()
	if token == nil {
		return nil
	}
	authManager := managers.NewAuthorizationManager(nil, sessionOrDefault(session))
	if err := authManager.RevokeAccessToken(ctx, &schemas.PostOAuth2Revoke{
		ClientID:     a.Config.ClientID,
		ClientSecret: a.Config.ClientSecret,
		Token:        token.AccessToken,
	}, nil); err != nil {
		return err
	}
	a.TokenStorage.Clear()
	return nil
}

// DownscopeToken exchanges the developer token for one restricted to the given
// scopes and (optionally) resource or shared link.
func (a *BoxDeveloperTokenAuth) DownscopeToken(ctx context.Context, scopes []string, resource, sharedLink string, session *networking.NetworkSession) (*schemas.AccessToken, error) {
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

// Ensure all four auth types satisfy the Authentication interface.
var (
	_ networking.Authentication = (*BoxCcgAuth)(nil)
	_ networking.Authentication = (*BoxJwtAuth)(nil)
	_ networking.Authentication = (*BoxOAuth)(nil)
	_ networking.Authentication = (*BoxDeveloperTokenAuth)(nil)
)
