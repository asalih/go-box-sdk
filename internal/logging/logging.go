// Package logging provides the DataSanitizer used to redact sensitive values
// from request and response data before they appear in errors or logs. It
// mirrors src/internal/logging.ts from the Box SDK.
package logging

import (
	"strings"

	"github.com/asalih/go-box-sdk/serialization"
)

// defaultKeysToSanitize lists the (lowercased) header and body keys whose values
// must never be exposed. The value is unused; only the key set matters.
var defaultKeysToSanitize = map[string]string{
	"authorization":              "",
	"access_token":               "",
	"refresh_token":              "",
	"subject_token":              "",
	"token":                      "",
	"client_id":                  "",
	"client_secret":              "",
	"shared_link":                "",
	"download_url":               "",
	"jwt_private_key":            "",
	"jwt_private_key_passphrase": "",
	"password":                   "",
}

// DataSanitizer redacts sensitive values from headers and bodies.
type DataSanitizer struct {
	keysToSanitize map[string]string
}

// NewDataSanitizer returns a DataSanitizer seeded with the default key set.
func NewDataSanitizer() *DataSanitizer {
	return &DataSanitizer{keysToSanitize: defaultKeysToSanitize}
}

// SanitizeHeaders returns a copy of headers with sensitive values redacted.
func (d *DataSanitizer) SanitizeHeaders(headers map[string]string) map[string]string {
	out := make(map[string]string, len(headers))
	for k, v := range headers {
		if _, ok := d.keysToSanitize[strings.ToLower(k)]; ok {
			out[k] = serialization.SanitizedValue()
			continue
		}
		out[k] = v
	}
	return out
}

// SanitizeBody returns a copy of body with sensitive values redacted.
func (d *DataSanitizer) SanitizeBody(body serialization.SerializedData) serialization.SerializedData {
	return serialization.SanitizeSerializedData(body, d.keysToSanitize)
}

// SanitizeFormEncodedBody redacts sensitive parameters in a form-encoded body.
func (d *DataSanitizer) SanitizeFormEncodedBody(body string) string {
	return serialization.SanitizeFormEncodedBodyFromString(body, d.keysToSanitize)
}

// SanitizeStringBody redacts a string body based on its content type. JSON and
// form-encoded bodies are parsed and redacted; anything else is returned as-is.
func (d *DataSanitizer) SanitizeStringBody(body, contentType string) string {
	switch contentType {
	case "application/json", "application/json-patch+json":
		parsed, err := serialization.JSONToSerializedData(body)
		if err != nil {
			return body
		}
		out, err := serialization.SDToJSON(d.SanitizeBody(parsed))
		if err != nil {
			return body
		}
		return out
	case "application/x-www-form-urlencoded":
		return d.SanitizeFormEncodedBody(body)
	default:
		return body
	}
}
