package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"akhilsingh.in/skillctl/internal/config"
)

var (
	primary     = lipgloss.Color("#67E8F9")
	secondary   = lipgloss.Color("#93C5FD")
	accent      = lipgloss.Color("#FBBF24")
	success     = lipgloss.Color("#34D399")
	danger      = lipgloss.Color("#F87171")
	muted       = lipgloss.Color("#94A3B8")
	text        = lipgloss.Color("#E2E8F0")
	panelBorder = lipgloss.Color("#334155")
)

var (
	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(panelBorder).
			Padding(1, 2)

	appNameStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primary)

	versionStyle = lipgloss.NewStyle().
			Foreground(muted)

	statusStyle = lipgloss.NewStyle().
			Foreground(accent)

	statsStyle = lipgloss.NewStyle().
			Foreground(muted)

	repoStyle = lipgloss.NewStyle().
			Foreground(text)

	headerDivider = lipgloss.NewStyle().
			Foreground(panelBorder)

	welcomeTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(primary)

	welcomeBrandStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(primary)

	welcomeHintStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(accent)

	inputPromptStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(secondary)

	inputStyle = lipgloss.NewStyle().
			Foreground(text)

	invalidInputStyle = lipgloss.NewStyle().
				Foreground(accent)

	dropdownStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(panelBorder).
			Padding(0, 1)

	dropdownHeaderStyle = lipgloss.NewStyle().
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

	commandBodyStyle = lipgloss.NewStyle().
				Foreground(secondary)

	outputBodyStyle = lipgloss.NewStyle().
			Foreground(text)

	messageLabelStyle = lipgloss.NewStyle().
				Foreground(muted)

	successStyle = lipgloss.NewStyle().Foreground(success)
	warnStyle    = lipgloss.NewStyle().Foreground(accent)
	errorStyle   = lipgloss.NewStyle().Foreground(danger)
	infoStyle    = lipgloss.NewStyle().Foreground(secondary)
	mutedStyle   = lipgloss.NewStyle().Foreground(muted)

	helpStyle = lipgloss.NewStyle().Foreground(muted)
)

func (m Model) renderChatWorkspace() string {
	compact := m.compactLayout()
	tiny := m.tinyLayout()

	innerWidth := m.contentWidth
	if innerWidth <= 0 {
		innerWidth = defaultContentWidth
	}
	if compact {
		if m.width > 0 {
			innerWidth = m.width - 2
			if innerWidth < 12 {
				innerWidth = m.width - 1
			}
			if innerWidth < 10 {
				innerWidth = 10
			}
		}
	} else if m.width > 0 {
		maxWidth := max(36, m.width-8)
		if innerWidth > maxWidth {
			innerWidth = maxWidth
		}
	}

	header := m.renderHeader(innerWidth, compact, tiny)
	transcript := m.chatViewport.View()
	inputRow := m.renderInputRow(innerWidth)
	helpRow := m.renderHelpBar(innerWidth)

	parts := []string{header}
	if !tiny {
		parts = append(parts, "")
	}
	parts = append(parts, transcript)
	if !tiny {
		parts = append(parts, "")
	}
	parts = append(parts, inputRow)
	if dropdown := m.renderSkillPickerDropdown(innerWidth); dropdown != "" {
		parts = append(parts, dropdown)
	} else if dropdown := m.renderCommandDropdown(innerWidth); dropdown != "" {
		parts = append(parts, dropdown)
	}
	if !tiny {
		parts = append(parts, "")
	}
	parts = append(parts, helpRow)

	body := lipgloss.JoinVertical(lipgloss.Left, parts...)
	panel := body
	if !compact {
		panel = panelStyle.Render(body)
	}

	if m.width > 0 {
		panel = lipgloss.PlaceHorizontal(m.width, lipgloss.Center, panel)
	}

	if compact {
		return panel
	}
	if m.height > 28 {
		return "\n" + panel
	}

	return "\n" + panel
}

func (m Model) renderHeader(width int, compact bool, tiny bool) string {
	stats := fmt.Sprintf("repos %d   catalog %d   selected %d   targets %d",
		len(m.cfg.Repositories),
		len(m.available),
		len(m.cfg.SelectedSkills),
		len(m.cfg.Targets),
	)
	stats = truncateASCII(stats, width)

	repo := truncateASCII("workspace "+config.CompactPath(m.paths.WorkspaceDir), width)

	state := mutedStyle.Render("idle")
	if m.gitPullRunning {
		state = statusStyle.Render("pulling updates...")
	}

	title := lipgloss.JoinHorizontal(
		lipgloss.Left,
		appNameStyle.Render("skillctl"),
		" ",
		versionStyle.Render("v"+config.Version),
		"   ",
		state,
	)

	divider := headerDivider.Render(strings.Repeat("-", min(width, 96)))
	center := lipgloss.NewStyle().Width(width).Align(lipgloss.Center)
	if tiny {
		return center.Render(title)
	}

	if compact {
		return lipgloss.JoinVertical(
			lipgloss.Left,
			center.Render(title),
			center.Render(statsStyle.Render(stats)),
		)
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		center.Render(title),
		center.Render(statsStyle.Render(stats)),
		center.Render(repoStyle.Render(repo)),
		center.Render(divider),
	)
}

