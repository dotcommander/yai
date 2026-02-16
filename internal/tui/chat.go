package tui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"

	"charm.land/fantasy"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/dotcommander/yai/internal/agent"
	"github.com/dotcommander/yai/internal/config"
	"github.com/dotcommander/yai/internal/errs"
	"github.com/dotcommander/yai/internal/present"
	"github.com/dotcommander/yai/internal/proto"
	"github.com/dotcommander/yai/internal/stream"
)

type chatState int

const (
	chatInputState chatState = iota
	chatStreamState
)

// SaveFn persists conversation messages after each turn.
type SaveFn func([]proto.Message) error

// Chat is the Bubble Tea model for an interactive multi-turn REPL.
type Chat struct {
	Error *errs.Error

	state    chatState
	input    textinput.Model
	viewport viewport.Model
	glam     *glamour.TermRenderer
	renderer *lipgloss.Renderer
	styles   present.Styles
	anim     tea.Model

	history      []proto.Message
	historyBuf   bytes.Buffer // rendered conversation so far
	streamBuf    bytes.Buffer // current response being streamed
	activeStream stream.Stream
	activeCancel context.CancelFunc

	agent  *agent.Service
	saveFn SaveFn
	cfg    *config.Config
	ctx    context.Context

	width  int
	height int

	renderScheduled bool
	dirtyOutput     bool
	stopWarned      bool
	retries         int
	initialPrompt   string
	waitingSince    time.Time
}

// NewChat creates the Bubble Tea model for interactive chat.
func NewChat(
	ctx context.Context,
	r *lipgloss.Renderer,
	cfg *config.Config,
	agentSvc *agent.Service,
	history []proto.Message,
	saveFn SaveFn,
	initialPrompt string,
) *Chat {
	gr, _ := glamour.NewTermRenderer(
		glamour.WithEnvironmentConfig(),
		glamour.WithWordWrap(cfg.WordWrap),
	)

	ti := textinput.New()
	ti.Prompt = "yai> "
	ti.Focus()
	ti.CharLimit = 0

	vp := viewport.New(0, 0)
	vp.GotoBottom()

	c := &Chat{
		state:         chatInputState,
		input:         ti,
		viewport:      vp,
		glam:          gr,
		renderer:      r,
		styles:        present.MakeStyles(r),
		agent:         agentSvc,
		saveFn:        saveFn,
		cfg:           cfg,
		ctx:           ctx,
		history:       history,
		initialPrompt: initialPrompt,
	}

	// Pre-render existing history into historyBuf.
	if len(history) > 0 {
		for _, msg := range history {
			if msg.Role == proto.RoleSystem || msg.Content == "" {
				continue
			}
			switch msg.Role {
			case proto.RoleUser:
				fmt.Fprintf(&c.historyBuf, "> %s\n\n", msg.Content)
			case proto.RoleAssistant:
				fmt.Fprintf(&c.historyBuf, "%s\n\n", msg.Content)
			}
		}
	}

	return c
}

// chatSubmitMsg is sent when the user presses Enter with non-empty input.
type chatSubmitMsg struct {
	prompt string
}

// chatStreamChunkMsg wraps a chunk of streaming response.
type chatStreamChunkMsg struct {
	content string
	stream  stream.Stream
	errh    func(error) tea.Msg
}

// chatStreamDoneMsg signals the stream is complete.
type chatStreamDoneMsg struct {
	messages []proto.Message
}

type chatRenderMsg struct{}

type chatWaitingTickMsg struct{}

// Init implements tea.Model.
func (c *Chat) Init() tea.Cmd {
	cmds := []tea.Cmd{textinput.Blink}
	if !c.cfg.Quiet {
		c.anim = newAnim(c.cfg.Fanciness, c.cfg.StatusText, c.renderer, c.styles)
		cmds = append(cmds, c.anim.Init())
	}
	if c.initialPrompt != "" {
		cmds = append(cmds, func() tea.Msg {
			return chatSubmitMsg{prompt: c.initialPrompt}
		})
	}
	return tea.Batch(cmds...)
}

