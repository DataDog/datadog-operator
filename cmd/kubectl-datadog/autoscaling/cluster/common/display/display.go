// Package display provides utilities for formatting text output
// in the kubectl-datadog CLI tool.
package display

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/samber/lo"
)

// ansiEscapeRegex matches ANSI escape sequences (e.g., color codes).
var ansiEscapeRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// stripANSI removes ANSI escape sequences from a string.
func stripANSI(s string) string {
	return ansiEscapeRegex.ReplaceAllString(s, "")
}

// visualWidth returns the display width of a string in terminal columns,
// correctly handling ANSI escape sequences and wide Unicode characters.
func visualWidth(s string) int {
	return runewidth.StringWidth(stripANSI(s))
}

// PrintBox prints text inside a Unicode box to the given writer.
// For multiple lines, all lines are padded to the width of the longest line.
// This function correctly handles ANSI escape sequences and wide Unicode
// characters (such as emojis and East Asian characters).
func PrintBox(w io.Writer, lines ...string) {
	maxWidth := lo.Max(lo.Map(lines, func(line string, _ int) int {
		return visualWidth(line)
	}))

	fmt.Fprintln(w, "╭─"+strings.Repeat("─", maxWidth)+"─╮")
	for _, line := range lines {
		padding := maxWidth - visualWidth(line)
		fmt.Fprintf(w, "│ %s%s │\n", line, strings.Repeat(" ", padding))
	}
	fmt.Fprintln(w, "╰─"+strings.Repeat("─", maxWidth)+"─╯")
}
