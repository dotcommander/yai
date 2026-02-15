package tui

import "github.com/dotcommander/yai/internal/proto"

// GlamourOutput returns the last rendered formatted output.
func (m *Yai) GlamourOutput() string {
	return m.glamOutput
}

// Messages returns the message list built/received during streaming.
func (m *Yai) Messages() []proto.Message {
	return m.messages
}
