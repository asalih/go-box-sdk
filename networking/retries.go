package networking

import (
	"context"
	"strconv"

	"github.com/asalih/go-box-sdk/internal/utils"
)

// RetryStrategy decides whether and when a request should be retried. It
// mirrors the RetryStrategy interface in src/networking/retries.ts.
type RetryStrategy interface {
	ShouldRetry(ctx context.Context, options *FetchOptions, response *FetchResponse, attemptNumber int) (bool, error)
	RetryAfter(options *FetchOptions, response *FetchResponse, attemptNumber int) float64
}

// BoxRetryStrategy is the default retry strategy. It mirrors BoxRetryStrategy
// in src/networking/retries.ts: up to 5 attempts, at most 2 retries on
// transport exceptions, a 1s base interval, and +/-50% jitter.
type BoxRetryStrategy struct {
	MaxAttempts              int
	RetryRandomizationFactor float64
	RetryBaseInterval        float64
	MaxRetriesOnException    int
}

// NewBoxRetryStrategy returns a BoxRetryStrategy with the source defaults.
func NewBoxRetryStrategy() *BoxRetryStrategy {
	return &BoxRetryStrategy{
		MaxAttempts:              5,
		RetryRandomizationFactor: 0.5,
		RetryBaseInterval:        1,
		MaxRetriesOnException:    2,
	}
}

// ShouldRetry reports whether the request should be retried. On a 401 with an
// auth object it first refreshes the token, mirroring the source behavior.
func (s *BoxRetryStrategy) ShouldRetry(ctx context.Context, options *FetchOptions, response *FetchResponse, attemptNumber int) (bool, error) {
	if response.Status == 0 {
		return attemptNumber <= s.MaxRetriesOnException, nil
	}
	isSuccessful := response.Status >= 200 && response.Status < 400
	_, hasRetryAfter := retryAfterHeader(response)
	isAcceptedWithRetryAfter := response.Status == 202 && hasRetryAfter

	if attemptNumber >= s.MaxAttempts {
		return false, nil
	}
	if isAcceptedWithRetryAfter {
		return true, nil
	}
	if response.Status >= 500 {
		return true, nil
	}
	if response.Status == 429 {
		return true, nil
	}
	if response.Status == 401 && options.Auth != nil {
		if _, err := options.Auth.RefreshToken(ctx, options.NetworkSession); err != nil {
			return false, err
		}
		return true, nil
	}
	if isSuccessful {
		return false, nil
	}
	return false, nil
}

// RetryAfter returns the delay in seconds before the next attempt. It honors a
// Retry-After header when present, otherwise applies exponential backoff with
// jitter.
func (s *BoxRetryStrategy) RetryAfter(options *FetchOptions, response *FetchResponse, attemptNumber int) float64 {
	if header, ok := retryAfterHeader(response); ok {
		if v, err := strconv.ParseFloat(header, 64); err == nil {
			return v
		}
	}
	randomization := utils.Random(1-s.RetryRandomizationFactor, 1+s.RetryRandomizationFactor)
	exponential := pow2(attemptNumber)
	return exponential * s.RetryBaseInterval * randomization
}

// retryAfterHeader returns the Retry-After header value if present.
func retryAfterHeader(response *FetchResponse) (string, bool) {
	v, ok := response.Headers["Retry-After"]
	return v, ok
}

// pow2 returns 2 raised to n as a float64.
func pow2(n int) float64 {
	result := 1.0
	for i := 0; i < n; i++ {
		result *= 2
	}
	return result
}
