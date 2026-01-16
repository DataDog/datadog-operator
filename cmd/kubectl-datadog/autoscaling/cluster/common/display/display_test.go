package display

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no ANSI codes",
			input:    "Hello World",
			expected: "Hello World",
		},
		{
			name:     "single color code",
			input:    "\x1b[31mRed\x1b[0m",
			expected: "Red",
		},
		{
			name:     "multiple color codes",
			input:    "\x1b[1;31;40mBold Red on Black\x1b[0m",
			expected: "Bold Red on Black",
		},
		{
			name:     "mixed text and codes",
			input:    "Start \x1b[32mGreen\x1b[0m End",
			expected: "Start Green End",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripANSI(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestVisualWidth(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "ASCII only",
			input:    "Hello",
			expected: 5,
		},
		{
			name:     "with ANSI codes",
			input:    "\x1b[31mHello\x1b[0m",
			expected: 5,
		},
		{
			name:     "with emoji",
			input:    "Hi 🎉",
			expected: 5, // "Hi " = 3, emoji = 2
		},
		{
			name:     "CJK characters",
			input:    "日本語",
			expected: 6, // each CJK char = 2
		},
		{
			name:     "mixed",
			input:    "\x1b[34m日本\x1b[0m OK",
			expected: 7, // 日本 = 4, " OK" = 3
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := visualWidth(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPrintBox(t *testing.T) {
	tests := []struct {
		name     string
		lines    []string
		expected string
	}{
		{
			name:  "single line",
			lines: []string{"Hello"},
			expected: "" +
				"╭───────╮\n" +
				"│ Hello │\n" +
				"╰───────╯\n",
		},
		{
			name:  "multiple lines same length",
			lines: []string{"Hello", "World"},
			expected: "" +
				"╭───────╮\n" +
				"│ Hello │\n" +
				"│ World │\n" +
				"╰───────╯\n",
		},
		{
			name:  "multiple lines different lengths",
			lines: []string{"Hi", "Hello", "Hey"},
			expected: "" +
				"╭───────╮\n" +
				"│ Hi    │\n" +
				"│ Hello │\n" +
				"│ Hey   │\n" +
				"╰───────╯\n",
		},
		{
			name:  "empty line",
			lines: []string{"Hello", "", "World"},
			expected: "" +
				"╭───────╮\n" +
				"│ Hello │\n" +
				"│       │\n" +
				"│ World │\n" +
				"╰───────╯\n",
		},
		{
			name:  "line with ANSI color codes",
			lines: []string{"\x1b[31mRed\x1b[0m", "Normal"},
			expected: "" +
				"╭────────╮\n" +
				"│ \x1b[31mRed\x1b[0m    │\n" +
				"│ Normal │\n" +
				"╰────────╯\n",
		},
		{
			name:  "line with emoji",
			lines: []string{"Hello 🎉", "World"},
			expected: "" +
				"╭──────────╮\n" +
				"│ Hello 🎉 │\n" +
				"│ World    │\n" +
				"╰──────────╯\n",
		},
		{
			name:  "line with wide CJK characters",
			lines: []string{"日本語", "Hello"},
			expected: "" +
				"╭────────╮\n" +
				"│ 日本語 │\n" +
				"│ Hello  │\n" +
				"╰────────╯\n",
		},
		{
			name:  "mixed ANSI and emoji",
			lines: []string{"\x1b[1;34mBlue 🔵\x1b[0m", "Normal text"},
			expected: "" +
				"╭─────────────╮\n" +
				"│ \x1b[1;34mBlue 🔵\x1b[0m     │\n" +
				"│ Normal text │\n" +
				"╰─────────────╯\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			PrintBox(&buf, tt.lines...)
			assert.Equal(t, tt.expected, buf.String())
		})
	}
}
