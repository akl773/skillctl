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
	primary   = lipgloss.Color("#7C3AED") // violet
	secondary = lipgloss.Color("#06B6D4") // cyan
	accent    = lipgloss.Color("#F59E0B") // amber
	success   = lipgloss.Color("#10B981") // emerald
	danger    = lipgloss.Color("#EF4444") // red
	muted     = lipgloss.Color("#6B7280") // gray
	surface   = lipgloss.Color("#1E1B4B") // dark indigo
	text      = lipgloss.Color("#F8FAFC") // slate-50
	dim       = lipgloss.Color("#94A3B8") // slate-400
)

// --- Styles ---
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(text).
			Background(primary).
			Padding(0, 2).
			MarginBottom(1)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(dim).
			MarginBottom(1)

	menuItemStyle = lipgloss.NewStyle().
			PaddingLeft(2)

	activeItemStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(text).
			Background(primary).
			PaddingLeft(2).
			PaddingRight(2)

	descStyle = lipgloss.NewStyle().
			Foreground(dim).
			PaddingLeft(1)

	sectionStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(primary).
			Padding(1, 2).
			MarginTop(1)

	resultStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(secondary).
			Padding(1, 2)

	successStyle = lipgloss.NewStyle().Foreground(success)
	warnStyle    = lipgloss.NewStyle().Foreground(accent)
	errorStyle   = lipgloss.NewStyle().Foreground(danger)
	infoStyle    = lipgloss.NewStyle().Foreground(secondary)
	mutedStyle   = lipgloss.NewStyle().Foreground(muted)

	badgeOk      = lipgloss.NewStyle().Foreground(success).Bold(true)
	badgeMissing = lipgloss.NewStyle().Foreground(danger).Bold(true)

	helpStyle = lipgloss.NewStyle().Foreground(muted).MarginTop(1)

	headerDivider = lipgloss.NewStyle().Foreground(primary)
)

// --- View renderers ---

func (m Model) renderMainMenu() string {
	var b strings.Builder

	// Header
	title := titleStyle.Render(" ⚡ SkillCtl ")
	stats := subtitleStyle.Render(
		fmt.Sprintf("catalog: %d │ selected: %d │ targets: %d │ repo: %s",
			len(m.available),
			len(m.cfg.SelectedSkills),
			len(m.cfg.Targets),
			config.CompactPath(m.paths.SourceRepo),
		),
	)
	b.WriteString(title + "\n")
	b.WriteString(stats + "\n")

	w := m.width
	if w < 40 {
		w = 80
	}
	divider := headerDivider.Render(strings.Repeat("─", min(w-4, 72)))
	b.WriteString(divider + "\n\n")

	// Menu items
	for i, item := range m.items {
		cursor := "  "
		var line string

		if i == m.cursor {
			cursor = "▸ "
			entry := fmt.Sprintf("%s %s", item.icon, item.title)
			line = cursor + activeItemStyle.Render(entry) + descStyle.Render(item.desc)
		} else {
			entry := fmt.Sprintf("%s %s", item.icon, item.title)
			line = cursor + menuItemStyle.Render(entry) + descStyle.Render(item.desc)
		}
		b.WriteString(line + "\n")
	}

	// Footer help
	b.WriteString(helpStyle.Render("\n  ↑/k up • ↓/j down • enter select • q quit"))

	return b.String()
}

func (m Model) renderResultView() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render(" ⚡ SkillCtl ") + "\n\n")

	content := resultStyle.Render(m.result)
	b.WriteString(content)

	b.WriteString(helpStyle.Render("\n\n  press enter or esc to go back"))

	return b.String()
}

func (m Model) renderInputView() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render(" ⚡ SkillCtl ") + "\n\n")

	var label string
	switch m.inputAction {
	case inputAddSkill:
		label = "Add skill(s)"
		b.WriteString(infoStyle.Render("  Enter skill name(s) or catalog numbers, comma-separated") + "\n")
		b.WriteString(m.renderSkillCatalog() + "\n")
	case inputSearch:
		label = "Search skills"
		b.WriteString(infoStyle.Render("  Enter search terms to find skills in the catalog") + "\n\n")
	case inputRemoveSkill:
		label = "Remove skill(s)"
		b.WriteString(infoStyle.Render("  Enter skill name(s) or list numbers to remove") + "\n")
		b.WriteString(m.renderSelectedList() + "\n")
	}

	prompt := lipgloss.NewStyle().
		Bold(true).
		Foreground(secondary).
		Render(fmt.Sprintf("  %s: ", label))

	b.WriteString(prompt + m.textInput.View() + "\n")
	b.WriteString(helpStyle.Render("\n  enter to submit • esc to cancel"))

	return b.String()
}

