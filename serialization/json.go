// Package serialization provides the SerializedData type and helpers used across
// the SDK to convert between JSON text, in-memory data, and URL-encoded form
// bodies. It mirrors src/serialization/json.ts from the Box SDK.
package serialization

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// SerializedData is the dynamic representation of a JSON value. After
// unmarshalling it is one of: nil, bool, float64, string, []any, or
// map[string]any.
type SerializedData = any

// JSONToSerializedData parses JSON text into SerializedData.
func JSONToSerializedData(text string) (SerializedData, error) {
	var data SerializedData
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		return nil, fmt.Errorf("serialization: failed to parse JSON: %w", err)
	}
	return data, nil
}

// SDToJSON serializes SerializedData into compact JSON text.
func SDToJSON(data SerializedData) (string, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("serialization: failed to encode JSON: %w", err)
	}
	return string(b), nil
}

// SDToURLParams encodes a map (or JSON object string) into an
// application/x-www-form-urlencoded body. Entries with nil values are dropped.
func SDToURLParams(data SerializedData) (string, error) {
	if SDIsString(data) {
		parsed, err := JSONToSerializedData(data.(string))
		if err != nil {
			return "", err
		}
		data = parsed
	}
	m, ok := data.(map[string]any)
	if !ok {
		// Allow typed structs (with JSON tags) by round-tripping through JSON.
		// This lets managers pass request-body structs for form-encoded
		// endpoints while preserving snake_case keys and omitempty semantics.
		b, err := json.Marshal(data)
		if err != nil {
			return "", fmt.Errorf("serialization: failed to encode object for SDToURLParams: %w", err)
		}
		if err := json.Unmarshal(b, &m); err != nil {
			return "", fmt.Errorf("serialization: expecting an object or string for SDToURLParams")
		}
	}

	// Stable ordering keeps output deterministic for tests and logging.
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	values := url.Values{}
	for _, k := range keys {
		v := m[k]
		if v == nil {
			continue
		}
		values.Set(k, ToString(v))
	}
	return values.Encode(), nil
}

// GetSDValueByKey returns the string representation of a map entry, or "".
func GetSDValueByKey(obj SerializedData, key string) string {
	if m, ok := obj.(map[string]any); ok {
		if v, ok := m[key]; ok && v != nil {
			return ToString(v)
		}
	}
	return ""
}

// SDIsEmpty reports whether the value is nil.
func SDIsEmpty(data SerializedData) bool { return data == nil }

// SDIsBoolean reports whether the value is a bool.
func SDIsBoolean(data SerializedData) bool { _, ok := data.(bool); return ok }

// SDIsNumber reports whether the value is a JSON number (float64).
func SDIsNumber(data SerializedData) bool { _, ok := data.(float64); return ok }

// SDIsString reports whether the value is a string.
func SDIsString(data SerializedData) bool { _, ok := data.(string); return ok }

// SDIsList reports whether the value is a JSON array.
func SDIsList(data SerializedData) bool { _, ok := data.([]any); return ok }

// SDIsMap reports whether the value is a JSON object.
func SDIsMap(data SerializedData) bool { _, ok := data.(map[string]any); return ok }

// ToString converts a SerializedData value to its string form, mirroring the
// behavior of the source toString helper for scalars and arrays.
func ToString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case []any:
		parts := make([]string, len(v))
		for i, item := range v {
			parts[i] = ToString(item)
		}
		return strings.Join(parts, ",")
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}

// SanitizedValue is the placeholder substituted for sensitive data.
func SanitizedValue() string { return "---[redacted]---" }

// SanitizeFormEncodedBodyFromString redacts sensitive keys in a form body.
func SanitizeFormEncodedBodyFromString(body string, keysToSanitize map[string]string) string {
	params := strings.Split(body, "&")
	for i, parameter := range params {
		params[i] = sanitizeFormEncodedParameter(parameter, keysToSanitize)
	}
	return strings.Join(params, "&")
}

func sanitizeFormEncodedParameter(parameter string, keysToSanitize map[string]string) string {
	idx := strings.Index(parameter, "=")
	if idx < 0 {
		return parameter
	}
	key := parameter[:idx]
	value := parameter[idx+1:]
	if _, ok := keysToSanitize[strings.ToLower(key)]; ok {
		value = SanitizedValue()
	}
	return key + "=" + value
}

// SanitizeSerializedData returns a copy of sd with sensitive string values
// replaced by the sanitized placeholder, recursing into nested maps.
func SanitizeSerializedData(sd SerializedData, keysToSanitize map[string]string) SerializedData {
	m, ok := sd.(map[string]any)
	if !ok {
		return sd
	}
	out := make(map[string]any, len(m))
	for key, value := range m {
		if _, redact := keysToSanitize[strings.ToLower(key)]; redact {
			if _, isStr := value.(string); isStr {
				out[key] = SanitizedValue()
				continue
			}
		}
		switch value.(type) {
		case map[string]any, []any:
			out[key] = SanitizeSerializedData(value, keysToSanitize)
		default:
			out[key] = value
		}
	}
	return out
}
