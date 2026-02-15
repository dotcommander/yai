package tui

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/dotcommander/yai/internal/config"
)

func BenchmarkStreamingRenderComparison(b *testing.B) {
	chunks := makeBenchmarkChunks(256)

	b.Run("legacy_render_every_chunk", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			m := newBenchmarkYaiForRender()
			for _, chunk := range chunks {
				m.Output += chunk
				m.renderFormattedOutput()
			}
		}
	})

	b.Run("throttled_render_every_12_chunks", func(b *testing.B) {
		b.ReportAllocs()
		const batchSize = 12
		for i := 0; i < b.N; i++ {
			m := newBenchmarkYaiForRender()
			for j, chunk := range chunks {
				m.Output += chunk
				if (j+1)%batchSize == 0 {
					m.renderFormattedOutput()
				}
			}
			if len(chunks)%batchSize != 0 {
				m.renderFormattedOutput()
			}
		}
	})
}

func newBenchmarkYaiForRender() *Yai {
	r := lipgloss.NewRenderer(io.Discard)
	m := NewYai(context.Background(), r, &config.Config{Settings: config.Settings{WordWrap: 100}}, nil)

	m.width = 120
	m.height = 40
	m.glamViewport.Width = m.width
	m.glamViewport.Height = m.height
	return m
}

func makeBenchmarkChunks(n int) []string {
	chunk := strings.Repeat("x", 32) + "\n- list item\n`code`\n"
	chunks := make([]string, n)
	for i := range chunks {
		chunks[i] = chunk
	}
	return chunks
}
