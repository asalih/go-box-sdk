package utils

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash"
)

// Hash incrementally computes a SHA1 digest. It mirrors the source Hash class,
// which only ever uses the "sha1" algorithm.
type Hash struct {
	h hash.Hash
}

// NewHash creates a new SHA1 hash.
func NewHash() *Hash {
	return &Hash{h: sha1.New()}
}

// Update feeds more data into the hash.
func (h *Hash) Update(data []byte) {
	h.h.Write(data)
}

// DigestBase64 returns the current digest encoded as standard base64. The hash
// may continue to be updated afterwards.
func (h *Hash) DigestBase64() string {
	return base64.StdEncoding.EncodeToString(h.h.Sum(nil))
}

// SHA1Hex returns the SHA1 hash of data as a lowercase hex string. Box's
// content-md5 request header uses the SHA1 of the file content despite its name;
// this mirrors the source calculateMD5Hash helper.
func SHA1Hex(data []byte) string {
	sum := sha1.Sum(data)
	return hex.EncodeToString(sum[:])
}

// HexToBase64 decodes a hex string and re-encodes it as standard base64.
func HexToBase64(data string) (string, error) {
	b, err := hex.DecodeString(data)
	if err != nil {
		return "", fmt.Errorf("utils: failed to decode hex: %w", err)
	}
	return base64.StdEncoding.EncodeToString(b), nil
}
