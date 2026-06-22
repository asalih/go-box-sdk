package utils

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
	"github.com/youmark/pkcs8"
)

// JwtKey holds a PEM-encoded private key and its passphrase.
type JwtKey struct {
	Key        string
	Passphrase string
}

// JwtSignOptions configures JWT assertion creation.
type JwtSignOptions struct {
	Algorithm           string
	KeyID               string
	Audience            string
	Subject             string
	Issuer              string
	JWTID               string
	PrivateKeyDecryptor PrivateKeyDecryptor
}

// PrivateKeyDecryptor decrypts a private key for JWT auth. It mirrors the
// source PrivateKeyDecryptor interface but returns a parsed RSA key.
type PrivateKeyDecryptor interface {
	DecryptPrivateKey(encryptedPrivateKey, passphrase string) (*rsa.PrivateKey, error)
}

// DefaultPrivateKeyDecryptor decrypts PEM-encoded RSA private keys, supporting
// passphrase-encrypted PKCS#8, plain PKCS#8, and PKCS#1 (including legacy
// encrypted PKCS#1) keys.
type DefaultPrivateKeyDecryptor struct{}

// NewDefaultPrivateKeyDecryptor returns the default decryptor.
func NewDefaultPrivateKeyDecryptor() *DefaultPrivateKeyDecryptor {
	return &DefaultPrivateKeyDecryptor{}
}

// DecryptPrivateKey parses a PEM private key, decrypting it with the passphrase
// when required.
func (d *DefaultPrivateKeyDecryptor) DecryptPrivateKey(encryptedPrivateKey, passphrase string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(encryptedPrivateKey))
	if block == nil {
		return nil, fmt.Errorf("utils: failed to decode PEM private key block")
	}

	switch block.Type {
	case "ENCRYPTED PRIVATE KEY":
		key, err := pkcs8.ParsePKCS8PrivateKey(block.Bytes, []byte(passphrase))
		if err != nil {
			return nil, fmt.Errorf("utils: failed to parse encrypted PKCS#8 private key: %w", err)
		}
		return asRSAPrivateKey(key)
	case "PRIVATE KEY":
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("utils: failed to parse PKCS#8 private key: %w", err)
		}
		return asRSAPrivateKey(key)
	case "RSA PRIVATE KEY":
		der := block.Bytes
		//nolint:staticcheck // legacy encrypted PKCS#1 PEM support is required for older Box keys.
		if x509.IsEncryptedPEMBlock(block) {
			decrypted, err := x509.DecryptPEMBlock(block, []byte(passphrase))
			if err != nil {
				return nil, fmt.Errorf("utils: failed to decrypt PKCS#1 private key: %w", err)
			}
			der = decrypted
		}
		key, err := x509.ParsePKCS1PrivateKey(der)
		if err != nil {
			return nil, fmt.Errorf("utils: failed to parse PKCS#1 private key: %w", err)
		}
		return key, nil
	default:
		return nil, fmt.Errorf("utils: unsupported private key PEM type %q", block.Type)
	}
}

func asRSAPrivateKey(key any) (*rsa.PrivateKey, error) {
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("utils: private key is not an RSA key (got %T)", key)
	}
	return rsaKey, nil
}

// CreateJWTAssertion builds and signs a JWT assertion. The signing algorithm is
// RS256 (the only algorithm Box uses); the key id is placed in the header and
// the issued-at claim is set to the current time. The exp claim must already be
// present in claims.
func CreateJWTAssertion(claims map[string]any, key JwtKey, options JwtSignOptions) (string, error) {
	if options.PrivateKeyDecryptor == nil {
		return "", fmt.Errorf("utils: missing private key decryptor")
	}
	privateKey, err := options.PrivateKeyDecryptor.DecryptPrivateKey(key.Key, key.Passphrase)
	if err != nil {
		return "", err
	}
	if privateKey == nil {
		return "", fmt.Errorf("utils: decrypted jwt private key is empty")
	}

	algorithm := options.Algorithm
	if algorithm == "" {
		algorithm = "RS256"
	}
	method := jwt.GetSigningMethod(algorithm)
	if method == nil {
		return "", fmt.Errorf("utils: unsupported JWT signing algorithm %q", algorithm)
	}

	mapClaims := jwt.MapClaims{}
	for k, v := range claims {
		mapClaims[k] = v
	}
	if options.Audience != "" {
		mapClaims["aud"] = options.Audience
	}
	if options.Issuer != "" {
		mapClaims["iss"] = options.Issuer
	}
	if options.JWTID != "" {
		mapClaims["jti"] = options.JWTID
	}
	if options.Subject != "" {
		mapClaims["sub"] = options.Subject
	}
	mapClaims["iat"] = GetEpochTimeInSeconds()

	token := jwt.NewWithClaims(method, mapClaims)
	if options.KeyID != "" {
		token.Header["kid"] = options.KeyID
	}

	signed, err := token.SignedString(privateKey)
	if err != nil {
		return "", fmt.Errorf("utils: failed to sign JWT assertion: %w", err)
	}
	return signed, nil
}
