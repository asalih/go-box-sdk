package networking

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	boxerrors "github.com/asalih/go-box-sdk/errors"
	"github.com/asalih/go-box-sdk/schemas"
	"github.com/asalih/go-box-sdk/serialization"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordedReq is a snapshot of an outgoing request for assertions.
type recordedReq struct {
	method  string
	url     string
	headers map[string]string
	body    string
}

type responder func(*http.Request) (*http.Response, error)

// mockTransport returns programmed responses and records requests.
type mockTransport struct {
	seq    []responder
	always responder
	reqs   []recordedReq
}

func (m *mockTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	rr := recordedReq{method: r.Method, url: r.URL.String(), headers: map[string]string{}}
	for k, v := range r.Header {
		if len(v) > 0 {
			rr.headers[k] = v[0]
		}
	}
	if r.Body != nil {
		b, _ := io.ReadAll(r.Body)
		rr.body = string(b)
	}
	idx := len(m.reqs)
	m.reqs = append(m.reqs, rr)
	if idx < len(m.seq) {
		return m.seq[idx](r)
	}
	if m.always != nil {
		return m.always(r)
	}
	return nil, fmt.Errorf("mockTransport: no response for request %d", idx)
}

func respond(status int, body string, headers map[string]string) responder {
	return func(r *http.Request) (*http.Response, error) {
		h := http.Header{}
		for k, v := range headers {
			h.Set(k, v)
		}
		return &http.Response{
			StatusCode: status,
			Header:     h,
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    r,
		}, nil
	}
}

func failWith(err error) responder {
	return func(_ *http.Request) (*http.Response, error) {
		return nil, err
	}
}

func newTestClient(rt http.RoundTripper) *BoxNetworkClient {
	return &BoxNetworkClient{HTTPClient: &http.Client{
		Transport: rt,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
}

// fastSession returns a session whose retry backoff is zero, so retry tests do
// not actually sleep.
func fastSession() *NetworkSession {
	s := NewNetworkSession()
	s.RetryStrategy = &BoxRetryStrategy{MaxAttempts: 5, RetryRandomizationFactor: 0.5, RetryBaseInterval: 0, MaxRetriesOnException: 2}
	return s
}

// mockAuth is a test Authentication that swaps its token on refresh.
type mockAuth struct {
	token      string
	refreshErr error
}

func (a *mockAuth) RetrieveToken(context.Context, *NetworkSession) (*schemas.AccessToken, error) {
	return &schemas.AccessToken{AccessToken: a.token}, nil
}

func (a *mockAuth) RefreshToken(context.Context, *NetworkSession) (*schemas.AccessToken, error) {
	if a.refreshErr != nil {
		return nil, a.refreshErr
	}
	a.token = "new_token321"
	return &schemas.AccessToken{AccessToken: a.token}, nil
}

func (a *mockAuth) RetrieveAuthorizationHeader(context.Context, *NetworkSession) (string, error) {
	return "Bearer " + a.token, nil
}

func (a *mockAuth) RevokeToken(context.Context, *NetworkSession) error { return nil }

func (a *mockAuth) DownscopeToken(context.Context, []string, string, string, *NetworkSession) (*schemas.AccessToken, error) {
	return nil, nil
}

func TestFetchJSONSuccess(t *testing.T) {
	rt := &mockTransport{seq: []responder{respond(200, `{"id":"123456"}`, nil)}}
	client := newTestClient(rt)

	resp, err := client.Fetch(context.Background(), &FetchOptions{
		Method:         http.MethodGet,
		URL:            "https://example.com",
		NetworkSession: NewNetworkSession(),
	})
	require.NoError(t, err)
	assert.Equal(t, 200, resp.Status)
	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "123456", data["id"])
}

func TestFetchBinarySuccess(t *testing.T) {
	rt := &mockTransport{seq: []responder{respond(200, "binary data", nil)}}
	client := newTestClient(rt)

	resp, err := client.Fetch(context.Background(), &FetchOptions{
		Method:         http.MethodGet,
		URL:            "https://example.com",
		ResponseFormat: ResponseFormatBinary,
		NetworkSession: NewNetworkSession(),
	})
	require.NoError(t, err)
	assert.Equal(t, 200, resp.Status)
	got, err := io.ReadAll(resp.Content)
	require.NoError(t, err)
	assert.Equal(t, "binary data", string(got))
	if closer, ok := resp.Content.(io.Closer); ok {
		_ = closer.Close()
	}
}

func TestPrepareHeadersPrecedence(t *testing.T) {
	rt := &mockTransport{seq: []responder{respond(200, `{}`, nil)}}
	client := newTestClient(rt)
	session := NewNetworkSession().WithAdditionalHeaders(map[string]string{"X-Additional-Header": "test"})

	_, err := client.Fetch(context.Background(), &FetchOptions{
		Method:         http.MethodGet,
		URL:            "https://example.com",
		Headers:        map[string]string{"X-Header": "test"},
		Auth:           &mockAuth{token: "token123"},
		NetworkSession: session,
	})
	require.NoError(t, err)
	require.Len(t, rt.reqs, 1)
	h := rt.reqs[0].headers
	assert.Equal(t, "Bearer token123", h["Authorization"])
	assert.Equal(t, "test", h["X-Header"])
	assert.Equal(t, "test", h["X-Additional-Header"])
	assert.Equal(t, userAgentHeader, h["User-Agent"])
	assert.Equal(t, xBoxUaHeader, h["X-Box-Ua"])
	// GET requests must not carry a Content-Type header.
	_, hasContentType := h["Content-Type"]
	assert.False(t, hasContentType)
}

func TestPrepareJSONBody(t *testing.T) {
	rt := &mockTransport{seq: []responder{respond(200, `{}`, nil)}}
	client := newTestClient(rt)

	_, err := client.Fetch(context.Background(), &FetchOptions{
		Method:         http.MethodPost,
		URL:            "https://example.com",
		Data:           map[string]any{"key": "value"},
		ContentType:    ContentTypeJSON,
		NetworkSession: NewNetworkSession(),
	})
	require.NoError(t, err)
	require.Len(t, rt.reqs, 1)
	assert.Equal(t, `{"key":"value"}`, rt.reqs[0].body)
	assert.Equal(t, ContentTypeJSON, rt.reqs[0].headers["Content-Type"])
}

func TestDefaultMaxAttemptsOn500(t *testing.T) {
	rt := &mockTransport{always: respond(500, "", map[string]string{"Retry-After": "0"})}
	client := newTestClient(rt)

	_, err := client.Fetch(context.Background(), &FetchOptions{
		Method:         http.MethodGet,
		URL:            "https://example.com",
		NetworkSession: NewNetworkSession(),
	})
	var apiErr *boxerrors.BoxAPIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 500, apiErr.ResponseInfo.StatusCode)
	assert.Len(t, rt.reqs, 5)
}

