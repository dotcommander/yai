package tui

import (
	"context"
	"errors"
	"time"

	"charm.land/fantasy"
	"github.com/dotcommander/yai/internal/agent"
	"github.com/dotcommander/yai/internal/stream"
)

const ttftFormat = "[ttft: %dms]"

func waitForRetryDelay(ctx context.Context, retries int, retryErr error) {
	var d time.Duration

	var providerErr *fantasy.ProviderError
	if errors.As(retryErr, &providerErr) {
		if ra := agent.RetryAfterFromHeaders(providerErr.ResponseHeaders); ra > 0 {
			d = ra
		}
	}
	if d == 0 {
		d = agent.CalculateBackoff(retries, 500*time.Millisecond, 30*time.Second)
	}

	select {
	case <-time.After(d):
	case <-ctx.Done():
	}
}

func closeStream(s stream.Stream, cancel context.CancelFunc) {
	if s != nil {
		_ = s.Close()
	}
	if cancel != nil {
		cancel()
	}
}

// adaptiveRenderInterval returns a render debounce interval that increases
// with output size to avoid O(n²) Glamour re-rendering cost during streaming.
func adaptiveRenderInterval(bufLen int) time.Duration {
	switch {
	case bufLen > 64*1024:
		return 500 * time.Millisecond
	case bufLen > 16*1024:
		return 200 * time.Millisecond
	case bufLen > 4*1024:
		return 100 * time.Millisecond
	default:
		return 33 * time.Millisecond
	}
}
