package tui

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"

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

	history         []proto.Message
	historyBuf      bytes.Buffer // rendered conversation so far
	renderedHistory string       // Glamour-rendered cache of historyBuf
	streamBuf       bytes.Buffer // current response being streamed
	activeStream    stream.Stream
	activeCancel    context.CancelFunc

	agent         *agent.Service
	startStreamFn func(context.Context, []proto.Message, string) (agent.StreamStart, error)
	saveFn        SaveFn
	cfg           *config.Config
	ctx           context.Context

	width  int
	height int

	renderScheduled bool
	dirtyOutput     bool
	stopWarned      bool
	retries         int
	initialPrompt   string
	waitingSince    time.Time
}

type ChatOptions struct {
	Context       context.Context
	Renderer      *lipgloss.Renderer
	Config        *config.Config
	Agent         *agent.Service
	StartStream   func(context.Context, []proto.Message, string) (agent.StreamStart, error)
	History       []proto.Message
	Save          SaveFn
	InitialPrompt string
}

// NewChat creates the Bubble Tea model for interactive chat.
func NewChat(opts ChatOptions) *Chat {
	gr, _ := glamour.NewTermRenderer(
		glamour.WithEnvironmentConfig(),
		glamour.WithWordWrap(opts.Config.WordWrap),
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
		renderer:      opts.Renderer,
		styles:        present.MakeStyles(opts.Renderer),
		agent:         opts.Agent,
		saveFn:        opts.Save,
		cfg:           opts.Config,
		ctx:           opts.Context,
		history:       opts.History,
		startStreamFn: opts.StartStream,
		initialPrompt: opts.InitialPrompt,
	}

	// Pre-render existing history into historyBuf.
	if len(opts.History) > 0 {
		for _, msg := range opts.History {
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
		if c.historyBuf.Len() > 0 {
			rendered, err := gr.Render(c.historyBuf.String())
			if err == nil {
				c.renderedHistory = strings.TrimRightFunc(rendered, unicode.IsSpace)
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
			if !c.waitingSince.IsZero() && c.streamBuf.Len() == 0 && !c.cfg.Quiet {
				ttft := time.Since(c.waitingSince)
				fmt.Fprintln(os.Stderr, c.styles.Comment.Render(fmt.Sprintf(ttftFormat, ttft.Milliseconds())))
			}
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
		if c.startStreamFn == nil {
			return errs.Error{Reason: "Stream starter is not available"}
		}

		res, err := startManagedStream(
			c.ctx,
			c.cfg.RequestTimeout,
			c.closeActiveStream,
			func(cancel context.CancelFunc) { c.activeCancel = cancel },
			func(st stream.Stream) { c.activeStream = st },
			func(ctx context.Context) (agent.StreamStart, error) {
				return c.startStreamFn(ctx, c.history, prompt)
			},
		)
		if err != nil {
			return streamStartErrorMsg(err)
		}
		mod := res.Model

		warnIgnoredStop(c.cfg.Stop, c.cfg.Quiet, &c.stopWarned, c.emitWarning)

		return c.receiveStreamCmd(chatStreamChunkMsg{stream: res.Stream, errh: func(err error) tea.Msg {
			return c.handleStreamError(err, mod, prompt)
		}})()
	}
}

func (c *Chat) receiveStreamCmd(msg chatStreamChunkMsg) tea.Cmd {
	return receiveManagedStreamCmd(
		msg.stream,
		c.cfg.Quiet,
		c.emitWarning,
		c.closeActiveStream,
		msg.errh,
		func(content string, st stream.Stream, errh func(error) tea.Msg) tea.Msg {
			return chatStreamChunkMsg{content: content, stream: st, errh: errh}
		},
		func(messages []proto.Message) tea.Msg {
			return chatStreamDoneMsg{messages: messages}
		},
	)
}

func (c *Chat) handleStreamError(err error, mod config.Model, prompt string) tea.Msg {
	return handleRetryableStreamError(c.agent, c.cfg.NoLimit, func(model string) {
		c.cfg.Model = model
	}, c.retry, err, mod, prompt)
}

func (c *Chat) retry(err errs.Error, content string) tea.Msg {
	c.retries++
	if c.retries >= c.cfg.MaxRetries {
		return err
	}
	waitForRetryDelay(c.ctx, c.retries, err.Err)
	return chatSubmitMsg{prompt: content}
}

func (c *Chat) finishTurn() {
	// Move streamed response into history buffer.
	if c.streamBuf.Len() > 0 {
		fmt.Fprintf(&c.historyBuf, "%s\n\n", c.streamBuf.String())
		c.streamBuf.Reset()
	}
	// Cache rendered history so refreshViewport only renders the stream portion.
	if c.historyBuf.Len() > 0 {
		rendered, err := c.glam.Render(c.historyBuf.String())
		if err == nil {
			c.renderedHistory = strings.TrimRightFunc(rendered, unicode.IsSpace)
		}
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
	closeStream(c.activeStream, c.activeCancel)
	c.activeStream = nil
	c.activeCancel = nil
}

func (c *Chat) emitWarning(message string) {
	emitCommentWarning(c.styles.Comment.Render, message)
}

func (c *Chat) refreshViewport() {
	if c.historyBuf.Len() == 0 && c.streamBuf.Len() == 0 {
		return
	}

	var rendered string
	if c.streamBuf.Len() > 0 {
		streamRendered, err := c.glam.Render(c.streamBuf.String())
		if err != nil {
			streamRendered = c.streamBuf.String()
		}
		streamRendered = strings.TrimRightFunc(streamRendered, unicode.IsSpace)
		if c.renderedHistory != "" {
			rendered = c.renderedHistory + "\n" + streamRendered
		} else {
			rendered = streamRendered
		}
	} else {
		rendered = c.renderedHistory
	}

	if rendered == "" {
		return
	}
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
	return tea.Tick(adaptiveRenderInterval(c.streamBuf.Len()), func(time.Time) tea.Msg {
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