func TestCustomMaxAttempts(t *testing.T) {
	rt := &mockTransport{always: respond(500, "", map[string]string{"Retry-After": "0"})}
	client := newTestClient(rt)
	session := NewNetworkSession()
	session.RetryStrategy = &BoxRetryStrategy{MaxAttempts: 3, RetryRandomizationFactor: 0.5, RetryBaseInterval: 0, MaxRetriesOnException: 2}

	_, err := client.Fetch(context.Background(), &FetchOptions{
		Method:         http.MethodGet,
		URL:            "https://example.com",
		NetworkSession: session,
	})
	require.Error(t, err)
	assert.Len(t, rt.reqs, 3)
}

func TestRetryableStatusCodesThenSuccess(t *testing.T) {
	for _, status := range []int{429, 500, 503} {
		t.Run(fmt.Sprintf("status_%d", status), func(t *testing.T) {
			rt := &mockTransport{seq: []responder{
				respond(status, "", map[string]string{"Retry-After": "0"}),
				respond(status, "", map[string]string{"Retry-After": "0"}),
				respond(200, `{"id":"123456"}`, nil),
			}}
			client := newTestClient(rt)

			resp, err := client.Fetch(context.Background(), &FetchOptions{
				Method:         http.MethodGet,
				URL:            "https://example.com",
				NetworkSession: fastSession(),
			})
			require.NoError(t, err)
			assert.Equal(t, 200, resp.Status)
			assert.Len(t, rt.reqs, 3)
		})
	}
}

func TestStatus202WithoutRetryAfterReturned(t *testing.T) {
	rt := &mockTransport{always: respond(202, "", map[string]string{"Content-Type": "text/html"})}
	client := newTestClient(rt)

	resp, err := client.Fetch(context.Background(), &FetchOptions{
		Method:         http.MethodGet,
		URL:            "https://example.com",
		NetworkSession: NewNetworkSession(),
	})
	require.NoError(t, err)
	assert.Equal(t, 202, resp.Status)
	assert.Len(t, rt.reqs, 1)
}

