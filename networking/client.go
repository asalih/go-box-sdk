package networking

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"runtime"
	"sort"
	"strings"
	"time"

	boxerrors "github.com/asalih/go-box-sdk/errors"
	"github.com/asalih/go-box-sdk/internal/logging"
	"github.com/asalih/go-box-sdk/internal/utils"
	"github.com/asalih/go-box-sdk/serialization"
)

// sdkVersion mirrors src/networking/version.ts.
const sdkVersion = "10.12.0"

// userAgentHeader and xBoxUaHeader mirror the values constructed in
// src/networking/boxNetworkClient.ts, adapted for the Go runtime.
var (
	userAgentHeader = fmt.Sprintf("Box Go generated SDK v%s (Go %s)", sdkVersion, runtime.Version())
	xBoxUaHeader    = fmt.Sprintf("agent=box-go-generated-sdk/%s; env=Go/%s", sdkVersion, strings.TrimPrefix(runtime.Version(), "go"))
)

// NetworkClient performs an HTTP request and returns the response. It mirrors
// the NetworkClient interface in src/networking/networkClient.ts.
type NetworkClient interface {
	Fetch(ctx context.Context, options *FetchOptions) (*FetchResponse, error)
}

// BoxNetworkClient is the default NetworkClient. It mirrors BoxNetworkClient in
// src/networking/boxNetworkClient.ts: it builds the request, applies
// interceptors, manages timeouts, follows redirects manually (stripping the
// auth header and query params on cross-origin redirects), retries per the
// configured strategy, and maps error responses to BoxAPIError.
type BoxNetworkClient struct {
	HTTPClient *http.Client
}

// NewBoxNetworkClient returns a BoxNetworkClient with a default HTTP client.
func NewBoxNetworkClient() *BoxNetworkClient {
	return &BoxNetworkClient{HTTPClient: newHTTPClient(nil)}
}

// NewBoxNetworkClientWithProxy returns a BoxNetworkClient that routes requests
// through the given proxy.
func NewBoxNetworkClientWithProxy(proxy *ProxyConfig) *BoxNetworkClient {
	return &BoxNetworkClient{HTTPClient: newHTTPClient(proxy)}
}

// newHTTPClient builds an HTTP client that does not auto-follow redirects, so
// that the manual redirect handling in do can strip credentials cross-origin.
func newHTTPClient(proxy *ProxyConfig) *http.Client {
	transport, _ := http.DefaultTransport.(*http.Transport)
	transport = transport.Clone()
	if proxy != nil && proxy.URL != "" {
		if u, err := url.Parse(proxy.URL); err == nil {
			if proxy.Username != "" {
				u.User = url.UserPassword(proxy.Username, proxy.Password)
			}
			transport.Proxy = http.ProxyURL(u)
		}
	}
	return &http.Client{
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// preparedRequest holds the request body and content metadata, computed once so
// it can be re-sent across retries and redirects.
type preparedRequest struct {
	body        []byte
	hasBody     bool
	textualBody bool
	contentType string
	contentMD5  string
}

// Fetch applies request interceptors, prepares the body, and dispatches the
// request through the retry/redirect loop.
func (c *BoxNetworkClient) Fetch(ctx context.Context, options *FetchOptions) (*FetchResponse, error) {
	opts := options
	if options.NetworkSession != nil {
		for _, interceptor := range options.NetworkSession.Interceptors {
			opts = interceptor.BeforeRequest(opts)
		}
	}
	prepared, err := prepareRequest(opts)
	if err != nil {
		return nil, err
	}
	return c.do(ctx, opts, prepared, 1, 0)
}

// prepareRequest builds the request body and headers for the given options.
func prepareRequest(options *FetchOptions) (*preparedRequest, error) {
	if len(options.MultipartData) > 0 {
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		var contentMD5 string
		for _, item := range options.MultipartData {
			switch {
			case item.FileStream != nil:
				data, err := io.ReadAll(item.FileStream)
				if err != nil {
					return nil, fmt.Errorf("networking: failed to read multipart file stream: %w", err)
				}
				contentMD5 = utils.SHA1Hex(data)
				fileName := item.FileName
				if fileName == "" {
					fileName = "file"
				}
				partContentType := item.ContentType
				if partContentType == "" {
					partContentType = ContentTypeOctetStream
				}
				header := textproto.MIMEHeader{}
				header.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, item.PartName, fileName))
				header.Set("Content-Type", partContentType)
				part, err := writer.CreatePart(header)
				if err != nil {
					return nil, fmt.Errorf("networking: failed to create multipart part: %w", err)
				}
				if _, err := part.Write(data); err != nil {
					return nil, fmt.Errorf("networking: failed to write multipart part: %w", err)
				}
			case item.Data != nil:
				text, err := serialization.SDToJSON(item.Data)
				if err != nil {
					return nil, err
				}
				if err := writer.WriteField(item.PartName, text); err != nil {
					return nil, fmt.Errorf("networking: failed to write multipart field: %w", err)
				}
			default:
				return nil, boxerrors.NewBoxSDKError("Multipart item must have either body or fileStream")
			}
		}
		if err := writer.Close(); err != nil {
			return nil, fmt.Errorf("networking: failed to finalize multipart body: %w", err)
		}
		return &preparedRequest{
			body:        buf.Bytes(),
			hasBody:     true,
			contentType: writer.FormDataContentType(),
			contentMD5:  contentMD5,
		}, nil
	}

	contentType := options.contentTypeOrDefault()
	switch contentType {
	case ContentTypeJSON, ContentTypeJSONPatch:
		if options.Data == nil {
			return &preparedRequest{contentType: contentType}, nil
		}
		text, err := serialization.SDToJSON(options.Data)
		if err != nil {
			return nil, err
		}
		return &preparedRequest{body: []byte(text), hasBody: true, textualBody: true, contentType: contentType}, nil
	case ContentTypeURLEncoded:
		text, err := serialization.SDToURLParams(options.Data)
		if err != nil {
			return nil, err
		}
		return &preparedRequest{body: []byte(text), hasBody: true, textualBody: true, contentType: contentType}, nil
	case ContentTypeOctetStream:
		if options.FileStream == nil {
			return nil, boxerrors.NewBoxSDKError("fileStream required for application/octet-stream content type")
		}
		data, err := io.ReadAll(options.FileStream)
		if err != nil {
			return nil, fmt.Errorf("networking: failed to read file stream: %w", err)
		}
		return &preparedRequest{body: data, hasBody: true, contentType: contentType}, nil
	default:
		return nil, boxerrors.NewBoxSDKError(fmt.Sprintf("Unsupported content type : %s", contentType))
	}
}

