package box

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
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
