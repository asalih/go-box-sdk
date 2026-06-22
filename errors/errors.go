// Package boxerrors defines the error types returned by the SDK. It mirrors
// src/box/errors.ts and src/internal/errors.ts from the Box SDK. It lives in a
// dedicated leaf package so that both the networking layer and the auth layer
// can depend on it without creating an import cycle.
package boxerrors

import (
	"encoding/json"

	"github.com/asalih/go-box-sdk/serialization"
)

// Sanitizer redacts sensitive values from headers and bodies. It is implemented
// by the internal logging.DataSanitizer and injected into BoxAPIError so the
// error package does not depend on the logging package.
type Sanitizer interface {
	SanitizeHeaders(headers map[string]string) map[string]string
	SanitizeBody(body serialization.SerializedData) serialization.SerializedData
	SanitizeStringBody(body, contentType string) string
}

// BoxSDKError is the base error type for all errors raised by the SDK.
type BoxSDKError struct {
	Message   string
	Timestamp string
	// Err holds an optional wrapped cause.
	Err error
}

// Error implements the error interface.
func (e *BoxSDKError) Error() string { return e.Message }

// Unwrap returns the wrapped cause, if any.
func (e *BoxSDKError) Unwrap() error { return e.Err }

// NewBoxSDKError creates a BoxSDKError with the given message.
func NewBoxSDKError(message string) *BoxSDKError {
	return &BoxSDKError{Message: message}
}

// RequestInfo captures the request details attached to a BoxAPIError.
type RequestInfo struct {
	ContentType string
	Method      string
	URL         string
	QueryParams map[string]string
	Headers     map[string]string
	Body        string
}

// ResponseInfo captures the response details attached to a BoxAPIError.
type ResponseInfo struct {
	StatusCode  int
	Headers     map[string]string
	Body        serialization.SerializedData
	RawBody     string
	Code        string
	ContextInfo map[string]any
	RequestID   string
	HelpURL     string
}

// BoxAPIError is raised when the Box API returns a non-success response. It
// carries the full request and response context for debugging.
type BoxAPIError struct {
	BoxSDKError
	RequestInfo  RequestInfo
	ResponseInfo ResponseInfo
	sanitizer    Sanitizer
}

// NewBoxAPIError builds a BoxAPIError. If sanitizer is nil, no redaction is
// applied when rendering the error as JSON.
func NewBoxAPIError(message, timestamp string, reqInfo RequestInfo, respInfo ResponseInfo, sanitizer Sanitizer) *BoxAPIError {
	return &BoxAPIError{
		BoxSDKError:  BoxSDKError{Message: message, Timestamp: timestamp},
		RequestInfo:  reqInfo,
		ResponseInfo: respInfo,
		sanitizer:    sanitizer,
	}
}

// Error returns the concise error message.
func (e *BoxAPIError) Error() string { return e.Message }

// JSONString renders the error as pretty-printed JSON with sensitive values
// redacted, mirroring BoxApiError.toString in the source.
func (e *BoxAPIError) JSONString() string {
	reqHeaders := e.RequestInfo.Headers
	reqBody := any(e.RequestInfo.Body)
	respHeaders := e.ResponseInfo.Headers
	respBody := e.ResponseInfo.Body
	if e.sanitizer != nil {
		reqHeaders = e.sanitizer.SanitizeHeaders(e.RequestInfo.Headers)
		respHeaders = e.sanitizer.SanitizeHeaders(e.ResponseInfo.Headers)
		respBody = e.sanitizer.SanitizeBody(e.ResponseInfo.Body)
		if e.RequestInfo.Body != "" {
			reqBody = e.sanitizer.SanitizeStringBody(e.RequestInfo.Body, e.RequestInfo.ContentType)
		}
	}

	repr := map[string]any{
		"name":      "BoxApiError",
		"message":   e.Message,
		"timestamp": e.Timestamp,
		"requestInfo": map[string]any{
			"method":      e.RequestInfo.Method,
			"url":         e.RequestInfo.URL,
			"queryParams": e.RequestInfo.QueryParams,
			"headers":     reqHeaders,
			"body":        reqBody,
		},
		"responseInfo": map[string]any{
			"statusCode":  e.ResponseInfo.StatusCode,
			"headers":     respHeaders,
			"body":        respBody,
			"code":        e.ResponseInfo.Code,
			"contextInfo": e.ResponseInfo.ContextInfo,
			"requestId":   e.ResponseInfo.RequestID,
			"helpUrl":     e.ResponseInfo.HelpURL,
		},
	}
	b, err := json.MarshalIndent(repr, "", "  ")
	if err != nil {
		return e.Message
	}
	return string(b)
}