// Update implements tea.Model.
func (c *Chat) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		c.width = msg.Width
		c.height = msg.Height
		c.resizeViewport()
		c.refreshViewport()
		return c, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			if c.state == chatStreamState {
				c.closeActiveStream()
				c.waitingSince = time.Time{}
				c.finishTurn()
				c.state = chatInputState
				c.resizeViewport()
				return c, nil
			}
			return c, tea.Quit
		case "enter":
			if c.state != chatInputState {
				break
			}
			text := strings.TrimSpace(c.input.Value())
			if text == "" {
				return c, nil
			}
			if text == "/exit" || text == "/quit" {
				return c, tea.Quit
			}
			c.input.SetValue("")
			return c, func() tea.Msg {
				return chatSubmitMsg{prompt: text}
			}
		}

	case chatSubmitMsg:
		c.retries = 0
		fmt.Fprintf(&c.historyBuf, "> %s\n\n", msg.prompt)
		c.streamBuf.Reset()
		c.waitingSince = time.Now()
		c.state = chatStreamState
		c.resizeViewport()
		c.dirtyOutput = true
		c.refreshViewport()
		return c, tea.Batch(c.startStreamCmd(msg.prompt), c.waitingTickCmd())

	case chatStreamChunkMsg:
		if msg.stream == nil {
			// Stream complete.
			return c, nil
		}
		if msg.content != "" {
			c.waitingSince = time.Time{}
			c.streamBuf.WriteString(msg.content)
			c.resizeViewport()
			c.dirtyOutput = true
			if !c.renderScheduled {
				c.renderScheduled = true
				cmds = append(cmds, c.renderTickCmd())
			}
		}
		cmds = append(cmds, c.receiveStreamCmd(chatStreamChunkMsg{
			stream: msg.stream,
			errh:   msg.errh,
		}))
		return c, tea.Batch(cmds...)

	case chatStreamDoneMsg:
		c.history = msg.messages
		c.waitingSince = time.Time{}
		c.finishTurn()
		c.state = chatInputState
		c.resizeViewport()
		c.refreshViewport()
		return c, nil

	case chatWaitingTickMsg:
		if c.state == chatStreamState && c.streamBuf.Len() == 0 {
			return c, c.waitingTickCmd()
		}
		return c, nil

	case chatRenderMsg:
		c.renderScheduled = false
		if c.dirtyOutput {
			c.refreshViewport()
		}
		return c, nil

	case errs.Error:
		e := msg
		c.Error = &e
		return c, tea.Quit

	case error:
		e := errs.Error{Err: msg}
		c.Error = &e
		return c, tea.Quit
	}

	// Update sub-models.
	if c.state == chatInputState {
		var cmd tea.Cmd
		c.input, cmd = c.input.Update(msg)
		cmds = append(cmds, cmd)
	}
	if c.state == chatStreamState && !c.cfg.Quiet && c.anim != nil {
		var cmd tea.Cmd
		c.anim, cmd = c.anim.Update(msg)
		cmds = append(cmds, cmd)
	}

	var cmd tea.Cmd
	c.viewport, cmd = c.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return c, tea.Batch(cmds...)
}

// View implements tea.Model.
func (c *Chat) View() string {
	if c.width == 0 || c.height == 0 {
		return ""
	}

	divider := c.styles.Comment.Render(strings.Repeat("─", max(c.width, 1)))

	var content string
	if c.state == chatStreamState && c.streamBuf.Len() == 0 {
		status := c.waitingStatus(time.Now())
		if !c.cfg.Quiet && c.anim != nil {
			// Show explicit waiting status plus animation while waiting for first chunk.
			content = c.viewport.View() + "\n" + divider + "\n" + status + "\n" + c.anim.View()
		} else {
			content = c.viewport.View() + "\n" + divider + "\n" + status
		}
	} else {
		content = c.viewport.View() + "\n" + divider + "\n" + c.input.View()
	}

	return content
}

// Messages returns the current conversation history.
func (c *Chat) Messages() []proto.Message {
	return c.history
}

func (c *Chat) startStreamCmd(prompt string) tea.Cmd {
	return func() tea.Msg {
		if c.agent == nil {
			return errs.Error{Reason: "Agent is not available"}
		}
		c.closeActiveStream()

		ctx := c.ctx
		if c.cfg.RequestTimeout > 0 {
			cctx, cancel := context.WithTimeout(c.ctx, c.cfg.RequestTimeout)
			ctx = cctx
			c.activeCancel = cancel
		}

		res, err := c.agent.StreamContinue(ctx, c.history, prompt)
		if err != nil {
			c.closeActiveStream()
			var e errs.Error
			if errors.As(err, &e) {
				return e
			}
			return errs.Error{Err: err}
		}

		c.activeStream = res.Stream
		mod := res.Model

		if len(c.cfg.Stop) > 0 && !c.cfg.Quiet && !c.stopWarned {
			fmt.Fprintln(os.Stderr, c.styles.Comment.Render("Warning: stop sequences are currently ignored by the Fantasy bridge."))
			c.stopWarned = true
		}

		return c.receiveStreamCmd(chatStreamChunkMsg{
			stream: res.Stream,
			errh: func(err error) tea.Msg {
				return c.handleStreamError(err, mod, prompt)
			},
		})()
	}
}

