package box

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/asalih/go-box-sdk/networking"
	"github.com/asalih/go-box-sdk/schemas"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testSession returns a NetworkSession whose base URL points at the test server.
func testSession(serverURL string) *networking.NetworkSession {
	session := networking.NewNetworkSession()
	session.BaseURLs = &networking.BaseURLs{
		BaseURL:   serverURL,
		UploadURL: serverURL,
		OAuth2URL: serverURL,
	}
	return session
}

func TestGetAuthorizeURL(t *testing.T) {
	auth := NewBoxOAuth(OAuthConfig{ClientID: "OAUTH_CLIENT_ID", ClientSecret: "secret"})
	raw := auth.GetAuthorizeURL(GetAuthorizeURLOptions{})

	parsed, err := url.Parse(raw)
	require.NoError(t, err)
	assert.Equal(t, "account.box.com", parsed.Host)
	assert.Equal(t, "/api/oauth2/authorize", parsed.Path)
	q := parsed.Query()
	assert.Equal(t, "OAUTH_CLIENT_ID", q.Get("client_id"))
	assert.Equal(t, "code", q.Get("response_type"))
	// No empty optional params should be present.
	assert.Empty(t, q.Get("redirect_uri"))
	assert.Empty(t, q.Get("state"))
	assert.Empty(t, q.Get("scope"))
}

func TestGetAuthorizeURLWithOptions(t *testing.T) {
	auth := NewBoxOAuth(OAuthConfig{ClientID: "OAUTH_CLIENT_ID", ClientSecret: "secret"})
	raw := auth.GetAuthorizeURL(GetAuthorizeURLOptions{
		RedirectURI: "https://app.example.com/cb",
		State:       "xyz",
		Scope:       "root_readwrite",
	})
	parsed, err := url.Parse(raw)
	require.NoError(t, err)
	q := parsed.Query()
	assert.Equal(t, "https://app.example.com/cb", q.Get("redirect_uri"))
	assert.Equal(t, "xyz", q.Get("state"))
	assert.Equal(t, "root_readwrite", q.Get("scope"))
}

func TestDeveloperTokenAuthHeaderAndRefresh(t *testing.T) {
	auth := NewBoxDeveloperTokenAuth("dev_token_123", DeveloperTokenConfig{})

	header, err := auth.RetrieveAuthorizationHeader(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, "Bearer dev_token_123", header)

	// Developer tokens cannot be refreshed.
	_, err = auth.RefreshToken(context.Background(), nil)
	require.Error(t, err)
}

