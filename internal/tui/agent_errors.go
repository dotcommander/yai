package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/dotcommander/yai/internal/config"
	"github.com/dotcommander/yai/internal/errs"
)

func (m *Yai) handleStreamError(err error, mod config.Model, prompt string) tea.Msg {
	return handleRetryableStreamError(m.agent, m.Config.NoLimit, func(model string) {
		m.Config.Model = model
	}, func(retryErr errs.Error, next string) tea.Msg {
		return m.retry(next, retryErr)
	}, err, mod, prompt)
}
