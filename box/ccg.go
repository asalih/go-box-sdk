package box

import (
	"context"
	"strings"

	boxerrors "github.com/asalih/go-box-sdk/errors"
	"github.com/asalih/go-box-sdk/managers"
	"github.com/asalih/go-box-sdk/networking"
	"github.com/asalih/go-box-sdk/schemas"
)

// CcgConfig configures Client Credentials Grant authentication. It mirrors
// CcgConfig in src/box/ccgAuth.ts.
type CcgConfig struct {
	ClientID     string
	ClientSecret string
	EnterpriseID string
	UserID       string
	TokenStorage TokenStorage
}

// BoxCcgAuth authenticates using the Client Credentials Grant. It mirrors
// BoxCcgAuth and implements networking.Authentication.
type BoxCcgAuth struct {
	Config       CcgConfig
	TokenStorage TokenStorage
	SubjectID    string
	SubjectType  string
}

// NewBoxCcgAuth builds a BoxCcgAuth from the given config. The subject defaults
// to the user when a user ID is set, otherwise the enterprise.
func NewBoxCcgAuth(config CcgConfig) *BoxCcgAuth {
	if config.TokenStorage == nil {
		config.TokenStorage = NewInMemoryTokenStorage()
	}
	subjectID := config.EnterpriseID
	subjectType := schemas.BoxSubjectTypeEnterprise
	if config.UserID != "" {
		subjectID = config.UserID
		subjectType = schemas.BoxSubjectTypeUser
	}
	return &BoxCcgAuth{
		Config:       config,
		TokenStorage: config.TokenStorage,
		SubjectID:    subjectID,
		SubjectType:  subjectType,
	}
}

// RefreshToken fetches a new access token using the client-credentials grant
// and stores it.
func (a *BoxCcgAuth) RefreshToken(ctx context.Context, session *networking.NetworkSession) (*schemas.AccessToken, error) {
	authManager := managers.NewAuthorizationManager(nil, sessionOrDefault(session))
	token, err := authManager.RequestAccessToken(ctx, &schemas.PostOAuth2Token{
		GrantType:      schemas.GrantTypeClientCredentials,
		ClientID:       a.Config.ClientID,
		ClientSecret:   a.Config.ClientSecret,
		BoxSubjectType: a.SubjectType,
		BoxSubjectID:   a.SubjectID,
	}, nil)
	if err != nil {
		return nil, err
	}
	a.TokenStorage.Store(token)
	return token, nil
}

// RetrieveToken returns the cached token, fetching a new one if absent.
func (a *BoxCcgAuth) RetrieveToken(ctx context.Context, session *networking.NetworkSession) (*schemas.AccessToken, error) {
	if token := a.TokenStorage.Get(); token != nil {
		return token, nil
	}
	return a.RefreshToken(ctx, session)
}

// RetrieveAuthorizationHeader returns the Bearer authorization header value.
func (a *BoxCcgAuth) RetrieveAuthorizationHeader(ctx context.Context, session *networking.NetworkSession) (string, error) {
	token, err := a.RetrieveToken(ctx, session)
	if err != nil {
		return "", err
	}
	return "Bearer " + token.AccessToken, nil
}

// WithUserSubject returns a new auth that authenticates as the given user.
func (a *BoxCcgAuth) WithUserSubject(userID string, tokenStorage TokenStorage) *BoxCcgAuth {
	if tokenStorage == nil {
		tokenStorage = NewInMemoryTokenStorage()
	}
	return NewBoxCcgAuth(CcgConfig{
		ClientID:     a.Config.ClientID,
		ClientSecret: a.Config.ClientSecret,
		EnterpriseID: a.Config.EnterpriseID,
		UserID:       userID,
		TokenStorage: tokenStorage,
	})
}

// WithEnterpriseSubject returns a new auth that authenticates as the given
// enterprise.
func (a *BoxCcgAuth) WithEnterpriseSubject(enterpriseID string, tokenStorage TokenStorage) *BoxCcgAuth {
	if tokenStorage == nil {
		tokenStorage = NewInMemoryTokenStorage()
	}
	return NewBoxCcgAuth(CcgConfig{
		ClientID:     a.Config.ClientID,
		ClientSecret: a.Config.ClientSecret,
		EnterpriseID: enterpriseID,
		TokenStorage: tokenStorage,
	})
}

// DownscopeToken exchanges the current token for one restricted to the given
// scopes and (optionally) resource or shared link.
func (a *BoxCcgAuth) DownscopeToken(ctx context.Context, scopes []string, resource, sharedLink string, session *networking.NetworkSession) (*schemas.AccessToken, error) {
	token, err := a.RetrieveToken(ctx, session)
	if err != nil {
		return nil, err
	}
	if token == nil {
		return nil, boxerrors.NewBoxSDKError("No access token is available. Make an API call to retrieve a token before calling this method.")
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

// RevokeToken revokes the current token and clears it from storage.
func (a *BoxCcgAuth) RevokeToken(ctx context.Context, session *networking.NetworkSession) error {
	oldToken := a.TokenStorage.Get()
	if oldToken == nil {
		return nil
	}
	authManager := managers.NewAuthorizationManager(nil, sessionOrDefault(session))
	if err := authManager.RevokeAccessToken(ctx, &schemas.PostOAuth2Revoke{
		ClientID:     a.Config.ClientID,
		ClientSecret: a.Config.ClientSecret,
		Token:        oldToken.AccessToken,
	}, nil); err != nil {
		return err
	}
	a.TokenStorage.Clear()
	return nil
}

// sessionOrDefault returns the given session or a fresh default one.
func sessionOrDefault(session *networking.NetworkSession) *networking.NetworkSession {
	if session != nil {
		return session
	}
	return networking.NewNetworkSession()
}
