package tui

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dotcommander/yai/internal/proto"
	"github.com/dotcommander/yai/internal/stream"
	"github.com/stretchr/testify/require"
)

type fakeStream struct {
	next       bool
	chunk      proto.Chunk
	currentErr error
	err        error
	messages   []proto.Message
	tools      []proto.ToolCallStatus
	warnings   []string
	closed     bool
}

func (f *fakeStream) Next() bool                        { return f.next }
func (f *fakeStream) Current() (proto.Chunk, error)     { return f.chunk, f.currentErr }
func (f *fakeStream) Close() error                      { f.closed = true; return nil }
func (f *fakeStream) Err() error                        { return f.err }
func (f *fakeStream) Messages() []proto.Message         { return f.messages }
func (f *fakeStream) CallTools() []proto.ToolCallStatus { return f.tools }
func (f *fakeStream) DrainWarnings() []string           { out := f.warnings; f.warnings = nil; return out }

func TestReceiveManagedStreamCmdReturnsToolOutput(t *testing.T) {
	st := &fakeStream{tools: []proto.ToolCallStatus{{Name: "demo"}}}
	msg := receiveManagedStreamCmd(
		st,
		false,
		func(string) {},
		func() {},
		func(err error) tea.Msg { return err },
		func(content string, st stream.Stream, errh func(error) tea.Msg) tea.Msg {
			return completionOutput{content: content, stream: st, errh: errh}
		},
		func([]proto.Message) tea.Msg { return completionOutput{} },
	)()

	out, ok := msg.(completionOutput)
	require.True(t, ok)
	require.Contains(t, out.content, "Ran tool")
	require.Contains(t, out.content, "demo")
}

func TestReceiveManagedStreamCmdClosesOnStreamError(t *testing.T) {
	st := &fakeStream{err: errors.New("boom")}
	closed := false
	msg := receiveManagedStreamCmd(
		st,
		true,
		func(string) {},
		func() { closed = true },
		func(err error) tea.Msg { return err },
		func(content string, st stream.Stream, errh func(error) tea.Msg) tea.Msg { return nil },
		func([]proto.Message) tea.Msg { return nil },
	)()

	require.EqualError(t, msg.(error), "boom")
	require.True(t, closed)
}

func TestWarnIgnoredStopOnlyOnce(t *testing.T) {
	warned := false
	var messages []string
	emit := func(message string) { messages = append(messages, message) }

	warnIgnoredStop([]string{"done"}, false, &warned, emit)
	warnIgnoredStop([]string{"done"}, false, &warned, emit)

	require.Equal(t, []string{"stop sequences are currently ignored by the Fantasy bridge."}, messages)
}
