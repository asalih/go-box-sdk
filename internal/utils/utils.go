// Package utils provides low-level helpers shared across the SDK: hashing,
// chunked stream iteration, JWT assertion creation, date formatting, and small
// utilities. It mirrors src/internal/utils.ts and src/internal/utilsNode.ts.
package utils

import (
	"io"
	"math/rand"
	"os"
	"time"

	"github.com/google/uuid"
)

// ByteStream is the streamed body type used by request and response bodies.
type ByteStream = io.Reader

// GetUUID returns a new random UUID string, used for the JWT jti claim.
func GetUUID() string {
	return uuid.NewString()
}

// GetEpochTimeInSeconds returns the current Unix time in seconds.
func GetEpochTimeInSeconds() int64 {
	return time.Now().Unix()
}

// ReadByteStream reads a stream fully into a buffer.
func ReadByteStream(stream io.Reader) ([]byte, error) {
	return io.ReadAll(stream)
}

// ReadTextFromFile reads a file and returns its content as a string.
func ReadTextFromFile(filepath string) (string, error) {
	b, err := os.ReadFile(filepath)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Random returns a pseudo-random float64 in the half-open interval [min, max).
func Random(min, max float64) float64 {
	return rand.Float64()*(max-min) + min
}
