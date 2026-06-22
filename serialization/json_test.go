package serialization

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONToSerializedDataRoundTrip(t *testing.T) {
	data, err := JSONToSerializedData(`{"a":1,"b":"x","c":[1,2],"d":null}`)
	require.NoError(t, err)

	m, ok := data.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(1), m["a"])
	assert.Equal(t, "x", m["b"])
	assert.Equal(t, []any{float64(1), float64(2)}, m["c"])
	assert.Nil(t, m["d"])

	// Re-serializing yields valid JSON that parses back to the same value.
	out, err := SDToJSON(m)
	require.NoError(t, err)
	reparsed, err := JSONToSerializedData(out)
	require.NoError(t, err)
	assert.Equal(t, m, reparsed)
}

func TestJSONToSerializedDataInvalid(t *testing.T) {
	_, err := JSONToSerializedData(`{not json`)
	require.Error(t, err)
}

func TestSDToURLParamsFromMap(t *testing.T) {
	encoded, err := SDToURLParams(map[string]any{
		"grant_type": "client_credentials",
		"client_id":  "abc",
		"empty":      nil, // nil values are dropped
	})
	require.NoError(t, err)

	q, err := url.ParseQuery(encoded)
	require.NoError(t, err)
	assert.Equal(t, "client_credentials", q.Get("grant_type"))
	assert.Equal(t, "abc", q.Get("client_id"))
	assert.False(t, q.Has("empty"))
}

func TestSDToURLParamsFromJSONString(t *testing.T) {
	encoded, err := SDToURLParams(`{"client_id":"abc","scope":"x y"}`)
	require.NoError(t, err)

	q, err := url.ParseQuery(encoded)
	require.NoError(t, err)
	assert.Equal(t, "abc", q.Get("client_id"))
	assert.Equal(t, "x y", q.Get("scope"))
}

func TestSDToURLParamsFromStruct(t *testing.T) {
	// A typed struct with json tags must round-trip through JSON, preserving
	// snake_case keys and omitempty semantics.
	body := struct {
		GrantType    string `json:"grant_type"`
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret,omitempty"`
	}{
		GrantType: "client_credentials",
		ClientID:  "abc",
		// ClientSecret left empty -> omitted.
	}

	encoded, err := SDToURLParams(body)
	require.NoError(t, err)

	q, err := url.ParseQuery(encoded)
	require.NoError(t, err)
	assert.Equal(t, "client_credentials", q.Get("grant_type"))
	assert.Equal(t, "abc", q.Get("client_id"))
	assert.False(t, q.Has("client_secret"))
}

func TestSDToURLParamsDeterministicOrder(t *testing.T) {
	// Keys are sorted, so the encoded output is stable across calls.
	in := map[string]any{"b": "2", "a": "1", "c": "3"}
	first, err := SDToURLParams(in)
	require.NoError(t, err)
	second, err := SDToURLParams(in)
	require.NoError(t, err)
	assert.Equal(t, first, second)
	assert.Equal(t, "a=1&b=2&c=3", first)
}

func TestToString(t *testing.T) {
	assert.Equal(t, "", ToString(nil))
	assert.Equal(t, "hello", ToString("hello"))
	assert.Equal(t, "true", ToString(true))
	assert.Equal(t, "false", ToString(false))
	assert.Equal(t, "42", ToString(float64(42)))
	assert.Equal(t, "3.5", ToString(float64(3.5)))
	assert.Equal(t, "1,2,3", ToString([]any{float64(1), float64(2), float64(3)}))
	// Objects fall back to JSON encoding.
	assert.Equal(t, `{"k":"v"}`, ToString(map[string]any{"k": "v"}))
}

func TestGetSDValueByKey(t *testing.T) {
	obj := map[string]any{"name": "evidence", "size": float64(10), "nil": nil}
	assert.Equal(t, "evidence", GetSDValueByKey(obj, "name"))
	assert.Equal(t, "10", GetSDValueByKey(obj, "size"))
	assert.Equal(t, "", GetSDValueByKey(obj, "nil"))
	assert.Equal(t, "", GetSDValueByKey(obj, "missing"))
	assert.Equal(t, "", GetSDValueByKey("not a map", "name"))
}

func TestSDTypeGuards(t *testing.T) {
	assert.True(t, SDIsEmpty(nil))
	assert.False(t, SDIsEmpty("x"))

	assert.True(t, SDIsBoolean(true))
	assert.False(t, SDIsBoolean("true"))

	assert.True(t, SDIsNumber(float64(1)))
	assert.False(t, SDIsNumber(1)) // int is not the JSON number type

	assert.True(t, SDIsString("x"))
	assert.False(t, SDIsString(1))

	assert.True(t, SDIsList([]any{1}))
	assert.False(t, SDIsList("x"))

	assert.True(t, SDIsMap(map[string]any{}))
	assert.False(t, SDIsMap([]any{}))
}

func TestSanitizeFormEncodedBodyFromString(t *testing.T) {
	keys := map[string]string{"client_secret": "", "refresh_token": ""}
	body := "grant_type=client_credentials&client_secret=topsecret&refresh_token=abc"
	got := SanitizeFormEncodedBodyFromString(body, keys)

	assert.Contains(t, got, "grant_type=client_credentials")
	assert.Contains(t, got, "client_secret="+SanitizedValue())
	assert.Contains(t, got, "refresh_token="+SanitizedValue())
	assert.NotContains(t, got, "topsecret")
	assert.NotContains(t, got, "refresh_token=abc")
}

func TestSanitizeSerializedData(t *testing.T) {
	keys := map[string]string{"access_token": "", "authorization": ""}
	in := map[string]any{
		"access_token": "secret-token",
		"keep":         "visible",
		"nested": map[string]any{
			"authorization": "Bearer xyz",
			"other":         "ok",
		},
		// Non-string sensitive values are left untouched (only strings redacted).
		"count": float64(5),
	}

	out := SanitizeSerializedData(in, keys).(map[string]any)
	assert.Equal(t, SanitizedValue(), out["access_token"])
	assert.Equal(t, "visible", out["keep"])
	assert.Equal(t, float64(5), out["count"])

	nested := out["nested"].(map[string]any)
	assert.Equal(t, SanitizedValue(), nested["authorization"])
	assert.Equal(t, "ok", nested["other"])

	// The original input is not mutated.
	assert.Equal(t, "secret-token", in["access_token"])
}

func TestSanitizeSerializedDataNonMapPassthrough(t *testing.T) {
	assert.Equal(t, "scalar", SanitizeSerializedData("scalar", map[string]string{"x": ""}))
}
