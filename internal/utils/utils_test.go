package utils

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/youmark/pkcs8"
)

// Well-known SHA1 of the ASCII string "hello".
const (
	helloSHA1Hex    = "aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d"
	helloSHA1Base64 = "qvTGHdzF6KLavt4PO0gs2a6pQ00="
)

func TestSHA1Hex(t *testing.T) {
	assert.Equal(t, helloSHA1Hex, SHA1Hex([]byte("hello")))
}

func TestHashIncrementalDigestBase64(t *testing.T) {
	// Feeding the data in pieces yields the same digest as hashing it at once.
	h := NewHash()
	h.Update([]byte("hel"))
	h.Update([]byte("lo"))
	assert.Equal(t, helloSHA1Base64, h.DigestBase64())
}

func TestHexToBase64(t *testing.T) {
	got, err := HexToBase64(helloSHA1Hex)
	require.NoError(t, err)
	assert.Equal(t, helloSHA1Base64, got)
}

// TestChunkedUploadIntegrityInvariant asserts the property the chunked-upload
// commit relies on: the base64 digest of a buffer equals HexToBase64 of its
// hex SHA1. This is what lets the manager compare a locally computed digest to
// the hex sha1 Box returns for each part.
func TestChunkedUploadIntegrityInvariant(t *testing.T) {
	data := []byte("the quick brown fox")
	h := NewHash()
	h.Update(data)

	fromHex, err := HexToBase64(SHA1Hex(data))
	require.NoError(t, err)
	assert.Equal(t, h.DigestBase64(), fromHex)
}

func TestHexToBase64Invalid(t *testing.T) {
	_, err := HexToBase64("not-hex")
	require.Error(t, err)
}

func TestIterateChunksRemainder(t *testing.T) {
	input := []byte("0123456789") // 10 bytes
	it := IterateChunks(bytes.NewReader(input), 3, int64(len(input)))

	var chunks [][]byte
	for {
		chunk, ok, err := it.Next()
		require.NoError(t, err)
		if !ok {
			break
		}
		chunks = append(chunks, chunk)
	}

	require.Len(t, chunks, 4)
	assert.Equal(t, []byte("012"), chunks[0])
	assert.Equal(t, []byte("345"), chunks[1])
	assert.Equal(t, []byte("678"), chunks[2])
	assert.Equal(t, []byte("9"), chunks[3]) // final chunk holds the remainder
	assert.Equal(t, input, bytes.Join(chunks, nil))
}

func TestIterateChunksExactMultiple(t *testing.T) {
	input := []byte("abcdef") // 6 bytes, chunk size 3 -> exactly 2 chunks
	it := IterateChunks(bytes.NewReader(input), 3, int64(len(input)))

	var count int
	for {
		_, ok, err := it.Next()
		require.NoError(t, err)
		if !ok {
			break
		}
		count++
	}
	assert.Equal(t, 2, count)
}

func TestIterateChunksSizeMismatch(t *testing.T) {
	// Stream ends before the declared file size -> error.
	it := IterateChunks(strings.NewReader("abc"), 2, 10)
	_, _, err := it.Next() // first 2 bytes ok
	require.NoError(t, err)
	_, _, err = it.Next() // next read hits EOF early
	require.Error(t, err)
}

func TestReadByteStream(t *testing.T) {
	got, err := ReadByteStream(strings.NewReader("payload"))
	require.NoError(t, err)
	assert.Equal(t, []byte("payload"), got)
}

func TestReadTextFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(path, []byte("file-content"), 0o600))

	got, err := ReadTextFromFile(path)
	require.NoError(t, err)
	assert.Equal(t, "file-content", got)

	_, err = ReadTextFromFile(filepath.Join(dir, "missing.json"))
	require.Error(t, err)
}

func TestGetUUID(t *testing.T) {
	a := GetUUID()
	b := GetUUID()
	assert.NotEqual(t, a, b)
	_, err := uuid.Parse(a)
	require.NoError(t, err)
}

func TestEpochTimeHelpers(t *testing.T) {
	now := GetEpochTimeInSeconds()
	assert.Positive(t, now)

	ref := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	assert.Equal(t, ref.Unix(), DateTimeToEpochSeconds(ref))
	assert.Equal(t, ref, EpochSecondsToDateTime(ref.Unix()))
}

func TestSerializeDateRoundTrip(t *testing.T) {
	ref := time.Date(2024, 5, 6, 0, 0, 0, 0, time.UTC)
	s := SerializeDate(ref)
	assert.Equal(t, "2024-05-06", s)

	parsed, err := DeserializeDate(s)
	require.NoError(t, err)
	assert.Equal(t, ref, parsed)

	_, err = DeserializeDate("not-a-date")
	require.Error(t, err)
}

