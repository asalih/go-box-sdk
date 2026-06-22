// Package schemas contains the Box API data models used by the focused set of
// managers (files, folders, uploads, chunked uploads, downloads) and the
// authentication layer. It mirrors the relevant files under src/schemas in the
// Box SDK.
//
// Models are expressed as idiomatic Go structs with JSON tags (using struct
// embedding to mirror the base/mini/full hierarchy). This preserves the exact
// wire format while avoiding the hand-written per-field serializers used by the
// generated source. Optional scalar fields are pointers so that absent values
// can be distinguished from zero values.
package schemas

import (
	"encoding/json"
	"fmt"

	"github.com/asalih/go-box-sdk/serialization"
)

// Decode converts already-parsed SerializedData into a typed struct via a JSON
// round-trip. It mirrors the source deserialize* functions.
func Decode[T any](data serialization.SerializedData) (T, error) {
	var out T
	b, err := json.Marshal(data)
	if err != nil {
		return out, fmt.Errorf("schemas: failed to encode response data: %w", err)
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return out, fmt.Errorf("schemas: failed to decode response data: %w", err)
	}
	return out, nil
}

// DecodeBytes converts raw JSON bytes into a typed struct.
func DecodeBytes[T any](data []byte) (T, error) {
	var out T
	if err := json.Unmarshal(data, &out); err != nil {
		return out, fmt.Errorf("schemas: failed to decode response data: %w", err)
	}
	return out, nil
}

// String returns a pointer to s. Useful for building optional request fields.
func String(s string) *string { return &s }

// Int64 returns a pointer to i.
func Int64(i int64) *int64 { return &i }

// Bool returns a pointer to b.
func Bool(b bool) *bool { return &b }