func (m Model) renderSearchResultsView() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render(" ⚡ SkillCtl ") + "\n\n")

	if len(m.searchMatches) == 0 {
		b.WriteString(warnStyle.Render("  No matches found.") + "\n")
	} else {
		b.WriteString(infoStyle.Render(
			fmt.Sprintf("  Found %d match(es)", len(m.searchMatches)),
		) + "\n\n")

		limit := 30
		shown := m.searchMatches
		if len(shown) > limit {
			shown = shown[:limit]
		}

		selectedSet := make(map[string]bool)
		for _, s := range m.cfg.SelectedSkills {
			selectedSet[strings.ToLower(s)] = true
		}

		for i, skill := range shown {
			marker := " "
			if selectedSet[strings.ToLower(skill)] {
				marker = successStyle.Render("✓")
			}
			b.WriteString(fmt.Sprintf("  %s %2d. %s\n", marker, i+1, skill))
		}
		if len(m.searchMatches) > limit {
			b.WriteString(mutedStyle.Render(
				fmt.Sprintf("  ... and %d more\n", len(m.searchMatches)-limit),
			))
		}
	}

	prompt := lipgloss.NewStyle().
		Bold(true).
		Foreground(secondary).
		Render("  Add by # or name (comma): ")

	b.WriteString("\n" + prompt + m.textInput.View() + "\n")
	b.WriteString(helpStyle.Render("\n  enter to add • esc to go back"))

	return b.String()
}

func (m Model) renderTargetsView() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render(" ⚡ SkillCtl ") + "\n\n")
	b.WriteString(infoStyle.Render("  Target Folders") + "\n\n")

	if len(m.cfg.Targets) == 0 {
		b.WriteString(warnStyle.Render("  No targets configured.") + "\n")
	} else {
		for i, target := range m.cfg.Targets {
			path := config.ExpandPath(target)
			var status string
			if info, err := os.Stat(path); err == nil && info.IsDir() {
				count := countDirs(path)
				status = badgeOk.Render("exists") + mutedStyle.Render(fmt.Sprintf(" (%d dirs)", count))
			} else {
				status = badgeMissing.Render("missing")
			}
			b.WriteString(fmt.Sprintf("  %2d. %s  [%s]\n", i+1, target, status))
		}
	}

	b.WriteString(helpStyle.Render("\n  a add • r remove • esc/q back"))

	return b.String()
}

func (m Model) renderTargetInputView() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render(" ⚡ SkillCtl ") + "\n\n")

	var label string
	switch m.inputAction {
	case inputAddTarget:
		label = "New target path"
	case inputRemoveTarget:
		label = "Target # to remove"
	}

	prompt := lipgloss.NewStyle().
		Bold(true).
		Foreground(secondary).
		Render(fmt.Sprintf("  %s: ", label))

	b.WriteString(prompt + m.textInput.View() + "\n")
	b.WriteString(helpStyle.Render("\n  enter to submit • esc to cancel"))

	return b.String()
}

func renderGoodbye(width int) string {
	msg := lipgloss.NewStyle().
		Bold(true).
		Foreground(success).
		Render("  👋 Goodbye!")
	return msg + "\n"
}

// --- Helper renderers ---

func (m Model) renderSkillCatalog() string {
	if len(m.available) == 0 {
		return warnStyle.Render("  No skills available in catalog.")
	}

	var b strings.Builder
	b.WriteString("\n")

	selectedSet := make(map[string]bool)
	for _, s := range m.cfg.SelectedSkills {
		selectedSet[strings.ToLower(s)] = true
	}

	limit := 40
	shown := m.available
	if len(shown) > limit {
		shown = shown[:limit]
	}

	for i, skill := range shown {
		marker := " "
		if selectedSet[strings.ToLower(skill)] {
			marker = successStyle.Render("✓")
		}
		b.WriteString(fmt.Sprintf("  %s %2d. %s\n", marker, i+1, skill))
	}
	if len(m.available) > limit {
		b.WriteString(mutedStyle.Render(
			fmt.Sprintf("  ... and %d more\n", len(m.available)-limit),
		))
	}
	return b.String()
}

func (m Model) renderSelectedList() string {
	if len(m.cfg.SelectedSkills) == 0 {
		return warnStyle.Render("  No skills selected.")
	}

	var b strings.Builder
	b.WriteString("\n")

	availableSet := make(map[string]bool)
	for _, s := range m.available {
		availableSet[s] = true
	}

	for i, skill := range m.cfg.SelectedSkills {
		var status string
		if availableSet[skill] {
			status = badgeOk.Render("available")
		} else {
			status = badgeMissing.Render("missing")
		}
		b.WriteString(fmt.Sprintf("  %2d. %-48s %s\n", i+1, skill, status))
	}
	return b.String()
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