func TestCcgAuthTokenCaching(t *testing.T) {
	var calls int
	var lastForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/oauth2/token", r.URL.Path)
		require.NoError(t, r.ParseForm())
		lastForm = r.PostForm
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"ccg_token","expires_in":3600,"token_type":"bearer"}`))
	}))
	defer srv.Close()

	auth := NewBoxCcgAuth(CcgConfig{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		EnterpriseID: "ent-1",
	})
	session := testSession(srv.URL)

	header, err := auth.RetrieveAuthorizationHeader(context.Background(), session)
	require.NoError(t, err)
	assert.Equal(t, "Bearer ccg_token", header)

	// The form body carries the client-credentials grant and enterprise subject.
	assert.Equal(t, "client_credentials", lastForm.Get("grant_type"))
	assert.Equal(t, "client-id", lastForm.Get("client_id"))
	assert.Equal(t, "client-secret", lastForm.Get("client_secret"))
	assert.Equal(t, "enterprise", lastForm.Get("box_subject_type"))
	assert.Equal(t, "ent-1", lastForm.Get("box_subject_id"))

	// A second retrieval is served from the token cache (no new network call).
	_, err = auth.RetrieveAuthorizationHeader(context.Background(), session)
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestCcgAuthSubjectSelection(t *testing.T) {
	// A user ID selects the user subject over the enterprise.
	userAuth := NewBoxCcgAuth(CcgConfig{ClientID: "c", ClientSecret: "s", EnterpriseID: "ent", UserID: "user-9"})
	assert.Equal(t, "user", userAuth.SubjectType)
	assert.Equal(t, "user-9", userAuth.SubjectID)

	entAuth := NewBoxCcgAuth(CcgConfig{ClientID: "c", ClientSecret: "s", EnterpriseID: "ent"})
	assert.Equal(t, "enterprise", entAuth.SubjectType)
	assert.Equal(t, "ent", entAuth.SubjectID)
}

func TestJwtAuthSignsValidAssertion(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pkcs8, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)
	privatePEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}))

	var assertion string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/oauth2/token", r.URL.Path)
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "urn:ietf:params:oauth:grant-type:jwt-bearer", r.PostForm.Get("grant_type"))
		assertion = r.PostForm.Get("assertion")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"jwt_token","expires_in":3600,"token_type":"bearer"}`))
	}))
	defer srv.Close()

	auth := NewBoxJwtAuth(JwtConfig{
		ClientID:     "jwt-client",
		ClientSecret: "jwt-secret",
		JwtKeyID:     "key-42",
		PrivateKey:   privatePEM,
		EnterpriseID: "ent-7",
	})

	header, err := auth.RetrieveAuthorizationHeader(context.Background(), testSession(srv.URL))
	require.NoError(t, err)
	assert.Equal(t, "Bearer jwt_token", header)
	require.NotEmpty(t, assertion)

	// Verify the signed assertion with the public key and inspect its claims.
	parsed, err := jwt.Parse(assertion, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return &privateKey.PublicKey, nil
	})
	require.NoError(t, err)
	require.True(t, parsed.Valid)
	assert.Equal(t, "key-42", parsed.Header["kid"])

	claims, ok := parsed.Claims.(jwt.MapClaims)
	require.True(t, ok)
	assert.Equal(t, "jwt-client", claims["iss"])
	assert.Equal(t, "ent-7", claims["sub"])
	assert.Equal(t, "enterprise", claims["box_sub_type"])
	assert.Equal(t, "https://api.box.com/oauth2/token", claims["aud"])
	assert.NotEmpty(t, claims["jti"])
	assert.NotEmpty(t, claims["exp"])
}

func TestInMemoryTokenStorage(t *testing.T) {
	storage := NewInMemoryTokenStorage()
	assert.Nil(t, storage.Get())

	storage.Store(&schemas.AccessToken{AccessToken: "stored"})
	got := storage.Get()
	require.NotNil(t, got)
	assert.Equal(t, "stored", got.AccessToken)

	storage.Clear()
	assert.Nil(t, storage.Get())
}

// authServer is a mock Box auth backend serving /oauth2/token and
// /oauth2/revoke. It records the posted forms for assertions.
type authServer struct {
	*httptest.Server
	mu          sync.Mutex
	tokenForms  []url.Values
	revokeForms []url.Values
}

func (a *authServer) lastTokenForm(t *testing.T) url.Values {
	t.Helper()
	a.mu.Lock()
	defer a.mu.Unlock()
	require.NotEmpty(t, a.tokenForms, "expected a /oauth2/token request")
	return a.tokenForms[len(a.tokenForms)-1]
}

func (a *authServer) lastRevokeForm(t *testing.T) url.Values {
	t.Helper()
	a.mu.Lock()
	defer a.mu.Unlock()
	require.NotEmpty(t, a.revokeForms, "expected a /oauth2/revoke request")
	return a.revokeForms[len(a.revokeForms)-1]
}

func (a *authServer) revokeCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.revokeForms)
}

