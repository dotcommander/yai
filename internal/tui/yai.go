package tui

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"

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

type state int

const (
	startState state = iota
	requestState
	responseState
	doneState
	errorState
)

// Yai is the Bubble Tea model that manages reading stdin and streaming LLM output.
type Yai struct {
	// Output is populated at the end of a run for non-raw output printing.
	Output string
	Input  string
	Styles present.Styles
	Error  *errs.Error

	state        state
	retries      int
	renderer     *lipgloss.Renderer
	glam         *glamour.TermRenderer
	glamViewport viewport.Model
	glamOutput   string
	glamHeight   int
	messages     []proto.Message
	anim         tea.Model
	width        int
	height       int

	Config        *config.Config
	agent         *agent.Service
	startStreamFn func(context.Context, string) (agent.StreamStart, error)

	content      []string
	contentMutex *sync.Mutex

	outputBuf       bytes.Buffer
	outputTruncated bool
	activeStream    stream.Stream
	activeCancel    context.CancelFunc

	renderScheduled bool
	dirtyOutput     bool
	stopWarned      bool
	mcpNonTTYWarned bool
	streamStartedAt time.Time

	ctx context.Context
}

// NewYai creates the Bubble Tea model used for interactive streaming output.
func NewYai(
	ctx context.Context,
	r *lipgloss.Renderer,
	cfg *config.Config,
	agentSvc *agent.Service,
	startStreamFn func(context.Context, string) (agent.StreamStart, error),
) *Yai {
	gr, _ := glamour.NewTermRenderer(
		glamour.WithEnvironmentConfig(),
		glamour.WithWordWrap(cfg.WordWrap),
	)
	vp := viewport.New(0, 0)
	vp.GotoBottom()
	// agentSvc must be provided by the caller so that the TUI stays focused on
	// rendering and streaming (no config resolution, cache wiring, etc.).
	return &Yai{
		Styles:        present.MakeStyles(r),
		glam:          gr,
		state:         startState,
		renderer:      r,
		glamViewport:  vp,
		contentMutex:  &sync.Mutex{},
		startStreamFn: startStreamFn,
		Config:        cfg,
		agent:         agentSvc,
		ctx:           ctx,
	}
}

// completionInput is a tea.Msg that wraps the content read from stdin.
type completionInput struct {
	content string
}

// completionOutput a tea.Msg that wraps the content returned from the provider.
type completionOutput struct {
	content string
	stream  stream.Stream
	errh    func(error) tea.Msg
}

type renderOutputMsg struct{}

// Init implements tea.Model.
func (m *Yai) Init() tea.Cmd {
	cmds := []tea.Cmd{m.readStdinCmd}
	if !m.Config.Quiet {
		m.anim = newAnim(m.Config.Fanciness, m.Config.StatusText, m.renderer, m.Styles)
		cmds = append(cmds, m.anim.Init())
	}
	return tea.Batch(cmds...)
}

