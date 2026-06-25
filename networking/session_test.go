package networking

import (
	"testing"

	"github.com/asalih/go-box-sdk/internal/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// noopInterceptor is a do-nothing Interceptor used to exercise wiring.
type noopInterceptor struct{}

func (noopInterceptor) BeforeRequest(o *FetchOptions) *FetchOptions  { return o }
func (noopInterceptor) AfterRequest(r *FetchResponse) *FetchResponse { return r }

func TestNewNetworkSessionDefaults(t *testing.T) {
	s := NewNetworkSession()
	require.NotNil(t, s.BaseURLs)
	assert.Equal(t, DefaultBaseURL, s.BaseURLs.BaseURL)
	require.NotNil(t, s.NetworkClient)
	require.NotNil(t, s.RetryStrategy)
	require.NotNil(t, s.DataSanitizer)
	require.NotNil(t, s.AdditionalHeaders)
	assert.Empty(t, s.AdditionalHeaders)
}

func TestWithAdditionalHeadersCopyOnWrite(t *testing.T) {
	base := NewNetworkSession()
	derived := base.WithAdditionalHeaders(map[string]string{"X-A": "1"})
	derived2 := derived.WithAdditionalHeaders(map[string]string{"X-B": "2"})

	assert.Equal(t, "1", derived.AdditionalHeaders["X-A"])
	assert.Equal(t, "1", derived2.AdditionalHeaders["X-A"])
	assert.Equal(t, "2", derived2.AdditionalHeaders["X-B"])

	// Earlier sessions are not mutated.
	assert.Empty(t, base.AdditionalHeaders)
	_, hasB := derived.AdditionalHeaders["X-B"]
	assert.False(t, hasB)
}

func TestWithCustomBaseURLs(t *testing.T) {
	base := NewNetworkSession()
	custom := &BaseURLs{BaseURL: "https://eu.api.box.com"}
	derived := base.WithCustomBaseURLs(custom)

	assert.Same(t, custom, derived.BaseURLs)
	assert.NotSame(t, custom, base.BaseURLs)
}

func TestWithInterceptorsAppends(t *testing.T) {
	base := NewNetworkSession().WithInterceptors(noopInterceptor{})
	derived := base.WithInterceptors(noopInterceptor{})

	assert.Len(t, base.Interceptors, 1)
	assert.Len(t, derived.Interceptors, 2)
}

func TestWithProxy(t *testing.T) {
	base := NewNetworkSession()
	proxy := &ProxyConfig{URL: "http://proxy.local:3128", Username: "u", Password: "p"}
	derived := base.WithProxy(proxy)

	assert.Same(t, proxy, derived.ProxyConfig)
	require.NotNil(t, derived.NetworkClient)
	assert.Nil(t, base.ProxyConfig)
}

func TestWithNetworkClient(t *testing.T) {
	base := NewNetworkSession()
	custom := NewBoxNetworkClient()
	derived := base.WithNetworkClient(custom)

	assert.Same(t, custom, derived.NetworkClient)
}

func TestWithRetryStrategy(t *testing.T) {
	base := NewNetworkSession()
	strategy := &BoxRetryStrategy{MaxAttempts: 9}
	derived := base.WithRetryStrategy(strategy)

	assert.Same(t, strategy, derived.RetryStrategy)
}

func TestWithDataSanitizer(t *testing.T) {
	base := NewNetworkSession()
	sanitizer := logging.NewDataSanitizer()
	derived := base.WithDataSanitizer(sanitizer)

	assert.Same(t, sanitizer, derived.DataSanitizer)
}

func TestWithTimeoutConfig(t *testing.T) {
	base := NewNetworkSession()
	derived := base.WithTimeoutConfig(&TimeoutConfig{TimeoutMs: 2500})

	require.NotNil(t, derived.TimeoutConfig)
	assert.EqualValues(t, 2500, derived.TimeoutConfig.TimeoutMs)
	assert.Nil(t, base.TimeoutConfig)
}
