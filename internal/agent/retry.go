package agent

import (
	"math/rand/v2"
	"strconv"
	"time"
)

// CalculateBackoff returns a jittered exponential backoff duration.
// The result is initial * 2^attempt, capped at maxDur, with ±12.5% jitter.
func CalculateBackoff(attempt int, initial, maxDur time.Duration) time.Duration {
	if attempt > 62 {
		attempt = 62
	}
	d := initial * time.Duration(1<<uint(attempt))
	if d > maxDur || d <= 0 {
		d = maxDur
	}
	jitter := 0.875 + rand.Float64()*0.25 //nolint:gosec // G404: jitter calculation does not require cryptographic randomness
	return time.Duration(float64(d) * jitter)
}

// RetryAfterFromHeaders extracts a retry-after delay from provider response
// headers (retry-after-ms, retry-after). Returns 0 if no valid header found.
func RetryAfterFromHeaders(headers map[string]string) time.Duration {
	if headers == nil {
		return 0
	}
	if ms, ok := headers["retry-after-ms"]; ok {
		if v, err := strconv.ParseFloat(ms, 64); err == nil && v > 0 {
			return time.Duration(v * float64(time.Millisecond))
		}
	}
	if ra, ok := headers["retry-after"]; ok {
		if secs, err := strconv.ParseFloat(ra, 64); err == nil && secs > 0 {
			return time.Duration(secs * float64(time.Second))
		}
	}
	return 0
}