func (c *Chat) receiveStreamCmd(msg chatStreamChunkMsg) tea.Cmd {
	return func() tea.Msg {
		if msg.stream.Next() {
			chunk, err := msg.stream.Current()
			if err != nil && !errors.Is(err, stream.ErrNoContent) {
				_ = msg.stream.Close()
				return msg.errh(err)
			}
			return chatStreamChunkMsg{
				content: chunk.Content,
				stream:  msg.stream,
				errh:    msg.errh,
			}
		}

		if err := msg.stream.Err(); err != nil {
			c.closeActiveStream()
			return msg.errh(err)
		}

		if !c.cfg.Quiet {
			for _, warning := range msg.stream.DrainWarnings() {
				fmt.Fprintln(os.Stderr, c.styles.Comment.Render("Warning: "+warning))
			}
		}

		results := msg.stream.CallTools()
		if len(results) > 0 {
			toolMsg := chatStreamChunkMsg{
				stream: msg.stream,
				errh:   msg.errh,
			}
			for _, call := range results {
				toolMsg.content += call.String()
			}
			return toolMsg
		}

		messages := msg.stream.Messages()
		c.closeActiveStream()
		return chatStreamDoneMsg{messages: messages}
	}
}

func (c *Chat) handleStreamError(err error, mod config.Model, prompt string) tea.Msg {
	var providerErr *fantasy.ProviderError
	if errors.As(err, &providerErr) && providerErr.IsRetryable() {
		c.retries++
		if c.retries < c.cfg.MaxRetries {
			c.waitForRetryDelay(err)
			return chatSubmitMsg{prompt: prompt}
		}
	}
	var e errs.Error
	if errors.As(err, &e) {
		return e
	}
	return errs.Error{Err: err}
}

func (c *Chat) waitForRetryDelay(retryErr error) {
	var providerErr *fantasy.ProviderError
	if !errors.As(retryErr, &providerErr) {
		return
	}
	opts := fantasy.DefaultRetryOptions()
	opts.MaxRetries = 1
	opts.InitialDelayIn = 100 * time.Millisecond
	retryFn := fantasy.RetryWithExponentialBackoffRespectingRetryHeaders[struct{}](opts)
	_, _ = retryFn(c.ctx, func() (struct{}, error) {
		return struct{}{}, providerErr
	})
}

func (c *Chat) finishTurn() {
	// Move streamed response into history buffer.
	if c.streamBuf.Len() > 0 {
		fmt.Fprintf(&c.historyBuf, "%s\n\n", c.streamBuf.String())
		c.streamBuf.Reset()
	}
	c.dirtyOutput = true

	// Persist to cache.
	if c.saveFn != nil {
		if err := c.saveFn(c.history); err != nil {
			fmt.Fprintln(os.Stderr, c.styles.Comment.Render("Warning: failed to save conversation: "+err.Error()))
		}
	}
}

func (c *Chat) closeActiveStream() {
	if c.activeStream != nil {
		_ = c.activeStream.Close()
		c.activeStream = nil
	}
	if c.activeCancel != nil {
		c.activeCancel()
		c.activeCancel = nil
	}
}

func (c *Chat) refreshViewport() {
	combined := c.historyBuf.String() + c.streamBuf.String()
	if combined == "" {
		return
	}

	rendered, err := c.glam.Render(combined)
	if err != nil {
		rendered = combined
	}
	rendered = strings.TrimRightFunc(rendered, unicode.IsSpace)
	rendered += "\n"

	truncated := c.renderer.NewStyle().MaxWidth(c.width).Render(rendered)

	wasAtBottom := c.viewport.ScrollPercent() >= 1.0
	c.viewport.SetContent(truncated)
	if wasAtBottom {
		c.viewport.GotoBottom()
	}
	c.dirtyOutput = false
}

func (c *Chat) renderTickCmd() tea.Cmd {
	const renderInterval = 33 * time.Millisecond
	return tea.Tick(renderInterval, func(time.Time) tea.Msg {
		return chatRenderMsg{}
	})
}

func (c *Chat) waitingTickCmd() tea.Cmd {
	const waitingInterval = 200 * time.Millisecond
	return tea.Tick(waitingInterval, func(time.Time) tea.Msg {
		return chatWaitingTickMsg{}
	})
}

func (c *Chat) footerLineCount() int {
	if c.state == chatStreamState && c.streamBuf.Len() == 0 {
		if !c.cfg.Quiet && c.anim != nil {
			return 3
		}
		return 2
	}
	return 2
}

func (c *Chat) resizeViewport() {
	if c.width > 0 {
		c.viewport.Width = c.width
	}
	h := c.height - c.footerLineCount()
	if h < 1 {
		h = 1
	}
	c.viewport.Height = h
}

func (c *Chat) waitingStatus(now time.Time) string {
	if c.waitingSince.IsZero() {
		return c.styles.Comment.Render("Waiting for response...")
	}

	elapsed := now.Sub(c.waitingSince)
	if elapsed < 0 {
		elapsed = 0
	}

	return c.styles.Comment.Render("Waiting for response... [" + formatElapsedClock(elapsed) + "]")
}

func formatElapsedClock(d time.Duration) string {
	totalSeconds := int(d / time.Second)
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60

	if hours > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
	}
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}