// do performs a single attempt and then recurses for retries and redirects. It
// is the Go equivalent of the recursive fetch in the source network client.
func (c *BoxNetworkClient) do(ctx context.Context, options *FetchOptions, prepared *preparedRequest, attemptNumber, numRetriesOnException int) (*FetchResponse, error) {
	session := options.NetworkSession
	retryStrategy := RetryStrategy(NewBoxRetryStrategy())
	sanitizer := logging.NewDataSanitizer()
	var interceptors []Interceptor
	var timeoutMs int64
	if session != nil {
		if session.RetryStrategy != nil {
			retryStrategy = session.RetryStrategy
		}
		if session.DataSanitizer != nil {
			sanitizer = session.DataSanitizer
		}
		interceptors = session.Interceptors
		if session.TimeoutConfig != nil {
			timeoutMs = session.TimeoutConfig.TimeoutMs
		}
	}

	method := options.Method
	if method == "" {
		method = http.MethodGet
	}

	headers := buildHeaders(ctx, options, prepared, method, session)
	authHeaderErr := headers.authErr
	if authHeaderErr != nil {
		return nil, authHeaderErr
	}

	requestURL := buildURL(options.URL, options.Params)

	var bodyReader io.Reader
	if prepared.hasBody {
		bodyReader = bytes.NewReader(prepared.body)
	}

	reqCtx := ctx
	cancel := context.CancelFunc(func() {})
	if timeoutMs > 0 {
		reqCtx, cancel = context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	}

	req, err := http.NewRequestWithContext(reqCtx, method, requestURL, bodyReader)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("networking: failed to build request: %w", err)
	}
	for key, value := range headers.values {
		req.Header.Set(key, value)
	}

	format := options.responseFormatOrDefault()
	ignoreBody := !options.shouldFollowRedirects()

	isException := false
	var caughtErr error
	var fetchResponse *FetchResponse
	var responseBytes []byte
	bodyOpen := false

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		isException = true
		numRetriesOnException++
		if timeoutMs > 0 && errors.Is(err, context.DeadlineExceeded) {
			//nolint:staticcheck // ST1005: mirror upstream SDK error text verbatim
			caughtErr = fmt.Errorf("Connection timeout after %dms", timeoutMs)
		} else {
			caughtErr = err
		}
		cancel()
		fetchResponse = &FetchResponse{Status: 0, Headers: map[string]string{}}
	} else {
		headersMap := flattenHeaders(resp.Header)
		var content io.Reader = bytes.NewReader(nil)
		var data serialization.SerializedData
		switch {
		case ignoreBody:
			_ = resp.Body.Close()
			cancel()
		case format == ResponseFormatBinary:
			content = resp.Body
			bodyOpen = true
		default:
			buf, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			cancel()
			if readErr != nil {
				return nil, fmt.Errorf("networking: failed to read response body: %w", readErr)
			}
			responseBytes = buf
			if format == ResponseFormatJSON && len(buf) > 0 {
				if parsed, parseErr := serialization.JSONToSerializedData(string(buf)); parseErr == nil {
					data = parsed
				}
			}
			content = bytes.NewReader(buf)
		}
		fetchResponse = &FetchResponse{
			URL:          resp.Request.URL.String(),
			Status:       resp.StatusCode,
			Data:         data,
			Content:      content,
			ContentBytes: responseBytes,
			Headers:      headersMap,
		}
		for _, interceptor := range interceptors {
			fetchResponse = interceptor.AfterRequest(fetchResponse)
		}
	}

	releaseBody := func() {
		if bodyOpen && resp != nil {
			_ = resp.Body.Close()
			bodyOpen = false
		}
		cancel()
	}

	attemptForRetry := attemptNumber
	if isException {
		attemptForRetry = numRetriesOnException
	}

	shouldRetry, err := retryStrategy.ShouldRetry(ctx, options, fetchResponse, attemptForRetry)
	if err != nil {
		releaseBody()
		return nil, err
	}
	if shouldRetry {
		releaseBody()
		delay := retryStrategy.RetryAfter(options, fetchResponse, attemptForRetry)
		if err := sleepContext(ctx, delay); err != nil {
			return nil, err
		}
		return c.do(ctx, options, prepared, attemptNumber+1, numRetriesOnException)
	}

	if !isException && fetchResponse.Status >= 300 && fetchResponse.Status < 400 && options.shouldFollowRedirects() {
		location := fetchResponse.Headers["Location"]
		if location == "" {
			releaseBody()
			return nil, boxerrors.NewBoxSDKError(fmt.Sprintf("Unable to follow redirect for %s", options.URL))
		}
		isSameOrigin := sameOrigin(location, options.URL)
		releaseBody()
		newOptions := *options
		newOptions.Params = nil
		newOptions.URL = location
		if !isSameOrigin {
			newOptions.Auth = nil
		}
		return c.do(ctx, &newOptions, prepared, 1, 0)
	}

	if !isException && fetchResponse.Status >= 200 && fetchResponse.Status < 400 {
		if bodyOpen {
			fetchResponse.Content = &cancelReadCloser{rc: resp.Body, cancel: cancel}
			bodyOpen = false
		}
		return fetchResponse, nil
	}

	releaseBody()

	code, requestID, helpURL, message, errStr, errDescription, contextInfo := extractErrorFields(fetchResponse.Data)

	if fetchResponse.Status == 0 {
		msg := "Unexpected Error occurred"
		if caughtErr != nil {
			msg = caughtErr.Error()
		}
		return nil, &boxerrors.BoxSDKError{Message: msg, Timestamp: nowMillis(), Err: caughtErr}
	}

	errorMessage := buildErrorMessage(fetchResponse.Status, code, message, requestID, errStr, errDescription)

	requestBody := ""
	if prepared.textualBody {
		requestBody = string(prepared.body)
	}

	return nil, boxerrors.NewBoxAPIError(
		errorMessage,
		nowMillis(),
		boxerrors.RequestInfo{
			ContentType: options.contentTypeOrDefault(),
			Method:      method,
			URL:         options.URL,
			QueryParams: options.Params,
			Headers:     headers.values,
			Body:        requestBody,
		},
		boxerrors.ResponseInfo{
			StatusCode:  fetchResponse.Status,
			Headers:     fetchResponse.Headers,
			Body:        fetchResponse.Data,
			RawBody:     string(responseBytes),
			Code:        code,
			ContextInfo: contextInfo,
			RequestID:   requestID,
			HelpURL:     helpURL,
		},
		sanitizer,
	)
}