func (m Model) renderInputRow(width int) string {
	line := lipgloss.JoinHorizontal(
		lipgloss.Left,
		inputPromptStyle.Render("> "),
		inputStyle.Render(m.commandInput.View()),
	)

	value := strings.TrimSpace(m.commandInput.Value())
	if !m.skillPickerOpen && value != "" && !hasCommandPrefix(value) {
		line += invalidInputStyle.Render("  start with /")
	}

	return lipgloss.NewStyle().Width(width).Render(line)
}

func (m Model) renderCommandDropdown(width int) string {
	visible := m.visibleMatches()
	if !m.paletteOpen() || len(visible) == 0 {
		return ""
	}

	displayCount := min(len(visible), m.maxDropdownItems())
	lines := make([]string, 0, displayCount+3)
	lines = append(lines, dropdownHeaderStyle.Render("⌘"))
	lineWidth := width - 2
	if lineWidth < 6 {
		lineWidth = 6
	}

	for i := 0; i < displayCount; i++ {
		match := visible[i]
		line := fmt.Sprintf(" /%-16s %s", match.Command.Name, match.Command.Description)
		line = truncateASCII(line, lineWidth)
		if i == m.paletteCursor {
			lines = append(lines, activeItemStyle.Render(line))
		} else {
			lines = append(lines, paletteItemStyle.Render(line))
		}
	}

	if m.paletteCursor >= 0 && m.paletteCursor < len(visible) {
		selected := visible[m.paletteCursor].Command
		usage := truncateASCII(" usage: "+selected.Usage, lineWidth)
		lines = append(lines, usageStyle.Render(usage))
	}

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	if m.tinyLayout() {
		return lipgloss.NewStyle().Width(width).Render(content)
	}
	return dropdownStyle.Width(width).Render(content)
}

func (m Model) renderSkillPickerDropdown(width int) string {
	if !m.skillPickerOpen {
		return ""
	}

	visible := m.visibleSkillMatches()
	displayCount := min(len(visible), m.maxDropdownItems())
	lineWidth := width - 2
	if lineWidth < 6 {
		lineWidth = 6
	}

	query := strings.TrimSpace(m.commandInput.Value())
	header := "🔍 Skills"
	if query != "" {
		header = fmt.Sprintf("🔍 %d match(es) for %q", len(m.skillMatches), query)
	} else {
		header = fmt.Sprintf("🔍 %d skill(s)", len(m.skillMatches))
	}

	lines := make([]string, 0, displayCount+4)
	lines = append(lines, dropdownHeaderStyle.Render(truncateASCII(header, lineWidth)))

	if len(m.available) == 0 {
		lines = append(lines, warnStyle.Render(truncateASCII(" No skills available. Run /pull first.", lineWidth)))
	} else if displayCount == 0 {
		lines = append(lines, warnStyle.Render(truncateASCII(" No matching skills.", lineWidth)))
	} else {
		for i := 0; i < displayCount; i++ {
			match := visible[i]
			marker := " "
			if match.Selected {
				marker = "*"
			}

			prefix := fmt.Sprintf(" %s %4d. ", marker, match.CatalogIndex)
			name := skillDisplayName(match.Skill)
			namespace := skillNamespace(match.Skill)

			nameWidth := lineWidth - len(prefix)
			if namespace != "" {
				namespaceWidth := max(10, min(34, lineWidth/3))
				namespace = truncateASCII(namespace, namespaceWidth)
				nameWidth = lineWidth - len(prefix) - len(namespace) - 2
				if nameWidth < 8 {
					nameWidth = 8
					namespace = truncateASCII(namespace, max(0, lineWidth-len(prefix)-nameWidth-2))
				}
			}

			name = truncateASCII(name, nameWidth)
			plain := prefix + name
			styled := plain
			if namespace != "" {
				plain += "  " + namespace
				styled += "  " + mutedStyle.Render(namespace)
			}
			plain = truncateASCII(plain, lineWidth)
			if i == m.skillCursor {
				lines = append(lines, activeItemStyle.Render(plain))
			} else {
				lines = append(lines, paletteItemStyle.Render(styled))
			}
		}

		if len(m.skillMatches) > displayCount {
			more := fmt.Sprintf(" ... and %d more", len(m.skillMatches)-displayCount)
			lines = append(lines, mutedStyle.Render(truncateASCII(more, lineWidth)))
		}
	}

	lines = append(lines, usageStyle.Render(truncateASCII(" enter add  esc cancel", lineWidth)))
	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	if m.tinyLayout() {
		return lipgloss.NewStyle().Width(width).Render(content)
	}
	return dropdownStyle.Width(width).Render(content)
}

