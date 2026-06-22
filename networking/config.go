package networking

// ProxyConfig configures an outbound HTTP proxy. It mirrors
// src/networking/proxyConfig.ts.
type ProxyConfig struct {
	URL      string
	Username string
	Password string
}

// TimeoutConfig configures a per-request timeout. It mirrors
// src/networking/timeoutConfig.ts.
type TimeoutConfig struct {
	TimeoutMs int64
}

// Interceptor can modify requests before they are sent and responses after
// they are received. It mirrors src/networking/interceptors.ts.
type Interceptor interface {
	BeforeRequest(options *FetchOptions) *FetchOptions
	AfterRequest(response *FetchResponse) *FetchResponse
}
