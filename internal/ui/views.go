package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"akhilsingh.in/skillctl/internal/config"
)

// --- Color palette ---
var (
	primary   = lipgloss.Color("#22D3EE")
	secondary = lipgloss.Color("#93C5FD")
	accent    = lipgloss.Color("#FACC15")
	success   = lipgloss.Color("#34D399")
	danger    = lipgloss.Color("#F87171")
	muted     = lipgloss.Color("#94A3B8")
	text      = lipgloss.Color("#E2E8F0")
)

// --- Styles ---
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#0F172A")).
			Background(primary).
			Padding(0, 1)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(muted)

	headerDivider = lipgloss.NewStyle().
			Foreground(primary)

	promptLabelStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(secondary)

	inputStyle = lipgloss.NewStyle().
			Foreground(text)

	paletteHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(accent)

	paletteItemStyle = lipgloss.NewStyle().
				Foreground(text)

	activeItemStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#0F172A")).
			Background(secondary)

	usageStyle = lipgloss.NewStyle().
			Foreground(muted)

	sectionTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(secondary)

	resultStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(secondary).
			Padding(0, 1)

	successStyle = lipgloss.NewStyle().Foreground(success)
	warnStyle    = lipgloss.NewStyle().Foreground(accent)
	errorStyle   = lipgloss.NewStyle().Foreground(danger)
	infoStyle    = lipgloss.NewStyle().Foreground(secondary)
	mutedStyle   = lipgloss.NewStyle().Foreground(muted)

	helpStyle = lipgloss.NewStyle().Foreground(muted)
)

func (m Model) renderCommandWorkspace() string {
	var b strings.Builder

	w := m.width
	if w < 64 {
		w = 64
	}
	maxLine := w - 4

	stats := fmt.Sprintf("catalog:%d  selected:%d  targets:%d  repo:%s",
		len(m.available),
		len(m.cfg.SelectedSkills),
		len(m.cfg.Targets),
		config.CompactPath(m.paths.SourceRepo),
	)
	stats = truncateASCII(stats, maxLine)

	b.WriteString(titleStyle.Render(" skillctl // slash command center ") + "\n")
	b.WriteString(subtitleStyle.Render(" "+stats) + "\n")
	b.WriteString(headerDivider.Render(strings.Repeat("-", min(maxLine, 92))) + "\n\n")

	b.WriteString(promptLabelStyle.Render(" command "))
	b.WriteString(inputStyle.Render(m.commandInput.View()))
	b.WriteString("\n")

	visible := m.visibleMatches()
	if m.paletteOpen() && len(visible) > 0 {
		b.WriteString(paletteHeaderStyle.Render(" matching commands") + "\n")
		for i, match := range visible {
			line := fmt.Sprintf(" /%-14s %s", match.Command.Name, match.Command.Description)
			line = truncateASCII(line, maxLine)
			if i == m.paletteCursor {
				b.WriteString(activeItemStyle.Render(line) + "\n")
			} else {
				b.WriteString(paletteItemStyle.Render(line) + "\n")
			}
		}

		selected := visible[m.paletteCursor].Command
		usage := truncateASCII(" usage: "+selected.Usage, maxLine)
		b.WriteString(usageStyle.Render(usage) + "\n")
		b.WriteString(helpStyle.Render(" type to filter  up/down move  tab autocomplete  enter run  click select") + "\n")
	}

	b.WriteString("\n")
	b.WriteString(sectionTitleStyle.Render(" output ") + "\n")
	b.WriteString(m.renderOutputPanel(maxLine) + "\n")
	b.WriteString(helpStyle.Render("\n /help for command list  ctrl+c quit"))

	return b.String()
}

func (m Model) renderOutputPanel(maxLine int) string {
	content := strings.TrimRight(m.output, "\n")
	if strings.TrimSpace(content) == "" {
		content = mutedStyle.Render("No output yet. Run /help.")
	}

	lines := strings.Split(content, "\n")
	maxLines := 16
	if m.height > 0 {
		paletteLines := 0
		if m.paletteOpen() {
			paletteLines = 2 + len(m.visibleMatches())
		}
		reserved := 10 + paletteLines
		maxLines = m.height - reserved
		if maxLines < 6 {
			maxLines = 6
		}
	}
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
		lines[0] = mutedStyle.Render("...") + " " + lines[0]
	}

	body := strings.Join(lines, "\n")
	return resultStyle.Width(min(maxLine, 120)).Render(body)
}

func renderGoodbye(width int) string {
	return successStyle.Render("Goodbye.") + "\n"
}

// countDirs counts immediate subdirectories.
func countDirs(path string) int {
	entries, err := os.ReadDir(path)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() {
			n++
		}
	}
	return n
}

func truncateASCII(s string, maxLen int) string {
	if maxLen <= 0 || len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