func TestStatus202WithRetryAfterRetries(t *testing.T) {
	rt := &mockTransport{seq: []responder{
		respond(202, "", map[string]string{"Retry-After": "0"}),
		respond(202, "", map[string]string{"Retry-After": "0"}),
		respond(200, `{"id":"123456"}`, map[string]string{"Retry-After": "0"}),
	}}
	client := newTestClient(rt)

	resp, err := client.Fetch(context.Background(), &FetchOptions{
		Method:         http.MethodGet,
		URL:            "https://example.com",
		NetworkSession: fastSession(),
	})
	require.NoError(t, err)
	assert.Equal(t, 200, resp.Status)
	assert.Len(t, rt.reqs, 3)
}

func TestNotRetryableStatusCodes(t *testing.T) {
	for _, status := range []int{400, 403, 404} {
		t.Run(fmt.Sprintf("status_%d", status), func(t *testing.T) {
			rt := &mockTransport{always: respond(status, "", nil)}
			client := newTestClient(rt)

			_, err := client.Fetch(context.Background(), &FetchOptions{
				Method:         http.MethodGet,
				URL:            "https://example.com",
				NetworkSession: NewNetworkSession(),
			})
			var apiErr *boxerrors.BoxAPIError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, status, apiErr.ResponseInfo.StatusCode)
			assert.Len(t, rt.reqs, 1)
		})
	}
}

func TestReauth401ThenRetryWithNewToken(t *testing.T) {
	rt := &mockTransport{seq: []responder{
		respond(401, "", nil),
		respond(200, `{"id":"123456"}`, nil),
	}}
	client := newTestClient(rt)

	resp, err := client.Fetch(context.Background(), &FetchOptions{
		Method:         http.MethodGet,
		URL:            "https://example.com",
		Auth:           &mockAuth{token: "token123"},
		NetworkSession: fastSession(),
	})
	require.NoError(t, err)
	assert.Equal(t, 200, resp.Status)
	require.Len(t, rt.reqs, 2)
	assert.Equal(t, "Bearer token123", rt.reqs[0].headers["Authorization"])
	assert.Equal(t, "Bearer new_token321", rt.reqs[1].headers["Authorization"])
}

func TestNoRetry401WithoutAuth(t *testing.T) {
	rt := &mockTransport{always: respond(401, "", nil)}
	client := newTestClient(rt)

	_, err := client.Fetch(context.Background(), &FetchOptions{
		Method:         http.MethodGet,
		URL:            "https://example.com",
		NetworkSession: NewNetworkSession(),
	})
	var apiErr *boxerrors.BoxAPIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 401, apiErr.ResponseInfo.StatusCode)
	assert.Len(t, rt.reqs, 1)
}

func TestNetworkExceptionRetryThenSuccess(t *testing.T) {
	rt := &mockTransport{seq: []responder{
		failWith(fmt.Errorf("Connection cancelled")),
		respond(200, `{"id":"123456"}`, nil),
	}}
	client := newTestClient(rt)

	resp, err := client.Fetch(context.Background(), &FetchOptions{
		Method:         http.MethodGet,
		URL:            "https://example.com",
		NetworkSession: fastSession(),
	})
	require.NoError(t, err)
	assert.Equal(t, 200, resp.Status)
	assert.Len(t, rt.reqs, 2)
}

func TestNetworkExceptionMaxRetries(t *testing.T) {
	rt := &mockTransport{always: failWith(fmt.Errorf("Connection cancelled"))}
	client := newTestClient(rt)

	_, err := client.Fetch(context.Background(), &FetchOptions{
		Method:         http.MethodGet,
		URL:            "https://example.com",
		NetworkSession: fastSession(),
	})
	var sdkErr *boxerrors.BoxSDKError
	require.ErrorAs(t, err, &sdkErr)
	assert.Contains(t, sdkErr.Message, "Connection cancelled")
	assert.Len(t, rt.reqs, 3)
}

func TestFollowRedirectCrossOriginStripsAuth(t *testing.T) {
	rt := &mockTransport{seq: []responder{
		respond(302, "", map[string]string{"Location": "https://other.example.org/redirected"}),
		respond(200, `{"id":"123456"}`, nil),
	}}
	client := newTestClient(rt)

	resp, err := client.Fetch(context.Background(), &FetchOptions{
		Method:         http.MethodGet,
		URL:            "https://api.box.com/resource",
		Auth:           &mockAuth{token: "token123"},
		NetworkSession: NewNetworkSession(),
	})
	require.NoError(t, err)
	assert.Equal(t, 200, resp.Status)
	require.Len(t, rt.reqs, 2)
	assert.Equal(t, "Bearer token123", rt.reqs[0].headers["Authorization"])
	_, hasAuth := rt.reqs[1].headers["Authorization"]
	assert.False(t, hasAuth, "auth header must be stripped on cross-origin redirect")
}

