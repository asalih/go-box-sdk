package networking

import "github.com/asalih/go-box-sdk/internal/logging"

// NetworkSession holds the configuration applied to every request: extra
// headers, base URLs, interceptors, proxy, the network client, retry strategy,
// data sanitizer, and timeout. It mirrors NetworkSession in
// src/networking/network.ts. The With* helpers return a shallow copy with one
// field changed, preserving the immutable-builder style of the source.
type NetworkSession struct {
	AdditionalHeaders map[string]string
	BaseURLs          *BaseURLs
	Interceptors      []Interceptor
	ProxyConfig       *ProxyConfig
	NetworkClient     NetworkClient
	RetryStrategy     RetryStrategy
	DataSanitizer     *logging.DataSanitizer
	TimeoutConfig     *TimeoutConfig
}

// NewNetworkSession returns a NetworkSession populated with the SDK defaults.
func NewNetworkSession() *NetworkSession {
	return &NetworkSession{
		AdditionalHeaders: map[string]string{},
		BaseURLs:          NewBaseURLs(),
		Interceptors:      nil,
		NetworkClient:     NewBoxNetworkClient(),
		RetryStrategy:     NewBoxRetryStrategy(),
		DataSanitizer:     logging.NewDataSanitizer(),
	}
}

// clone returns a shallow copy of the session.
func (s *NetworkSession) clone() *NetworkSession {
	cp := *s
	return &cp
}

// WithAdditionalHeaders returns a copy with the given headers merged in.
func (s *NetworkSession) WithAdditionalHeaders(headers map[string]string) *NetworkSession {
	merged := make(map[string]string, len(s.AdditionalHeaders)+len(headers))
	for k, v := range s.AdditionalHeaders {
		merged[k] = v
	}
	for k, v := range headers {
		merged[k] = v
	}
	out := s.clone()
	out.AdditionalHeaders = merged
	return out
}

// WithCustomBaseURLs returns a copy that uses the provided base URLs.
func (s *NetworkSession) WithCustomBaseURLs(baseURLs *BaseURLs) *NetworkSession {
	out := s.clone()
	out.BaseURLs = baseURLs
	return out
}

// WithInterceptors returns a copy with the given interceptors appended.
func (s *NetworkSession) WithInterceptors(interceptors ...Interceptor) *NetworkSession {
	out := s.clone()
	out.Interceptors = append(append([]Interceptor{}, s.Interceptors...), interceptors...)
	return out
}

// WithProxy returns a copy that routes requests through the given proxy.
func (s *NetworkSession) WithProxy(proxy *ProxyConfig) *NetworkSession {
	out := s.clone()
	out.ProxyConfig = proxy
	out.NetworkClient = NewBoxNetworkClientWithProxy(proxy)
	return out
}

// WithNetworkClient returns a copy that uses the provided network client.
func (s *NetworkSession) WithNetworkClient(client NetworkClient) *NetworkSession {
	out := s.clone()
	out.NetworkClient = client
	return out
}

// WithRetryStrategy returns a copy that uses the provided retry strategy.
func (s *NetworkSession) WithRetryStrategy(retryStrategy RetryStrategy) *NetworkSession {
	out := s.clone()
	out.RetryStrategy = retryStrategy
	return out
}

// WithDataSanitizer returns a copy that uses a fresh data sanitizer.
func (s *NetworkSession) WithDataSanitizer(sanitizer *logging.DataSanitizer) *NetworkSession {
	out := s.clone()
	out.DataSanitizer = sanitizer
	return out
}

// WithTimeoutConfig returns a copy that applies the given timeout configuration.
func (s *NetworkSession) WithTimeoutConfig(timeoutConfig *TimeoutConfig) *NetworkSession {
	out := s.clone()
	out.TimeoutConfig = timeoutConfig
	return out
}