func (m Model) renderHelpBar(width int) string {
	if m.tinyLayout() {
		help := "/ commands  enter run  ctrl+c quit"
		if m.skillPickerOpen {
			help = "type search  up/down pick  enter add"
		} else if m.paletteOpen() {
			help = "up/down select  tab fill  enter run"
		}
		return helpStyle.Width(width).Render(truncateASCII(help, width))
	}

	help := "type / for commands  up/down history  pgup/pgdn scroll  ctrl+c quit"
	if m.skillPickerOpen {
		help = "type to search  up/down select  enter add  esc cancel  ctrl+c quit"
	} else if m.paletteOpen() {
		help = "up/down select  tab autocomplete  enter run  esc reset  ctrl+c quit"
	}
	if m.gitPullRunning {
		help = "git pull running... output updates live  esc reset input  ctrl+c quit"
	}
	return helpStyle.Width(width).Render(truncateASCII(help, width))
}

func (m Model) renderChatViewportContent(width int) string {
	if len(m.chatMessages) == 0 {
		return m.renderWelcomeState(width)
	}

	parts := make([]string, 0, len(m.chatMessages)*2)
	for i, msg := range m.chatMessages {
		parts = append(parts, m.renderChatMessage(msg, width))
		if i < len(m.chatMessages)-1 {
			parts = append(parts, "")
		}
	}

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m Model) renderWelcomeState(width int) string {
	center := lipgloss.NewStyle().Width(width).Align(lipgloss.Center)
	brand := welcomeBrandStyle.Render("S K I L L C T L")
	if m.tinyLayout() {
		return lipgloss.JoinVertical(
			lipgloss.Left,
			center.Render(welcomeTitleStyle.Render("skillctl")),
			center.Render(welcomeHintStyle.Render("Type / for commands")),
		)
	}

	if m.compactLayout() {
		stats := fmt.Sprintf("Repos: %d  Catalog: %d  Selected: %d  Targets: %d",
			len(m.cfg.Repositories),
			len(m.available),
			len(m.cfg.SelectedSkills),
			len(m.cfg.Targets),
		)
		return lipgloss.JoinVertical(
			lipgloss.Left,
			center.Render(brand),
			center.Render(welcomeTitleStyle.Render("Welcome to skillctl")),
			center.Render(statsStyle.Render(truncateASCII(stats, width))),
			center.Render(welcomeHintStyle.Render("Type / to open commands")),
		)
	}

	stats := fmt.Sprintf("Repos: %d   Catalog: %d   Selected: %d   Targets: %d",
		len(m.cfg.Repositories),
		len(m.available),
		len(m.cfg.SelectedSkills),
		len(m.cfg.Targets),
	)

	return lipgloss.JoinVertical(
		lipgloss.Left,
		center.Render(brand),
		center.Render(welcomeTitleStyle.Render("Welcome to skillctl")),
		center.Render(mutedStyle.Render("Manage and sync your local skills from one place.")),
		"",
		center.Render(statsStyle.Render(truncateASCII(stats, width))),
		"",
		center.Render(welcomeHintStyle.Render("Type / to open commands")),
		center.Render(mutedStyle.Render("Try /help, /list, or /sync")),
	)
}

func (m Model) renderChatMessage(msg chatMessage, width int) string {
	if width < 10 {
		width = 10
	}

	borderColor := primary
	bodyStyle := outputBodyStyle
	label := "💬"

	switch msg.Type {
	case chatMessageCommand:
		borderColor = secondary
		bodyStyle = commandBodyStyle
		label = "⌘"
	case chatMessageInfo:
		borderColor = muted
		bodyStyle = mutedStyle
		label = "ℹ️"
	case chatMessageError:
		borderColor = danger
		bodyStyle = errorStyle
		label = "⚠️"
	}

	content := strings.TrimRight(msg.Content, "\n")
	if m.tinyLayout() {
		return lipgloss.NewStyle().
			Width(width).
			BorderLeft(true).
			BorderStyle(lipgloss.ThickBorder()).
			BorderForeground(borderColor).
			PaddingLeft(1).
			Render(bodyStyle.Render(content))
	}

	rendered := lipgloss.JoinVertical(
		lipgloss.Left,
		messageLabelStyle.Render(label),
		bodyStyle.Render(content),
	)

	return lipgloss.NewStyle().
		Width(width).
		BorderLeft(true).
		BorderStyle(lipgloss.ThickBorder()).
		BorderForeground(borderColor).
		PaddingLeft(1).
		Render(rendered)
}

func renderGoodbye(width int) string {
	return successStyle.Render("Goodbye.") + "\n"
}

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

func skillDisplayName(skill config.AvailableSkill) string {
	name := strings.TrimSpace(skill.Name)
	if name != "" {
		return name
	}
	if slash := strings.LastIndex(skill.ID, "/"); slash >= 0 && slash+1 < len(skill.ID) {
		return skill.ID[slash+1:]
	}
	return skill.ID
}

func skillNamespace(skill config.AvailableSkill) string {
	ns := strings.TrimSpace(skill.RepoID)
	if ns != "" {
		return ns
	}
	if slash := strings.Index(skill.ID, "/"); slash > 0 {
		return skill.ID[:slash]
	}
	return ""
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