// builtHeaders carries the assembled headers plus any error from retrieving the
// authorization header.
type builtHeaders struct {
	values  map[string]string
	authErr error
}

// buildHeaders assembles request headers with the source precedence: content
// headers (non-GET) < explicit headers < auth header < UA headers < session
// additional headers.
func buildHeaders(ctx context.Context, options *FetchOptions, prepared *preparedRequest, method string, session *NetworkSession) builtHeaders {
	headers := map[string]string{}
	if method != http.MethodGet {
		headers["Content-Type"] = prepared.contentType
		if prepared.contentMD5 != "" {
			headers["content-md5"] = prepared.contentMD5
		}
	}
	for key, value := range options.Headers {
		headers[key] = value
	}
	if options.Auth != nil {
		authHeader, err := options.Auth.RetrieveAuthorizationHeader(ctx, session)
		if err != nil {
			return builtHeaders{values: headers, authErr: err}
		}
		headers["Authorization"] = authHeader
	}
	headers["User-Agent"] = userAgentHeader
	headers["X-Box-UA"] = xBoxUaHeader
	if session != nil {
		for key, value := range session.AdditionalHeaders {
			headers[key] = value
		}
	}
	return builtHeaders{values: headers}
}

// buildURL appends the query parameters to the URL, matching the source
// concatenation logic.
func buildURL(rawURL string, params map[string]string) string {
	if len(params) == 0 {
		return rawURL
	}
	separator := "?"
	if strings.HasSuffix(rawURL, "?") {
		separator = ""
	}
	values := url.Values{}
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		values.Set(key, params[key])
	}
	return rawURL + separator + values.Encode()
}

