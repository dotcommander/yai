package tui

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"

	"charm.land/fantasy"
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

	Config *config.Config
	agent  *agent.Service

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

	ctx context.Context
}

// NewYai creates the Bubble Tea model used for interactive streaming output.
func NewYai(
	ctx context.Context,
	r *lipgloss.Renderer,
	cfg *config.Config,
	agentSvc *agent.Service,
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
		Styles:       present.MakeStyles(r),
		glam:         gr,
		state:        startState,
		renderer:     r,
		glamViewport: vp,
		contentMutex: &sync.Mutex{},
		Config:       cfg,
		agent:        agentSvc,
		ctx:          ctx,
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
			m.Input = removeWhitespace(msg.content)
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
	m.waitForFantasyRetryDelay(err.Err)
	return completionInput{content}
}

func (m *Yai) waitForFantasyRetryDelay(retryErr error) {
	var providerErr *fantasy.ProviderError
	if !errors.As(retryErr, &providerErr) {
		return
	}

	opts := fantasy.DefaultRetryOptions()
	opts.MaxRetries = 1
	opts.InitialDelayIn = 100 * time.Millisecond

	retryFn := fantasy.RetryWithExponentialBackoffRespectingRetryHeaders[struct{}](opts)
	_, _ = retryFn(m.ctx, func() (struct{}, error) {
		return struct{}{}, providerErr
	})
}

func (m *Yai) startCompletionCmd(content string) tea.Cmd {
	return func() tea.Msg {
		if m.agent == nil {
			return errs.Error{Reason: "Agent is not available"}
		}
		m.closeActiveStream()
		ctx := m.ctx
		if m.Config.RequestTimeout > 0 {
			cctx, cancel := context.WithTimeout(m.ctx, m.Config.RequestTimeout)
			ctx = cctx
			m.activeCancel = cancel
		}
		res, err := m.agent.Stream(ctx, content)
		if err != nil {
			m.closeActiveStream()
			var e errs.Error
			if errors.As(err, &e) {
				return e
			}
			return errs.Error{Err: err}
		}
		m.activeStream = res.Stream
		m.messages = res.Messages
		mod := res.Model

		cfg := m.Config
		if len(cfg.Stop) > 0 && !cfg.Quiet && !m.stopWarned {
			fmt.Fprintln(os.Stderr, m.Styles.Comment.Render("Warning: stop sequences are currently ignored by the Fantasy bridge (current Fantasy Call API has no stop field)."))
			m.stopWarned = true
		}
		if !cfg.Quiet && !cfg.MCPAllowNonTTY && !present.IsInputTTY() && len(cfg.MCPServers) > 0 && !m.mcpNonTTYWarned {
			fmt.Fprintln(os.Stderr, m.Styles.Comment.Render("Warning: MCP tools are disabled for piped/non-interactive input by default. Use --mcp-allow-non-tty to enable."))
			m.mcpNonTTYWarned = true
		}

		return m.receiveCompletionStreamCmd(completionOutput{
			stream: res.Stream,
			errh: func(err error) tea.Msg {
				return m.handleStreamError(err, mod, m.Input)
			},
		})()
	}
}

func (m *Yai) receiveCompletionStreamCmd(msg completionOutput) tea.Cmd {
	return func() tea.Msg {
		if msg.stream.Next() {
			chunk, err := msg.stream.Current()
			if err != nil && !errors.Is(err, stream.ErrNoContent) {
				_ = msg.stream.Close()
				return msg.errh(err)
			}
			return completionOutput{
				content: chunk.Content,
				stream:  msg.stream,
				errh:    msg.errh,
			}
		}

		// stream is done, check for errors
		if err := msg.stream.Err(); err != nil {
			m.closeActiveStream()
			return msg.errh(err)
		}

		if !m.Config.Quiet {
			for _, warning := range msg.stream.DrainWarnings() {
				fmt.Fprintln(os.Stderr, m.Styles.Comment.Render("Warning: "+warning))
			}
		}

		results := msg.stream.CallTools()
		toolMsg := completionOutput{
			stream: msg.stream,
			errh:   msg.errh,
		}
		for _, call := range results {
			toolMsg.content += call.String()
		}
		if len(results) == 0 {
			m.messages = msg.stream.Messages()
			m.closeActiveStream()
			return completionOutput{errh: msg.errh}
		}
		return toolMsg
	}
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
			return errs.Error{Err: err, Reason: "Unable to read stdin."}
		}
		if !m.Config.NoLimit && m.Config.MaxInputChars > 0 && int64(len(stdinBytes)) > m.Config.MaxInputChars {
			stdinBytes = stdinBytes[:m.Config.MaxInputChars]
		}

		return completionInput{increaseIndent(string(stdinBytes))}
	}
	return completionInput{""}
}

const tabWidth = 4

const maxRetainedOutputBytes = 2 * 1024 * 1024

func (m *Yai) closeActiveStream() {
	if m.activeStream != nil {
		_ = m.activeStream.Close()
		m.activeStream = nil
	}
	if m.activeCancel != nil {
		m.activeCancel()
		m.activeCancel = nil
	}
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
	if m.outputBuf.Len() > maxRetainedOutputBytes {
		b := m.outputBuf.Bytes()
		if len(b) > maxRetainedOutputBytes {
			keep := append([]byte(nil), b[len(b)-maxRetainedOutputBytes:]...)
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
	const renderInterval = 33 * time.Millisecond
	return tea.Tick(renderInterval, func(time.Time) tea.Msg {
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

// if the input is whitespace only, make it empty.
func removeWhitespace(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return s
}

func increaseIndent(s string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = "\t" + lines[i]
	}
	return strings.Join(lines, "\n")
}