func newAuthServer(t *testing.T) *authServer {
	t.Helper()
	a := &authServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		a.mu.Lock()
		a.tokenForms = append(a.tokenForms, r.PostForm)
		n := len(a.tokenForms)
		a.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"access_token":"tok_%d","refresh_token":"rt_%d","expires_in":3600,"token_type":"bearer"}`, n, n)
	})
	mux.HandleFunc("/oauth2/revoke", func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		a.mu.Lock()
		a.revokeForms = append(a.revokeForms, r.PostForm)
		a.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})
	a.Server = httptest.NewServer(mux)
	t.Cleanup(a.Close)
	return a
}

func TestCcgRefreshTokenStoresAndReplaces(t *testing.T) {
	srv := newAuthServer(t)
	auth := NewBoxCcgAuth(CcgConfig{ClientID: "c", ClientSecret: "s", EnterpriseID: "ent"})
	session := testSession(srv.URL)

	first, err := auth.RefreshToken(context.Background(), session)
	require.NoError(t, err)
	assert.Equal(t, "tok_1", first.AccessToken)

	// A forced refresh issues a new request and replaces the cached token.
	second, err := auth.RefreshToken(context.Background(), session)
	require.NoError(t, err)
	assert.Equal(t, "tok_2", second.AccessToken)
	assert.Equal(t, "tok_2", auth.TokenStorage.Get().AccessToken)
}

func TestCcgRevokeToken(t *testing.T) {
	srv := newAuthServer(t)
	auth := NewBoxCcgAuth(CcgConfig{ClientID: "client-id", ClientSecret: "client-secret", EnterpriseID: "ent"})
	session := testSession(srv.URL)

	// Revoking with no token is a no-op (no network call).
	require.NoError(t, auth.RevokeToken(context.Background(), session))
	assert.Equal(t, 0, srv.revokeCount())

	_, err := auth.RetrieveToken(context.Background(), session)
	require.NoError(t, err)
	require.NotNil(t, auth.TokenStorage.Get())

	require.NoError(t, auth.RevokeToken(context.Background(), session))
	form := srv.lastRevokeForm(t)
	assert.Equal(t, "client-id", form.Get("client_id"))
	assert.Equal(t, "client-secret", form.Get("client_secret"))
	assert.Equal(t, "tok_1", form.Get("token"))
	// Storage is cleared after a successful revoke.
	assert.Nil(t, auth.TokenStorage.Get())
}

func TestCcgDownscopeToken(t *testing.T) {
	srv := newAuthServer(t)
	auth := NewBoxCcgAuth(CcgConfig{ClientID: "c", ClientSecret: "s", EnterpriseID: "ent"})
	session := testSession(srv.URL)

	downscoped, err := auth.DownscopeToken(context.Background(),
		[]string{"item_preview", "item_download"}, "https://api.box.com/2.0/files/123", "shared-link", session)
	require.NoError(t, err)
	require.NotNil(t, downscoped)

	// The token-exchange request carries the parent token as the subject.
	form := srv.lastTokenForm(t)
	assert.Equal(t, schemas.GrantTypeTokenExchange, form.Get("grant_type"))
	assert.Equal(t, schemas.SubjectTokenTypeAccessToken, form.Get("subject_token_type"))
	assert.Equal(t, "tok_1", form.Get("subject_token")) // parent token fetched first
	assert.Equal(t, "item_preview item_download", form.Get("scope"))
	assert.Equal(t, "https://api.box.com/2.0/files/123", form.Get("resource"))
	assert.Equal(t, "shared-link", form.Get("box_shared_link"))
}

func TestCcgWithSubjectSwitching(t *testing.T) {
	base := NewBoxCcgAuth(CcgConfig{ClientID: "c", ClientSecret: "s", EnterpriseID: "ent"})

	user := base.WithUserSubject("user-1", nil)
	assert.Equal(t, schemas.BoxSubjectTypeUser, user.SubjectType)
	assert.Equal(t, "user-1", user.SubjectID)
	assert.Equal(t, "c", user.Config.ClientID)
	// Each subject gets its own token storage, independent of the base.
	assert.NotSame(t, base.TokenStorage, user.TokenStorage)

	ent := base.WithEnterpriseSubject("ent-2", nil)
	assert.Equal(t, schemas.BoxSubjectTypeEnterprise, ent.SubjectType)
	assert.Equal(t, "ent-2", ent.SubjectID)
}

func TestOAuthGetTokensAuthorizationCodeGrant(t *testing.T) {
	srv := newAuthServer(t)
	auth := NewBoxOAuth(OAuthConfig{ClientID: "oauth-client", ClientSecret: "oauth-secret"})
	session := testSession(srv.URL)

	token, err := auth.GetTokensAuthorizationCodeGrant(context.Background(), "auth-code-xyz", session)
	require.NoError(t, err)
	assert.Equal(t, "tok_1", token.AccessToken)

	form := srv.lastTokenForm(t)
	assert.Equal(t, schemas.GrantTypeAuthorizationCode, form.Get("grant_type"))
	assert.Equal(t, "auth-code-xyz", form.Get("code"))
	assert.Equal(t, "oauth-client", form.Get("client_id"))
	// The token is now cached.
	require.NotNil(t, auth.TokenStorage.Get())
}

func TestOAuthRetrieveTokenWithoutAuthErrors(t *testing.T) {
	auth := NewBoxOAuth(OAuthConfig{ClientID: "c", ClientSecret: "s"})

	_, err := auth.RetrieveToken(context.Background(), networking.NewNetworkSession())
	require.Error(t, err)

	_, err = auth.RetrieveAuthorizationHeader(context.Background(), networking.NewNetworkSession())
	require.Error(t, err)
}

func TestOAuthRefreshToken(t *testing.T) {
	srv := newAuthServer(t)
	auth := NewBoxOAuth(OAuthConfig{ClientID: "c", ClientSecret: "s"})
	auth.TokenStorage.Store(&schemas.AccessToken{AccessToken: "old", RefreshToken: "old-refresh"})
	session := testSession(srv.URL)

	token, err := auth.RefreshToken(context.Background(), session)
	require.NoError(t, err)
	assert.Equal(t, "tok_1", token.AccessToken)

	form := srv.lastTokenForm(t)
	assert.Equal(t, schemas.GrantTypeRefreshToken, form.Get("grant_type"))
	assert.Equal(t, "old-refresh", form.Get("refresh_token"))
}

func TestOAuthRevokeAndDownscope(t *testing.T) {
	srv := newAuthServer(t)
	auth := NewBoxOAuth(OAuthConfig{ClientID: "c", ClientSecret: "s"})
	session := testSession(srv.URL)

	// Revoke with no token is a no-op.
	require.NoError(t, auth.RevokeToken(context.Background(), session))
	assert.Equal(t, 0, srv.revokeCount())

	auth.TokenStorage.Store(&schemas.AccessToken{AccessToken: "live-token"})
	downscoped, err := auth.DownscopeToken(context.Background(), []string{"item_preview"}, "", "", session)
	require.NoError(t, err)
	require.NotNil(t, downscoped)
	assert.Equal(t, "live-token", srv.lastTokenForm(t).Get("subject_token"))

	require.NoError(t, auth.RevokeToken(context.Background(), session))
	assert.Equal(t, "live-token", srv.lastRevokeForm(t).Get("token"))
}

func TestJwtWithSubjectSwitching(t *testing.T) {
	base := NewBoxJwtAuth(JwtConfig{
		ClientID:     "jwt-client",
		ClientSecret: "jwt-secret",
		JwtKeyID:     "key-1",
		PrivateKey:   testRSAPrivateKeyPEM(t),
		EnterpriseID: "ent-1",
	})
	assert.Equal(t, "enterprise", base.SubjectType)
	assert.Equal(t, "ent-1", base.SubjectID)

	user := base.WithUserSubject("user-9", nil)
	assert.Equal(t, "user", user.SubjectType)
	assert.Equal(t, "user-9", user.SubjectID)
	assert.Empty(t, user.Config.EnterpriseID)
	assert.Equal(t, "jwt-client", user.Config.ClientID)

	ent := user.WithEnterpriseSubject("ent-3", nil)
	assert.Equal(t, "enterprise", ent.SubjectType)
	assert.Equal(t, "ent-3", ent.SubjectID)
	assert.Empty(t, ent.Config.UserID)
}

func TestJwtRevokeAndDownscope(t *testing.T) {
	srv := newAuthServer(t)
	auth := NewBoxJwtAuth(JwtConfig{
		ClientID:     "jwt-client",
		ClientSecret: "jwt-secret",
		JwtKeyID:     "key-1",
		PrivateKey:   testRSAPrivateKeyPEM(t),
		EnterpriseID: "ent-1",
	})
	session := testSession(srv.URL)

	// Retrieve (cold cache -> signs assertion + exchanges) then downscope.
	_, err := auth.RetrieveToken(context.Background(), session)
	require.NoError(t, err)

	downscoped, err := auth.DownscopeToken(context.Background(), []string{"item_upload"}, "resource", "", session)
	require.NoError(t, err)
	require.NotNil(t, downscoped)
	assert.Equal(t, schemas.GrantTypeTokenExchange, srv.lastTokenForm(t).Get("grant_type"))

	require.NoError(t, auth.RevokeToken(context.Background(), session))
	assert.Equal(t, "jwt-client", srv.lastRevokeForm(t).Get("client_id"))
	assert.Nil(t, auth.TokenStorage.Get())
}

const jwtConfigJSONTemplate = `{
  "enterpriseID": "ent-123",
  "boxAppSettings": {
    "clientID": "%s",
    "clientSecret": "cfg-secret",
    "appAuth": {
      "publicKeyID": "pk-1",
      "privateKey": "PEM-DATA",
      "passphrase": "phrase"
    }
  }
}`

func TestJwtConfigFromConfigJSONString(t *testing.T) {
	cfg, err := JwtConfigFromConfigJSONString(fmt.Sprintf(jwtConfigJSONTemplate, "cfg-client"), nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "cfg-client", cfg.ClientID)
	assert.Equal(t, "cfg-secret", cfg.ClientSecret)
	assert.Equal(t, "ent-123", cfg.EnterpriseID)
	assert.Equal(t, "pk-1", cfg.JwtKeyID)
	assert.Equal(t, "PEM-DATA", cfg.PrivateKey)
	assert.Equal(t, "phrase", cfg.PrivateKeyPassphrase)
	assert.Equal(t, "RS256", cfg.Algorithm)
	require.NotNil(t, cfg.TokenStorage)
	require.NotNil(t, cfg.PrivateKeyDecryptor)

	// Missing clientID is rejected.
	_, err = JwtConfigFromConfigJSONString(fmt.Sprintf(jwtConfigJSONTemplate, ""), nil, nil)
	require.Error(t, err)

	// Malformed JSON is rejected.
	_, err = JwtConfigFromConfigJSONString("{not json", nil, nil)
	require.Error(t, err)
}

func TestJwtConfigFromConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "jwt.json")
	require.NoError(t, os.WriteFile(path, []byte(fmt.Sprintf(jwtConfigJSONTemplate, "file-client")), 0o600))

	cfg, err := JwtConfigFromConfigFile(path, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "file-client", cfg.ClientID)

	// A missing file surfaces an error.
	_, err = JwtConfigFromConfigFile(filepath.Join(dir, "missing.json"), nil, nil)
	require.Error(t, err)
}

func TestDeveloperTokenRevokeAndDownscope(t *testing.T) {
	srv := newAuthServer(t)
	auth := NewBoxDeveloperTokenAuth("dev-token", DeveloperTokenConfig{ClientID: "c", ClientSecret: "s"})
	session := testSession(srv.URL)

	downscoped, err := auth.DownscopeToken(context.Background(), []string{"item_preview"}, "", "shared", session)
	require.NoError(t, err)
	require.NotNil(t, downscoped)
	form := srv.lastTokenForm(t)
	assert.Equal(t, schemas.GrantTypeTokenExchange, form.Get("grant_type"))
	assert.Equal(t, "dev-token", form.Get("subject_token"))
	assert.Equal(t, "shared", form.Get("box_shared_link"))

	require.NoError(t, auth.RevokeToken(context.Background(), session))
	assert.Equal(t, "dev-token", srv.lastRevokeForm(t).Get("token"))
	assert.Nil(t, auth.TokenStorage.Get())
}
