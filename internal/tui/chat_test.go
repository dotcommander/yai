package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dotcommander/yai/internal/config"
	"github.com/dotcommander/yai/internal/proto"
)

func newTestChat(opts ...func(*Chat)) *Chat {
	r := lipgloss.DefaultRenderer()
	cfg := &config.Config{
		Settings: config.Settings{
			WordWrap:   80,
			MaxRetries: 3,
			Quiet:      true,
		},
	}
	c := NewChat(context.Background(), r, cfg, nil, nil, nil, "")
	for _, o := range opts {
		o(c)
	}
	// Simulate a window size so View doesn't short-circuit.
	c.width = 80
	c.height = 24
	c.viewport.Width = 80
	c.viewport.Height = 23
	return c
}

func TestChat_ExitCommand(t *testing.T) {
	c := newTestChat()

	// Type "/exit" and press enter.
	c.input.SetValue("/exit")
	m, cmd := c.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_ = m

	if cmd == nil {
		t.Fatal("expected a command from /exit")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg, got %T", msg)
	}
}

func TestChat_QuitCommand(t *testing.T) {
	c := newTestChat()

	c.input.SetValue("/quit")
	m, cmd := c.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_ = m

	if cmd == nil {
		t.Fatal("expected a command from /quit")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg, got %T", msg)
	}
}

func TestChat_CtrlC_InputState(t *testing.T) {
	c := newTestChat()

	_, cmd := c.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected a command from ctrl+c")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg, got %T", msg)
	}
}

func TestChat_CtrlC_StreamState(t *testing.T) {
	c := newTestChat()
	c.state = chatStreamState

	m, cmd := c.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	chat := m.(*Chat)
	if chat.state != chatInputState {
		t.Errorf("expected chatInputState, got %d", chat.state)
	}
	// Should not quit, just cancel stream.
	if cmd != nil {
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); ok {
			t.Error("ctrl+c during streaming should not quit")
		}
	}
}

func TestChat_EmptyInput_Ignored(t *testing.T) {
	c := newTestChat()

	c.input.SetValue("")
	m, cmd := c.Update(tea.KeyMsg{Type: tea.KeyEnter})
	chat := m.(*Chat)
	if chat.state != chatInputState {
		t.Errorf("expected state to remain chatInputState, got %d", chat.state)
	}
	if cmd != nil {
		t.Error("expected no command for empty input")
	}
}

func TestChat_WhitespaceInput_Ignored(t *testing.T) {
	c := newTestChat()

	c.input.SetValue("   ")
	m, cmd := c.Update(tea.KeyMsg{Type: tea.KeyEnter})
	chat := m.(*Chat)
	if chat.state != chatInputState {
		t.Errorf("expected state to remain chatInputState, got %d", chat.state)
	}
	if cmd != nil {
		t.Error("expected no command for whitespace input")
	}
}

func TestChat_SubmitInput_TransitionsToStream(t *testing.T) {
	c := newTestChat()

	// Simulate receiving a submit message.
	m, cmd := c.Update(chatSubmitMsg{prompt: "hello"})
	chat := m.(*Chat)

	if chat.state != chatStreamState {
		t.Errorf("expected chatStreamState, got %d", chat.state)
	}
	if cmd == nil {
		t.Fatal("expected a command to start streaming")
	}
}

func TestChat_FinishTurn_CallsSaveFn(t *testing.T) {
	saved := false
	c := newTestChat(func(c *Chat) {
		c.saveFn = func(msgs []proto.Message) error {
			saved = true
			return nil
		}
	})

	c.streamBuf.WriteString("response text")
	c.history = []proto.Message{
		{Role: proto.RoleUser, Content: "hello"},
		{Role: proto.RoleAssistant, Content: "response text"},
	}
	c.finishTurn()

	if !saved {
		t.Error("expected saveFn to be called")
	}
	if c.streamBuf.Len() != 0 {
		t.Error("expected streamBuf to be reset after finishTurn")
	}
}

func TestChat_StreamDone_ReturnsToInput(t *testing.T) {
	c := newTestChat()
	c.state = chatStreamState

	msgs := []proto.Message{
		{Role: proto.RoleUser, Content: "hi"},
		{Role: proto.RoleAssistant, Content: "hello"},
	}

	m, _ := c.Update(chatStreamDoneMsg{messages: msgs})
	chat := m.(*Chat)

	if chat.state != chatInputState {
		t.Errorf("expected chatInputState after stream done, got %d", chat.state)
	}
	if len(chat.history) != 2 {
		t.Errorf("expected history length 2, got %d", len(chat.history))
	}
}

func TestChat_InitialPrompt(t *testing.T) {
	c := newTestChat(func(c *Chat) {
		c.initialPrompt = "hello world"
	})

	cmd := c.Init()
	if cmd == nil {
		t.Fatal("expected init command")
	}
}

func TestChat_ViewShowsWaitingStatusBeforeFirstChunk(t *testing.T) {
	c := newTestChat()
	c.state = chatStreamState
	c.waitingSince = time.Now().Add(-3 * time.Second)
	c.historyBuf.WriteString("> hi\n\n")
	c.refreshViewport()

	v := c.View()
	if !strings.Contains(v, "Waiting for response...") {
		t.Fatalf("expected waiting status in view, got: %q", v)
	}
}

func TestChat_WaitingStatusIncludesElapsedClock(t *testing.T) {
	c := newTestChat()
	now := time.Date(2026, time.February, 16, 12, 0, 0, 0, time.UTC)
	c.waitingSince = now.Add(-(1*time.Minute + 15*time.Second))

	status := c.waitingStatus(now)
	if !strings.Contains(status, "[01:15]") {
		t.Fatalf("expected stopwatch in waiting status, got: %q", status)
	}
}