func TestSerializeDateTime(t *testing.T) {
	ref := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)
	assert.Equal(t, "2024-05-06T07:08:09+00:00", SerializeDateTime(ref))
}

func TestDeserializeDateTime(t *testing.T) {
	cases := []string{
		"2024-05-06T07:08:09Z",
		"2024-05-06T07:08:09+00:00",
		"2024-05-06T07:08:09",
	}
	for _, c := range cases {
		parsed, err := DeserializeDateTime(c)
		require.NoErrorf(t, err, "value %q", c)
		assert.Equal(t, 2024, parsed.Year())
	}

	_, err := DeserializeDateTime("garbage")
	require.Error(t, err)
}

func TestRandomWithinRange(t *testing.T) {
	for i := 0; i < 100; i++ {
		v := Random(1, 2)
		assert.GreaterOrEqual(t, v, 1.0)
		assert.Less(t, v, 2.0)
	}
}

func TestDefaultPrivateKeyDecryptorPKCS8(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	pemStr := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))

	dec := NewDefaultPrivateKeyDecryptor()
	got, err := dec.DecryptPrivateKey(pemStr, "")
	require.NoError(t, err)
	assert.Equal(t, key.N, got.N)
}

func TestDefaultPrivateKeyDecryptorPKCS1(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der := x509.MarshalPKCS1PrivateKey(key)
	pemStr := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der}))

	got, err := NewDefaultPrivateKeyDecryptor().DecryptPrivateKey(pemStr, "")
	require.NoError(t, err)
	assert.Equal(t, key.N, got.N)
}

func TestDefaultPrivateKeyDecryptorEncryptedPKCS8(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	passphrase := "s3cr3t"
	der, err := pkcs8.MarshalPrivateKey(key, []byte(passphrase), nil)
	require.NoError(t, err)
	pemStr := string(pem.EncodeToMemory(&pem.Block{Type: "ENCRYPTED PRIVATE KEY", Bytes: der}))

	dec := NewDefaultPrivateKeyDecryptor()
	got, err := dec.DecryptPrivateKey(pemStr, passphrase)
	require.NoError(t, err)
	assert.Equal(t, key.N, got.N)

	// Wrong passphrase fails.
	_, err = dec.DecryptPrivateKey(pemStr, "wrong")
	require.Error(t, err)
}

func TestDefaultPrivateKeyDecryptorBadPEM(t *testing.T) {
	_, err := NewDefaultPrivateKeyDecryptor().DecryptPrivateKey("not a pem", "")
	require.Error(t, err)
}

func TestCreateJWTAssertion(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	pemStr := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))

	exp := GetEpochTimeInSeconds() + 30
	assertion, err := CreateJWTAssertion(
		map[string]any{"exp": exp, "box_sub_type": "enterprise"},
		JwtKey{Key: pemStr},
		JwtSignOptions{
			Algorithm:           "RS256",
			KeyID:               "key-1",
			Audience:            "https://api.box.com/oauth2/token",
			Subject:             "ent-1",
			Issuer:              "client-1",
			JWTID:               "jti-1",
			PrivateKeyDecryptor: NewDefaultPrivateKeyDecryptor(),
		},
	)
	require.NoError(t, err)

	parsed, err := jwt.Parse(assertion, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return &key.PublicKey, nil
	})
	require.NoError(t, err)
	require.True(t, parsed.Valid)
	assert.Equal(t, "key-1", parsed.Header["kid"])

	claims := parsed.Claims.(jwt.MapClaims)
	assert.Equal(t, "client-1", claims["iss"])
	assert.Equal(t, "ent-1", claims["sub"])
	assert.Equal(t, "https://api.box.com/oauth2/token", claims["aud"])
	assert.Equal(t, "jti-1", claims["jti"])
	assert.Equal(t, "enterprise", claims["box_sub_type"])
	assert.NotNil(t, claims["iat"])
}

func TestCreateJWTAssertionMissingDecryptor(t *testing.T) {
	_, err := CreateJWTAssertion(map[string]any{}, JwtKey{}, JwtSignOptions{})
	require.Error(t, err)
}

func TestCreateJWTAssertionUnsupportedAlgorithm(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	pemStr := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))

	_, err = CreateJWTAssertion(map[string]any{}, JwtKey{Key: pemStr}, JwtSignOptions{
		Algorithm:           "BOGUS",
		PrivateKeyDecryptor: NewDefaultPrivateKeyDecryptor(),
	})
	require.Error(t, err)
}
