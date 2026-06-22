package box

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	boxerrors "github.com/asalih/go-box-sdk/errors"
	"github.com/asalih/go-box-sdk/internal/utils"
	"github.com/asalih/go-box-sdk/managers"
	"github.com/asalih/go-box-sdk/networking"
	"github.com/asalih/go-box-sdk/schemas"
)

// JwtConfig holds the JWT configuration. It mirrors JwtConfig in
// src/box/jwtAuth.ts.
type JwtConfig struct {
	ClientID             string
	ClientSecret         string
	JwtKeyID             string
	PrivateKey           string
	PrivateKeyPassphrase string
	EnterpriseID         string
	UserID               string
	Algorithm            string
	TokenStorage         TokenStorage
	PrivateKeyDecryptor  utils.PrivateKeyDecryptor
}

// jwtConfigFile mirrors the JSON config downloaded from the Box Developer
// Console (the boxAppSettings/appAuth shape).
type jwtConfigFile struct {
	EnterpriseID   string `json:"enterpriseID"`
	UserID         string `json:"userID"`
	BoxAppSettings struct {
		ClientID     string `json:"clientID"`
		ClientSecret string `json:"clientSecret"`
		AppAuth      struct {
			PublicKeyID string `json:"publicKeyID"`
			PrivateKey  string `json:"privateKey"`
			Passphrase  string `json:"passphrase"`
		} `json:"appAuth"`
	} `json:"boxAppSettings"`
}

// JwtConfigFromConfigJSONString builds a JwtConfig from the JSON string of a
// Box Developer Console config file. It mirrors JwtConfig.fromConfigJsonString.
func JwtConfigFromConfigJSONString(configJSON string, tokenStorage TokenStorage, decryptor utils.PrivateKeyDecryptor) (*JwtConfig, error) {
	var file jwtConfigFile
	if err := json.Unmarshal([]byte(configJSON), &file); err != nil {
		return nil, fmt.Errorf("box: failed to parse JWT config JSON: %w", err)
	}
	if file.BoxAppSettings.ClientID == "" {
		return nil, boxerrors.NewBoxSDKError(`Expecting "clientID" of type "JwtConfigAppSettings" to be defined`)
	}
	if tokenStorage == nil {
		tokenStorage = NewInMemoryTokenStorage()
	}
	if decryptor == nil {
		decryptor = utils.NewDefaultPrivateKeyDecryptor()
	}
	return &JwtConfig{
		ClientID:             file.BoxAppSettings.ClientID,
		ClientSecret:         file.BoxAppSettings.ClientSecret,
		EnterpriseID:         file.EnterpriseID,
		UserID:               file.UserID,
		JwtKeyID:             file.BoxAppSettings.AppAuth.PublicKeyID,
		PrivateKey:           file.BoxAppSettings.AppAuth.PrivateKey,
		PrivateKeyPassphrase: file.BoxAppSettings.AppAuth.Passphrase,
		Algorithm:            "RS256",
		TokenStorage:         tokenStorage,
		PrivateKeyDecryptor:  decryptor,
	}, nil
}

// JwtConfigFromConfigFile builds a JwtConfig from a Box Developer Console config
// file path. It mirrors JwtConfig.fromConfigFile.
func JwtConfigFromConfigFile(configFilePath string, tokenStorage TokenStorage, decryptor utils.PrivateKeyDecryptor) (*JwtConfig, error) {
	content, err := utils.ReadTextFromFile(configFilePath)
	if err != nil {
		return nil, fmt.Errorf("box: failed to read JWT config file: %w", err)
	}
	return JwtConfigFromConfigJSONString(content, tokenStorage, decryptor)
}

// BoxJwtAuth authenticates using a signed JWT assertion. It mirrors BoxJwtAuth
// and implements networking.Authentication.
type BoxJwtAuth struct {
	Config       JwtConfig
	TokenStorage TokenStorage
	SubjectID    string
	SubjectType  string
}

// NewBoxJwtAuth builds a BoxJwtAuth from the given config. The subject defaults
// to the enterprise when an enterprise ID is set, otherwise the user.
func NewBoxJwtAuth(config JwtConfig) *BoxJwtAuth {
	if config.TokenStorage == nil {
		config.TokenStorage = NewInMemoryTokenStorage()
	}
	if config.PrivateKeyDecryptor == nil {
		config.PrivateKeyDecryptor = utils.NewDefaultPrivateKeyDecryptor()
	}
	if config.Algorithm == "" {
		config.Algorithm = "RS256"
	}
	subjectID := config.UserID
	subjectType := "user"
	if config.EnterpriseID != "" {
		subjectID = config.EnterpriseID
		subjectType = "enterprise"
	}
	return &BoxJwtAuth{
		Config:       config,
		TokenStorage: config.TokenStorage,
		SubjectID:    subjectID,
		SubjectType:  subjectType,
	}
}

