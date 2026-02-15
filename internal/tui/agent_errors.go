package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/dotcommander/yai/internal/config"
	"github.com/dotcommander/yai/internal/errs"
)

func (m *Yai) handleStreamError(err error, mod config.Model, prompt string) tea.Msg {
	action := m.agent.ActionForStreamError(err, mod, prompt)
	if action.ModelOverride != "" {
		m.Config.Model = action.ModelOverride
	}
	if action.Retry {
		next := action.Prompt
		if next == "" {
			next = prompt
		}
		return m.retry(next, action.Err)
	}
	if action.Err.Err == nil {
		return errs.Error{Err: err}
	}
	return action.Err
}