func TestDisableFollowRedirects(t *testing.T) {
	rt := &mockTransport{seq: []responder{
		respond(302, "", map[string]string{"Location": "https://example.com/redirected"}),
	}}
	client := newTestClient(rt)
	follow := false

	resp, err := client.Fetch(context.Background(), &FetchOptions{
		Method:          http.MethodGet,
		URL:             "https://example.com",
		FollowRedirects: &follow,
		NetworkSession:  NewNetworkSession(),
	})
	require.NoError(t, err)
	assert.Equal(t, 302, resp.Status)
	assert.Len(t, rt.reqs, 1)
}

func TestErrorMappingValidJSONBody(t *testing.T) {
	body := `{
      "type": "error",
      "code": "item_name_invalid",
      "context_info": {"message": "Something went wrong."},
      "help_url": "https://developer.box.com/help",
      "message": "Method Not Allowed",
      "request_id": "abcdef123456",
      "status": 400
    }`
	rt := &mockTransport{always: respond(400, body, nil)}
	client := newTestClient(rt)

	_, err := client.Fetch(context.Background(), &FetchOptions{
		Method:         http.MethodPost,
		URL:            "https://example.com",
		Data:           map[string]any{"key": "value"},
		NetworkSession: NewNetworkSession(),
	})
	var apiErr *boxerrors.BoxAPIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 400, apiErr.ResponseInfo.StatusCode)
	assert.Equal(t, "item_name_invalid", apiErr.ResponseInfo.Code)
	assert.Equal(t, "abcdef123456", apiErr.ResponseInfo.RequestID)
	assert.Equal(t, "https://developer.box.com/help", apiErr.ResponseInfo.HelpURL)
	// Node-faithful message includes the code between status and message.
	assert.Equal(t, "400 item_name_invalid Method Not Allowed; Request ID: abcdef123456", apiErr.Message)
}

func TestErrorMappingInvalidJSONBody(t *testing.T) {
	for _, body := range []string{"", "Invalid json"} {
		t.Run(body, func(t *testing.T) {
			rt := &mockTransport{always: respond(400, body, nil)}
			client := newTestClient(rt)

			_, err := client.Fetch(context.Background(), &FetchOptions{
				Method:         http.MethodPost,
				URL:            "https://example.com",
				Data:           map[string]any{"key": "value"},
				NetworkSession: NewNetworkSession(),
			})
			var apiErr *boxerrors.BoxAPIError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, 400, apiErr.ResponseInfo.StatusCode)
			assert.Equal(t, "", apiErr.ResponseInfo.Code)
			assert.Equal(t, body, apiErr.ResponseInfo.RawBody)
		})
	}
}

func TestSensitiveDataSanitized(t *testing.T) {
	body := `{
      "client_secret": "secret",
      "password": "change-me",
      "message": "Method Not Allowed",
      "request_id": "abcdef123456",
      "status": 400
    }`
	rt := &mockTransport{always: respond(400, body, map[string]string{"token": "my_token"})}
	client := newTestClient(rt)

	_, err := client.Fetch(context.Background(), &FetchOptions{
		Method:         http.MethodPost,
		URL:            "https://example.com",
		Headers:        map[string]string{"Authorization": "Bearer acbdef123456"},
		Data:           map[string]any{"key": "value"},
		NetworkSession: NewNetworkSession(),
	})
	var apiErr *boxerrors.BoxAPIError
	require.ErrorAs(t, err, &apiErr)
	rendered := apiErr.JSONString()
	assert.Contains(t, rendered, serialization.SanitizedValue())
	assert.Contains(t, rendered, "Method Not Allowed")
	// The sensitive values themselves must not appear.
	assert.NotContains(t, rendered, "Bearer acbdef123456")
	assert.NotContains(t, rendered, "change-me")
	assert.NotContains(t, rendered, "my_token")
}

func TestRetryAfterUsesHeader(t *testing.T) {
	strategy := NewBoxRetryStrategy()
	resp := &FetchResponse{Status: 429, Headers: map[string]string{"Retry-After": "213"}}
	for attempt := 1; attempt < 5; attempt++ {
		assert.Equal(t, 213.0, strategy.RetryAfter(&FetchOptions{}, resp, attempt))
	}
}

func TestRetryAfterExponentialBackoff(t *testing.T) {
	strategy := NewBoxRetryStrategy()
	resp := &FetchResponse{Status: 500, Headers: map[string]string{}}
	for attempt := 1; attempt < 5; attempt++ {
		assert.Greater(t, strategy.RetryAfter(&FetchOptions{}, resp, attempt), 0.0)
	}
}