// RefreshToken signs a fresh JWT assertion, exchanges it for an access token,
// and stores the token.
func (a *BoxJwtAuth) RefreshToken(ctx context.Context, session *networking.NetworkSession) (*schemas.AccessToken, error) {
	claims := map[string]any{
		"exp":          utils.GetEpochTimeInSeconds() + 30,
		"box_sub_type": a.SubjectType,
	}
	assertion, err := utils.CreateJWTAssertion(claims, utils.JwtKey{
		Key:        a.Config.PrivateKey,
		Passphrase: a.Config.PrivateKeyPassphrase,
	}, utils.JwtSignOptions{
		Algorithm:           a.Config.Algorithm,
		Audience:            "https://api.box.com/oauth2/token",
		Subject:             a.SubjectID,
		Issuer:              a.Config.ClientID,
		JWTID:               utils.GetUUID(),
		KeyID:               a.Config.JwtKeyID,
		PrivateKeyDecryptor: a.Config.PrivateKeyDecryptor,
	})
	if err != nil {
		return nil, err
	}
	authManager := managers.NewAuthorizationManager(nil, sessionOrDefault(session))
	token, err := authManager.RequestAccessToken(ctx, &schemas.PostOAuth2Token{
		GrantType:    schemas.GrantTypeJWTBearer,
		Assertion:    assertion,
		ClientID:     a.Config.ClientID,
		ClientSecret: a.Config.ClientSecret,
	}, nil)
	if err != nil {
		return nil, err
	}
	a.TokenStorage.Store(token)
	return token, nil
}

// RetrieveToken returns the cached token, fetching a new one if absent.
func (a *BoxJwtAuth) RetrieveToken(ctx context.Context, session *networking.NetworkSession) (*schemas.AccessToken, error) {
	if token := a.TokenStorage.Get(); token != nil {
		return token, nil
	}
	return a.RefreshToken(ctx, session)
}

// RetrieveAuthorizationHeader returns the Bearer authorization header value.
func (a *BoxJwtAuth) RetrieveAuthorizationHeader(ctx context.Context, session *networking.NetworkSession) (string, error) {
	token, err := a.RetrieveToken(ctx, session)
	if err != nil {
		return "", err
	}
	return "Bearer " + token.AccessToken, nil
}

// WithUserSubject returns a new auth that signs assertions for the given user.
func (a *BoxJwtAuth) WithUserSubject(userID string, tokenStorage TokenStorage) *BoxJwtAuth {
	if tokenStorage == nil {
		tokenStorage = NewInMemoryTokenStorage()
	}
	cfg := a.Config
	cfg.EnterpriseID = ""
	cfg.UserID = userID
	cfg.TokenStorage = tokenStorage
	return NewBoxJwtAuth(cfg)
}

// WithEnterpriseSubject returns a new auth that signs assertions for the given
// enterprise.
func (a *BoxJwtAuth) WithEnterpriseSubject(enterpriseID string, tokenStorage TokenStorage) *BoxJwtAuth {
	if tokenStorage == nil {
		tokenStorage = NewInMemoryTokenStorage()
	}
	cfg := a.Config
	cfg.EnterpriseID = enterpriseID
	cfg.UserID = ""
	cfg.TokenStorage = tokenStorage
	return NewBoxJwtAuth(cfg)
}

// DownscopeToken exchanges the current token for one restricted to the given
// scopes and (optionally) resource or shared link.
func (a *BoxJwtAuth) DownscopeToken(ctx context.Context, scopes []string, resource, sharedLink string, session *networking.NetworkSession) (*schemas.AccessToken, error) {
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
		Resource:         resource,
		Scope:            strings.Join(scopes, " "),
		BoxSharedLink:    sharedLink,
	}, nil)
}

// RevokeToken revokes the current token and clears it from storage.
func (a *BoxJwtAuth) RevokeToken(ctx context.Context, session *networking.NetworkSession) error {
	oldToken := a.TokenStorage.Get()
	if oldToken == nil {
		return nil
	}
	authManager := managers.NewAuthorizationManager(nil, sessionOrDefault(session))
	if err := authManager.RevokeAccessToken(ctx, &schemas.PostOAuth2Revoke{
		Token:        oldToken.AccessToken,
		ClientID:     a.Config.ClientID,
		ClientSecret: a.Config.ClientSecret,
	}, nil); err != nil {
		return err
	}
	a.TokenStorage.Clear()
	return nil
}
