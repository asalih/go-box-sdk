package box

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/asalih/go-box-sdk/networking"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingInterceptor captures whether it ran, to verify interceptor wiring.
type recordingInterceptor struct{ before, after bool }

func (i *recordingInterceptor) BeforeRequest(o *networking.FetchOptions) *networking.FetchOptions {
	i.before = true
	return o
}

func (i *recordingInterceptor) AfterRequest(r *networking.FetchResponse) *networking.FetchResponse {
	i.after = true
	return r
}

func newDevClient(t *testing.T, serverURL string) *BoxClient {
	t.Helper()
	auth := NewBoxDeveloperTokenAuth("dev-token", DeveloperTokenConfig{})
	return NewBoxClient(auth, testSession(serverURL))
}

func TestNewBoxClientDefaultsSession(t *testing.T) {
	client := NewBoxClient(NewBoxDeveloperTokenAuth("t", DeveloperTokenConfig{}), nil)
	require.NotNil(t, client.NetworkSession)
	// All managers share the client's session instance.
	assert.Same(t, client.NetworkSession, client.Files.NetworkSession)
	assert.Same(t, client.NetworkSession, client.Folders.NetworkSession)
	assert.Same(t, client.NetworkSession, client.Uploads.NetworkSession)
	assert.Same(t, client.NetworkSession, client.ChunkedUploads.NetworkSession)
	assert.Same(t, client.NetworkSession, client.Downloads.NetworkSession)
}

// headerEcho records the request headers it receives and returns a minimal file.
func headerEcho(t *testing.T, captured *http.Header) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*captured = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"1","type":"file","name":"f"}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestClientWithAsUserHeader(t *testing.T) {
	var got http.Header
	srv := headerEcho(t, &got)
	client := newDevClient(t, srv.URL).WithAsUserHeader("user-42")

	_, err := client.Files.GetFileByID(context.Background(), "1", nil)
	require.NoError(t, err)
	assert.Equal(t, "user-42", got.Get("As-User"))
}

func TestClientWithSuppressedNotifications(t *testing.T) {
	var got http.Header
	srv := headerEcho(t, &got)
	client := newDevClient(t, srv.URL).WithSuppressedNotifications()

	_, err := client.Files.GetFileByID(context.Background(), "1", nil)
	require.NoError(t, err)
	assert.Equal(t, "off", got.Get("Box-Notifications"))
}

func TestClientWithExtraHeaders(t *testing.T) {
	var got http.Header
	srv := headerEcho(t, &got)
	client := newDevClient(t, srv.URL).WithExtraHeaders(map[string]string{"X-Custom": "abc"})

	_, err := client.Files.GetFileByID(context.Background(), "1", nil)
	require.NoError(t, err)
	assert.Equal(t, "abc", got.Get("X-Custom"))
}

func TestClientWithCustomBaseURLs(t *testing.T) {
	client := newDevClient(t, "https://api.box.com")
	custom := &networking.BaseURLs{BaseURL: "https://eu.api.box.com", UploadURL: "https://eu.upload.box.com", OAuth2URL: "https://eu.account.box.com"}

	derived := client.WithCustomBaseURLs(custom)
	assert.Same(t, custom, derived.NetworkSession.BaseURLs)
	// The original client is unaffected (copy-on-write session).
	assert.NotSame(t, custom, client.NetworkSession.BaseURLs)
}

func TestClientWithTimeouts(t *testing.T) {
	client := newDevClient(t, "https://api.box.com")
	derived := client.WithTimeouts(&networking.TimeoutConfig{TimeoutMs: 1234})

	require.NotNil(t, derived.NetworkSession.TimeoutConfig)
	assert.EqualValues(t, 1234, derived.NetworkSession.TimeoutConfig.TimeoutMs)
	assert.Nil(t, client.NetworkSession.TimeoutConfig)
}

func TestClientWithProxy(t *testing.T) {
	client := newDevClient(t, "https://api.box.com")
	proxy := &networking.ProxyConfig{URL: "http://proxy.local:8080"}
	derived := client.WithProxy(proxy)

	assert.Same(t, proxy, derived.NetworkSession.ProxyConfig)
	require.NotNil(t, derived.NetworkSession.NetworkClient)
	assert.Nil(t, client.NetworkSession.ProxyConfig)
}

func TestClientWithInterceptors(t *testing.T) {
	var got http.Header
	srv := headerEcho(t, &got)
	interceptor := &recordingInterceptor{}
	client := newDevClient(t, srv.URL).WithInterceptors(interceptor)

	require.Len(t, client.NetworkSession.Interceptors, 1)

	_, err := client.Files.GetFileByID(context.Background(), "1", nil)
	require.NoError(t, err)
	assert.True(t, interceptor.before, "BeforeRequest should run")
	assert.True(t, interceptor.after, "AfterRequest should run")
}

func TestClientConfigHelpersAreIndependent(t *testing.T) {
	base := newDevClient(t, "https://api.box.com")
	derived := base.WithExtraHeaders(map[string]string{"X-A": "1"})

	// Deriving must not mutate the base session's headers.
	assert.Empty(t, base.NetworkSession.AdditionalHeaders)
	assert.Equal(t, "1", derived.NetworkSession.AdditionalHeaders["X-A"])
}