// Update implements tea.Model.
func (m *Yai) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case completionInput:
		if msg.content != "" {
			m.Input = present.RemoveWhitespace(msg.content)
		}
		if m.Input == "" && m.Config.Prefix == "" {
			return m, m.quit
		}

		if m.Config.IncludePromptArgs {
			m.appendToOutput(m.Config.Prefix + "\n\n")
		}

		if m.Config.IncludePrompt > 0 {
			parts := strings.Split(m.Input, "\n")
			if len(parts) > m.Config.IncludePrompt {
				parts = parts[0:m.Config.IncludePrompt]
			}
			m.appendToOutput(strings.Join(parts, "\n") + "\n")
		}
		m.state = requestState
		cmds = append(cmds, m.startCompletionCmd(msg.content))

	case completionOutput:
		if msg.stream == nil {
			m.Output = m.outputBuf.String()
			if !present.IsOutputTTY() || m.Config.Raw {
				m.flushBufferedContent()
			}
			if m.shouldRenderFormattedOutput() && m.dirtyOutput {
				m.renderFormattedOutput()
			}
			m.state = doneState
			return m, m.quit
		}
		if msg.content != "" {
			if m.state == requestState && !m.streamStartedAt.IsZero() && !m.Config.Quiet {
				ttft := time.Since(m.streamStartedAt)
				fmt.Fprintln(os.Stderr, m.Styles.Comment.Render(fmt.Sprintf(ttftFormat, ttft.Milliseconds())))
			}
			m.appendToOutput(msg.content)
			m.state = responseState
			if m.shouldRenderFormattedOutput() && m.dirtyOutput && !m.renderScheduled {
				m.renderScheduled = true
				cmds = append(cmds, m.renderOutputCmd())
			}
		}
		cmds = append(cmds, m.receiveCompletionStreamCmd(completionOutput{
			stream: msg.stream,
			errh:   msg.errh,
		}))

	case renderOutputMsg:
		m.renderScheduled = false
		if m.dirtyOutput {
			m.renderFormattedOutput()
		}

	case errs.Error:
		e := msg
		m.Error = &e
		m.state = errorState
		return m, m.quit
	case error:
		e := errs.Error{Err: msg}
		m.Error = &e
		m.state = errorState
		return m, m.quit

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.glamViewport.Width = m.width
		m.glamViewport.Height = m.height
		if m.shouldRenderFormattedOutput() && m.outputBuf.Len() > 0 {
			m.renderFormattedOutput()
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.closeActiveStream()
			m.state = doneState
			return m, m.quit
		}
	}
	if !m.Config.Quiet && m.state == requestState {
		var cmd tea.Cmd
		m.anim, cmd = m.anim.Update(msg)
		cmds = append(cmds, cmd)
	}
	if m.viewportNeeded() {
		// Only respond to keypresses when the viewport (i.e. the content) is
		// taller than the window.
		var cmd tea.Cmd
		m.glamViewport, cmd = m.glamViewport.Update(msg)
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

func (m Yai) viewportNeeded() bool {
	return m.glamHeight > m.height
}

// View implements tea.Model.
func (m *Yai) View() string {
	//nolint:exhaustive
	switch m.state {
	case errorState:
		return ""
	case requestState:
		if !m.Config.Quiet {
			return m.anim.View()
		}
	case responseState:
		if !m.Config.Raw && present.IsOutputTTY() {
			if m.viewportNeeded() {
				return m.glamViewport.View()
			}
			// We don't need the viewport yet.
			return m.glamOutput
		}

		if present.IsOutputTTY() && !m.Config.Raw {
			return m.Output
		}

		m.flushBufferedContent()
	case doneState:
		if !present.IsOutputTTY() {
			fmt.Printf("\n")
		}
		return ""
	}
	return ""
}

func (m *Yai) quit() tea.Msg {
	return tea.Quit()
}

func (m *Yai) retry(content string, err errs.Error) tea.Msg {
	m.retries++
	if m.retries >= m.Config.MaxRetries {
		return err
	}
	waitForRetryDelay(m.ctx, m.retries, err.Err)
	return completionInput{content}
}

func (m *Yai) startCompletionCmd(content string) tea.Cmd {
	return func() tea.Msg {
		if m.agent == nil {
			return errs.Error{Reason: "Agent is not available"}
		}
		if m.startStreamFn == nil {
			return errs.Error{Reason: "Stream starter is not available"}
		}
		m.streamStartedAt = time.Now()
		res, err := startManagedStream(
			m.ctx,
			m.Config.RequestTimeout,
			m.closeActiveStream,
			func(cancel context.CancelFunc) { m.activeCancel = cancel },
			func(st stream.Stream) { m.activeStream = st },
			func(ctx context.Context) (agent.StreamStart, error) {
				return m.startStreamFn(ctx, content)
			},
		)
		if err != nil {
			return streamStartErrorMsg(err)
		}
		m.messages = res.Messages
		mod := res.Model

		warnIgnoredStop(m.Config.Stop, m.Config.Quiet, &m.stopWarned, m.emitWarning)
		warnMCPDisabledForNonTTY(m.Config, &m.mcpNonTTYWarned, m.emitWarning)

		return m.receiveCompletionStreamCmd(completionOutput{stream: res.Stream, errh: func(err error) tea.Msg {
			return m.handleStreamError(err, mod, m.Input)
		}})()
	}
}

func (m *Yai) receiveCompletionStreamCmd(msg completionOutput) tea.Cmd {
	return receiveManagedStreamCmd(
		msg.stream,
		m.Config.Quiet,
		m.emitWarning,
		m.closeActiveStream,
		msg.errh,
		func(content string, st stream.Stream, errh func(error) tea.Msg) tea.Msg {
			return completionOutput{content: content, stream: st, errh: errh}
		},
		func(messages []proto.Message) tea.Msg {
			m.messages = messages
			return completionOutput{errh: msg.errh}
		},
	)
}

func (m *Yai) readStdinCmd() tea.Msg {
	if !present.IsInputTTY() {
		reader := io.Reader(bufio.NewReader(os.Stdin))
		if !m.Config.NoLimit && m.Config.MaxInputChars > 0 {
			// Read at most MaxInputChars bytes (+1 sentinel) so we never OOM on huge pipes.
			reader = io.LimitReader(reader, m.Config.MaxInputChars+1)
		}
		stdinBytes, err := io.ReadAll(reader)
		if err != nil {
			return errs.Wrap(err, "Unable to read stdin.")
		}
		if !m.Config.NoLimit && m.Config.MaxInputChars > 0 && int64(len(stdinBytes)) > m.Config.MaxInputChars {
			stdinBytes = stdinBytes[:m.Config.MaxInputChars]
		}

		return completionInput{increaseIndent(string(stdinBytes))}
	}
	return completionInput{""}
}

const tabWidth = 4

func (m *Yai) closeActiveStream() {
	closeStream(m.activeStream, m.activeCancel)
	m.activeStream = nil
	m.activeCancel = nil
}

func (m *Yai) emitWarning(message string) {
	emitCommentWarning(m.Styles.Comment.Render, message)
}

func (m *Yai) outputStringForRender() string {
	if m.outputBuf.Len() == 0 {
		return ""
	}
	out := m.outputBuf.String()
	if m.outputTruncated {
		return "[output truncated]\n\n" + out
	}
	return out
}

func (m *Yai) appendToOutput(s string) {
	if !present.IsOutputTTY() || m.Config.Raw {
		m.contentMutex.Lock()
		m.content = append(m.content, s)
		m.contentMutex.Unlock()
		return
	}

	_, _ = m.outputBuf.WriteString(s)
	maxBytes := int(m.Config.MaxOutputBytes)
	if maxBytes > 0 && m.outputBuf.Len() > maxBytes {
		b := m.outputBuf.Bytes()
		if len(b) > maxBytes {
			if !m.outputTruncated && !m.Config.Quiet {
				fmt.Fprintf(os.Stderr, "Warning: output exceeds %d bytes, showing tail only.\n", maxBytes)
			}
			keep := append([]byte(nil), b[len(b)-maxBytes:]...)
			m.outputBuf.Reset()
			_, _ = m.outputBuf.Write(keep)
			m.outputTruncated = true
		}
	}
	m.dirtyOutput = true
}

func (m *Yai) flushBufferedContent() {
	m.contentMutex.Lock()
	defer m.contentMutex.Unlock()
	for _, c := range m.content {
		fmt.Print(c)
	}
	m.content = []string{}
}

func (m Yai) shouldRenderFormattedOutput() bool {
	return present.IsOutputTTY() && !m.Config.Raw
}

func (m *Yai) renderOutputCmd() tea.Cmd {
	return tea.Tick(adaptiveRenderInterval(m.outputBuf.Len()), func(time.Time) tea.Msg {
		return renderOutputMsg{}
	})
}

func (m *Yai) renderFormattedOutput() {
	wasAtBottom := m.glamViewport.ScrollPercent() == 1.0
	oldHeight := m.glamHeight
	m.glamOutput, _ = m.glam.Render(m.outputStringForRender())
	m.glamOutput = strings.TrimRightFunc(m.glamOutput, unicode.IsSpace)
	m.glamOutput = strings.ReplaceAll(m.glamOutput, "\t", strings.Repeat(" ", tabWidth))
	m.glamHeight = lipgloss.Height(m.glamOutput)
	m.glamOutput += "\n"
	truncatedGlamOutput := m.renderer.NewStyle().
		MaxWidth(m.width).
		Render(m.glamOutput)
	m.glamViewport.SetContent(truncatedGlamOutput)
	if oldHeight < m.glamHeight && wasAtBottom {
		// If the viewport's at the bottom and we've received a new
		// line of content, follow the output by auto scrolling to
		// the bottom.
		m.glamViewport.GotoBottom()
	}
	m.dirtyOutput = false
}

func increaseIndent(s string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = "\t" + lines[i]
	}
	return strings.Join(lines, "\n")
}
