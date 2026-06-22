package networking

import (
	"io"

	"github.com/asalih/go-box-sdk/serialization"
)

// ResponseFormat indicates how the response body should be handled.
type ResponseFormat string

// Supported response formats.
const (
	ResponseFormatJSON      ResponseFormat = "json"
	ResponseFormatBinary    ResponseFormat = "binary"
	ResponseFormatNoContent ResponseFormat = "no_content"
)

// Supported request content types.
const (
	ContentTypeJSON          = "application/json"
	ContentTypeJSONPatch     = "application/json-patch+json"
	ContentTypeURLEncoded    = "application/x-www-form-urlencoded"
	ContentTypeOctetStream   = "application/octet-stream"
	ContentTypeMultipartForm = "multipart/form-data"
)

// MultipartItem is one part of a multipart/form-data request body. Exactly one
// of Data or FileStream must be set. It mirrors the MultipartItem interface in
// src/networking/fetchOptions.ts.
type MultipartItem struct {
	PartName    string
	Data        serialization.SerializedData
	FileStream  io.Reader
	FileName    string
	ContentType string
}

// FetchOptions describes a single HTTP request. It mirrors the FetchOptions
// class in src/networking/fetchOptions.ts. ContentType defaults to
// application/json and ResponseFormat defaults to json when left empty.
type FetchOptions struct {
	URL            string
	Method         string
	Params         map[string]string
	Headers        map[string]string
	Data           serialization.SerializedData
	FileStream     io.Reader
	MultipartData  []MultipartItem
	ContentType    string
	ResponseFormat ResponseFormat
	Auth           Authentication
	NetworkSession *NetworkSession
	// FollowRedirects defaults to true when nil.
	FollowRedirects *bool
}

// contentTypeOrDefault returns the configured content type or the default.
func (o *FetchOptions) contentTypeOrDefault() string {
	if o.ContentType == "" {
		return ContentTypeJSON
	}
	return o.ContentType
}

// responseFormatOrDefault returns the configured response format or the default.
func (o *FetchOptions) responseFormatOrDefault() ResponseFormat {
	if o.ResponseFormat == "" {
		return ResponseFormatJSON
	}
	return o.ResponseFormat
}

// shouldFollowRedirects reports whether redirects should be followed (default true).
func (o *FetchOptions) shouldFollowRedirects() bool {
	return o.FollowRedirects == nil || *o.FollowRedirects
}

// FetchResponse is the result of an HTTP request. It mirrors the FetchResponse
// interface in src/networking/fetchResponse.ts. Content streams the body for
// binary responses; ContentBytes holds the buffered body for json responses.
type FetchResponse struct {
	URL          string
	Status       int
	Data         serialization.SerializedData
	Content      io.Reader
	ContentBytes []byte
	Headers      map[string]string
}
