package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dotcommander/yai/internal/agent"
	"github.com/dotcommander/yai/internal/config"
	"github.com/dotcommander/yai/internal/errs"
	"github.com/dotcommander/yai/internal/present"
	"github.com/dotcommander/yai/internal/proto"
	"github.com/dotcommander/yai/internal/stream"
)

func startManagedStream(
	ctx context.Context,
	timeout time.Duration,
	closeActive func(),
	setCancel func(context.CancelFunc),
	setStream func(stream.Stream),
	start func(context.Context) (agent.StreamStart, error),
) (agent.StreamStart, error) {
	closeActive()
	requestCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		requestCtx, cancel = context.WithTimeout(ctx, timeout)
		setCancel(cancel)
	}

	res, err := start(requestCtx)
	if err != nil {
		closeActive()
		return agent.StreamStart{}, err
	}

	setStream(res.Stream)
	return res, nil
}

func streamStartErrorMsg(err error) tea.Msg {
	var e errs.Error
	if errors.As(err, &e) {
		return e
	}
	return errs.Error{Err: err}
}

func receiveManagedStreamCmd(
	st stream.Stream,
	quiet bool,
	emitWarning func(string),
	closeActive func(),
	errh func(error) tea.Msg,
	onChunk func(string, stream.Stream, func(error) tea.Msg) tea.Msg,
	onDone func([]proto.Message) tea.Msg,
) tea.Cmd {
	return func() tea.Msg {
		if st.Next() {
			chunk, err := st.Current()
			if err != nil && !errors.Is(err, stream.ErrNoContent) {
				closeStream(st, nil)
				return errh(err)
			}
			return onChunk(chunk.Content, st, errh)
		}

		if err := st.Err(); err != nil {
			closeActive()
			return errh(err)
		}

		if !quiet {
			for _, warning := range st.DrainWarnings() {
				emitWarning(warning)
			}
		}

		results := st.CallTools()
		if len(results) > 0 {
			var content strings.Builder
			for _, call := range results {
				content.WriteString(call.String())
			}
			return onChunk(content.String(), st, errh)
		}

		messages := st.Messages()
		closeActive()
		return onDone(messages)
	}
}

func handleRetryableStreamError(
	agentSvc *agent.Service,
	noLimit bool,
	setModel func(string),
	retry func(errs.Error, string) tea.Msg,
	err error,
	mod config.Model,
	prompt string,
) tea.Msg {
	action := agentSvc.ActionForStreamError(err, mod, prompt, noLimit)
	if action.ModelOverride != "" {
		setModel(action.ModelOverride)
	}
	if action.Retry {
		next := action.Prompt
		if next == "" {
			next = prompt
		}
		return retry(action.Err, next)
	}
	if action.Err.Err == nil {
		return errs.Error{Err: err}
	}
	return action.Err
}

func warnIgnoredStop(stop []string, quiet bool, warned *bool, emitWarning func(string)) {
	if len(stop) == 0 || quiet || *warned {
		return
	}
	emitWarning("stop sequences are currently ignored by the Fantasy bridge.")
	*warned = true
}

func warnMCPDisabledForNonTTY(cfg *config.Config, warned *bool, emitWarning func(string)) {
	if cfg.Quiet || cfg.MCPAllowNonTTY || present.IsInputTTY() || len(cfg.MCPServers) == 0 || *warned {
		return
	}
	emitWarning("MCP tools are disabled for piped/non-interactive input by default. Use --mcp-allow-non-tty to enable.")
	*warned = true
}

func emitCommentWarning(commentRenderer func(...string) string, message string) {
	fmt.Fprintln(os.Stderr, commentRenderer("Warning: "+message))
}

func retryOrFail(
	ctx context.Context,
	retries *int,
	maxRetries int,
	err errs.Error,
	content string,
	submit func(string) tea.Msg,
) tea.Msg {
	*retries++
	if *retries >= maxRetries {
		return err
	}
	waitForRetryDelay(ctx, *retries, err.Err)
	return submit(content)
}
