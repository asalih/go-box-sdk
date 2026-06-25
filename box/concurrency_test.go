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
	"sync"
	"sync/atomic"
	"testing"

	"github.com/asalih/go-box-sdk/schemas"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingTokenServer is an httptest server that serves the /oauth2/token
// endpoint, counts how many times it is hit, and hands out a fresh token on
// each call. The counter is safe for concurrent access.
func countingTokenServer(t *testing.T) (*httptest.Server, *int32) {
	t.Helper()
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/oauth2/token", r.URL.Path)
		n := atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"access_token":"token_%d","expires_in":3600,"token_type":"bearer"}`, n)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

// runConcurrently fans out fn across n goroutines released simultaneously from
// a barrier, so the test maximizes contention on the auth's refresh path.
func runConcurrently(t *testing.T, n int, fn func() error) {
	t.Helper()
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			errs[idx] = fn()
		}(i)
	}
	close(start)
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}
}

const concurrentCallers = 50

func TestCcgConcurrentColdCacheSingleFetch(t *testing.T) {
	srv, calls := countingTokenServer(t)
	auth := NewBoxCcgAuth(CcgConfig{ClientID: "c", ClientSecret: "s", EnterpriseID: "ent"})
	session := testSession(srv.URL)

	runConcurrently(t, concurrentCallers, func() error {
		_, err := auth.RetrieveAuthorizationHeader(context.Background(), session)
		return err
	})

	// All callers must collapse onto a single token request.
	assert.Equal(t, int32(1), atomic.LoadInt32(calls))
	require.NotNil(t, auth.TokenStorage.Get())
}

func TestCcgConcurrentForcedRefreshSingleFetch(t *testing.T) {
	srv, calls := countingTokenServer(t)
	auth := NewBoxCcgAuth(CcgConfig{ClientID: "c", ClientSecret: "s", EnterpriseID: "ent"})
	session := testSession(srv.URL)

	// Prime the cache with one token.
	_, err := auth.RetrieveToken(context.Background(), session)
	require.NoError(t, err)
	require.Equal(t, int32(1), atomic.LoadInt32(calls))

	// Concurrent forced refreshes must collapse into exactly one extra fetch.
	runConcurrently(t, concurrentCallers, func() error {
		_, err := auth.RefreshToken(context.Background(), session)
		return err
	})
	assert.Equal(t, int32(2), atomic.LoadInt32(calls))
}

func TestJwtConcurrentColdCacheSingleFetch(t *testing.T) {
	srv, calls := countingTokenServer(t)
	auth := NewBoxJwtAuth(JwtConfig{
		ClientID:     "jwt-client",
		ClientSecret: "jwt-secret",
		JwtKeyID:     "key-1",
		PrivateKey:   testRSAPrivateKeyPEM(t),
		EnterpriseID: "ent-1",
	})
	session := testSession(srv.URL)

	runConcurrently(t, concurrentCallers, func() error {
		_, err := auth.RetrieveAuthorizationHeader(context.Background(), session)
		return err
	})

	assert.Equal(t, int32(1), atomic.LoadInt32(calls))
}

// TestOAuthConcurrentRefreshRotatesTokenSafely models Box's single-use refresh
// tokens: the server invalidates a refresh token the first time it is used and
// rejects any reuse. Without serialized refresh, concurrent callers would race
// to consume the same refresh token and all but one would fail invalid_grant.
func TestOAuthConcurrentRefreshRotatesTokenSafely(t *testing.T) {
	var mu sync.Mutex
	valid := map[string]bool{"rt0": true}
	var refreshCalls int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		require.Equal(t, schemas.GrantTypeRefreshToken, r.PostForm.Get("grant_type"))
		presented := r.PostForm.Get("refresh_token")

		mu.Lock()
		ok := valid[presented]
		if ok {
			delete(valid, presented) // single-use: consume it
		}
		mu.Unlock()

		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"refresh token has expired"}`))
			return
		}

		n := atomic.AddInt32(&refreshCalls, 1)
		newRefresh := fmt.Sprintf("rt%d", n)
		mu.Lock()
		valid[newRefresh] = true
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"access_token":"at%d","refresh_token":%q,"expires_in":3600,"token_type":"bearer"}`, n, newRefresh)
	}))
	t.Cleanup(srv.Close)

	auth := NewBoxOAuth(OAuthConfig{ClientID: "c", ClientSecret: "s"})
	auth.TokenStorage.Store(&schemas.AccessToken{AccessToken: "at0", RefreshToken: "rt0"})
	session := testSession(srv.URL)

	runConcurrently(t, concurrentCallers, func() error {
		_, err := auth.RefreshToken(context.Background(), session)
		return err
	})

	// The single-use refresh token must have been consumed exactly once.
	assert.Equal(t, int32(1), atomic.LoadInt32(&refreshCalls))
	final := auth.TokenStorage.Get()
	require.NotNil(t, final)
	assert.Equal(t, "at1", final.AccessToken)
}

// testRSAPrivateKeyPEM returns a fresh PKCS#8 RSA private key in PEM form.
func testRSAPrivateKeyPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}
