package agent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCalculateBackoff(t *testing.T) {
	tests := []struct {
		name     string
		attempt  int
		initial  time.Duration
		max      time.Duration
		wantBase time.Duration
	}{
		{
			name:     "attempt 0 equals initial",
			attempt:  0,
			initial:  500 * time.Millisecond,
			max:      30 * time.Second,
			wantBase: 500 * time.Millisecond,
		},
		{
			name:     "attempt 1 doubles",
			attempt:  1,
			initial:  500 * time.Millisecond,
			max:      30 * time.Second,
			wantBase: 1000 * time.Millisecond,
		},
		{
			name:     "attempt 10 capped at max",
			attempt:  10,
			initial:  500 * time.Millisecond,
			max:      30 * time.Second,
			wantBase: 30 * time.Second,
		},
		{
			name:     "overflow capped at max",
			attempt:  100,
			initial:  500 * time.Millisecond,
			max:      30 * time.Second,
			wantBase: 30 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateBackoff(tt.attempt, tt.initial, tt.max)
			lo := time.Duration(float64(tt.wantBase) * 0.875)
			hi := time.Duration(float64(tt.wantBase) * 1.125)
			assert.GreaterOrEqual(t, got, lo, "backoff %v below lower bound %v", got, lo)
			assert.LessOrEqual(t, got, hi, "backoff %v above upper bound %v", got, hi)
		})
	}
}

func TestRetryAfterFromHeaders(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
		wantMin time.Duration
		wantMax time.Duration
	}{
		{
			name:    "nil headers",
			headers: nil,
			wantMin: 0,
			wantMax: 0,
		},
		{
			name:    "retry-after-ms",
			headers: map[string]string{"retry-after-ms": "500"},
			wantMin: 500 * time.Millisecond,
			wantMax: 500 * time.Millisecond,
		},
		{
			name:    "retry-after seconds",
			headers: map[string]string{"retry-after": "2"},
			wantMin: 2 * time.Second,
			wantMax: 2 * time.Second,
		},
		{
			name:    "retry-after-ms takes priority",
			headers: map[string]string{"retry-after-ms": "100", "retry-after": "5"},
			wantMin: 100 * time.Millisecond,
			wantMax: 100 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RetryAfterFromHeaders(tt.headers)
			assert.GreaterOrEqual(t, got, tt.wantMin)
			assert.LessOrEqual(t, got, tt.wantMax)
		})
	}
}
