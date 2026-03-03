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

func waitForRetryDelay(retries int, retryErr error) {
	var providerErr *fantasy.ProviderError
	if errors.As(retryErr, &providerErr) {
		if ra := agent.RetryAfterFromHeaders(providerErr.ResponseHeaders); ra > 0 {
			time.Sleep(ra)
			return
		}
	}
	delay := agent.CalculateBackoff(retries, 500*time.Millisecond, 30*time.Second)
	time.Sleep(delay)
}

func closeStream(s stream.Stream, cancel context.CancelFunc) {
	if s != nil {
		_ = s.Close()
	}
	if cancel != nil {
		cancel()
	}
}
