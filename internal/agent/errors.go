package agent

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"charm.land/fantasy"

	"github.com/dotcommander/yai/internal/config"
	"github.com/dotcommander/yai/internal/errs"
)

// StreamErrorAction describes how yai should respond to a streaming error.
type StreamErrorAction struct {
	Retry         bool
	Prompt        string
	ModelOverride string
	Err           errs.Error
}

// ActionForStreamError decides whether a provider error should be retried, and
// if so which prompt/model override should be used.
func (s *Service) ActionForStreamError(err error, mod config.Model, prompt string) StreamErrorAction {
	var providerErr *fantasy.ProviderError
	if errors.As(err, &providerErr) {
		return s.actionForProviderError(providerErr, mod, prompt)
	}
	return StreamErrorAction{
		Err: errs.Error{Err: err, Reason: fmt.Sprintf("There was a problem with the %s API request.", mod.API)},
	}
}

func (s *Service) actionForProviderError(err *fantasy.ProviderError, mod config.Model, prompt string) StreamErrorAction {
	cfg := s.cfg
	switch err.StatusCode {
	case http.StatusNotFound:
		if mod.Fallback != "" {
			reason := fantasy.ErrorTitleForStatusCode(err.StatusCode)
			if reason == "" {
				reason = fmt.Sprintf("%s API server error.", mod.API)
			}
			return StreamErrorAction{
				Retry:         true,
				Prompt:        prompt,
				ModelOverride: mod.Fallback,
				Err:           errs.Error{Err: err, Reason: reason},
			}
		}
		return StreamErrorAction{
			Err: errs.Error{Err: err, Reason: fmt.Sprintf("Missing model '%s' for API '%s'.", cfg.Model, cfg.API)},
		}

	case http.StatusBadRequest:
		if isContextLengthExceeded(err) {
			pe := errs.Error{Err: err, Reason: "Maximum prompt size exceeded."}
			if cfg.NoLimit {
				return StreamErrorAction{Err: pe}
			}
			return StreamErrorAction{
				Retry:  true,
				Prompt: cutPrompt(err.Error(), prompt),
				Err:    pe,
			}
		}
		reason := fantasy.ErrorTitleForStatusCode(err.StatusCode)
		if reason == "" {
			reason = fmt.Sprintf("%s API request error.", mod.API)
		}
		return StreamErrorAction{Err: errs.Error{Err: err, Reason: reason}}
	}

	if err.IsRetryable() {
		reason := fantasy.ErrorTitleForStatusCode(err.StatusCode)
		if reason == "" {
			reason = "Retryable API error."
		}
		return StreamErrorAction{
			Retry:  true,
			Prompt: prompt,
			Err:    errs.Error{Err: err, Reason: reason},
		}
	}

	reason := fantasy.ErrorTitleForStatusCode(err.StatusCode)
	if reason == "" {
		reason = fmt.Sprintf("%s API request error.", mod.API)
	}
	return StreamErrorAction{Err: errs.Error{Err: err, Reason: reason}}
}

func isContextLengthExceeded(err *fantasy.ProviderError) bool {
	if strings.Contains(strings.ToLower(err.Message), "context_length_exceeded") {
		return true
	}
	if strings.Contains(strings.ToLower(string(err.ResponseBody)), "context_length_exceeded") {
		return true
	}
	return false
}

var tokenErrRe = regexp.MustCompile(`This model's maximum context length is (\d+) tokens. However, your messages resulted in (\d+) tokens`)

func cutPrompt(msg, prompt string) string {
	found := tokenErrRe.FindStringSubmatch(msg)
	if len(found) != 3 { //nolint:mnd
		return prompt
	}

	maxt, _ := strconv.Atoi(found[1])
	current, _ := strconv.Atoi(found[2])

	if maxt > current {
		return prompt
	}

	// 1 token =~ 4 chars
	// cut 10 extra chars 'just in case'
	reduceBy := 10 + (current-maxt)*4 //nolint:mnd
	if len(prompt) > reduceBy {
		return prompt[:len(prompt)-reduceBy]
	}

	return prompt
}