// flattenHeaders converts an http.Header to a single-value map keyed by the
// canonical header names.
func flattenHeaders(header http.Header) map[string]string {
	out := make(map[string]string, len(header))
	for key, values := range header {
		if len(values) > 0 {
			out[key] = values[0]
		}
	}
	return out
}

// sameOrigin reports whether two URLs share the same scheme and host.
func sameOrigin(a, b string) bool {
	ua, errA := url.Parse(a)
	ub, errB := url.Parse(b)
	if errA != nil || errB != nil {
		return false
	}
	return ua.Scheme == ub.Scheme && ua.Host == ub.Host
}

// extractErrorFields pulls the standard Box error fields from a response body.
func extractErrorFields(data serialization.SerializedData) (code, requestID, helpURL, message, errStr, errDescription string, contextInfo map[string]any) {
	m, ok := data.(map[string]any)
	if !ok {
		return "", "", "", "", "", "", nil
	}
	code = serialization.GetSDValueByKey(m, "code")
	requestID = serialization.GetSDValueByKey(m, "request_id")
	helpURL = serialization.GetSDValueByKey(m, "help_url")
	message = serialization.GetSDValueByKey(m, "message")
	errStr = serialization.GetSDValueByKey(m, "error")
	errDescription = serialization.GetSDValueByKey(m, "error_description")
	if ci, ok := m["context_info"].(map[string]any); ok {
		contextInfo = ci
	}
	return code, requestID, helpURL, message, errStr, errDescription, contextInfo
}

// buildErrorMessage formats the BoxAPIError message, mirroring the source join.
func buildErrorMessage(status int, code, message, requestID, errStr, errDescription string) string {
	primary := joinNonEmpty(" ", fmt.Sprintf("%d", status), code, message)
	parts := []string{primary}
	if requestID != "" {
		parts = append(parts, "Request ID: "+requestID)
	}
	if errStr != "" {
		parts = append(parts, errStr+" - "+errDescription)
	}
	return joinNonEmpty("; ", parts...)
}

// joinNonEmpty joins the non-empty parts with the given separator.
func joinNonEmpty(sep string, parts ...string) string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			filtered = append(filtered, part)
		}
	}
	return strings.Join(filtered, sep)
}

// sleepContext waits for the given number of seconds or until ctx is canceled.
func sleepContext(ctx context.Context, seconds float64) error {
	if seconds <= 0 {
		return nil
	}
	timer := time.NewTimer(time.Duration(seconds * float64(time.Second)))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// nowMillis returns the current time as a millisecond epoch string.
func nowMillis() string {
	return fmt.Sprintf("%d", time.Now().UnixMilli())
}

// cancelReadCloser closes the underlying body and cancels the request context
// when the consumer is done reading a streamed (binary) response.
type cancelReadCloser struct {
	rc     io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelReadCloser) Read(p []byte) (int, error) { return c.rc.Read(p) }

func (c *cancelReadCloser) Close() error {
	err := c.rc.Close()
	c.cancel()
	return err
}
