package ui

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"akhilsingh.in/skillctl/internal/config"
	"akhilsingh.in/skillctl/internal/core"
)

// --- Action handlers ---

func (m *Model) actionGitPull() string {
	outcome := core.RunGitPull(m.paths)

	var sb strings.Builder
	if strings.TrimSpace(outcome.Stdout) != "" {
		sb.WriteString(outcome.Stdout)
	}
	if strings.TrimSpace(outcome.Stderr) != "" {
		sb.WriteString(mutedStyle.Render(strings.TrimSpace(outcome.Stderr)) + "\n")
	}

	if outcome.Success() {
		sb.WriteString(successStyle.Render("\n✓ Repository is up to date."))
		m.refresh()
	} else {
		sb.WriteString(errorStyle.Render("\n✗ Git pull failed. Resolve git issues before syncing."))
	}

	return sb.String()
}

func (m *Model) actionListSelected() string {
	if len(m.cfg.SelectedSkills) == 0 {
		return warnStyle.Render("No skills selected yet.")
	}

	availableSet := make(map[string]bool)
	for _, s := range m.available {
		availableSet[s] = true
	}

	var sb strings.Builder
	sb.WriteString(infoStyle.Render("Selected Skills") + "\n")
	sb.WriteString(mutedStyle.Render(strings.Repeat("─", 64)) + "\n")
	sb.WriteString(fmt.Sprintf("%-4s %-48s %s\n", "#", "Skill", "Status"))
	sb.WriteString(mutedStyle.Render(strings.Repeat("─", 64)) + "\n")

	for i, skill := range m.cfg.SelectedSkills {
		var status string
		if availableSet[skill] {
			status = successStyle.Render("available")
		} else {
			status = errorStyle.Render("missing")
		}
		sb.WriteString(fmt.Sprintf("%-4d %-48s %s\n", i+1, skill, status))
	}

	sb.WriteString(mutedStyle.Render(strings.Repeat("─", 64)) + "\n")
	sb.WriteString(infoStyle.Render(fmt.Sprintf("Total selected: %d", len(m.cfg.SelectedSkills))))

	return sb.String()
}

func (m *Model) actionAddSkill(raw string) string {
	tokens := config.InputCSV(raw)
	if len(tokens) == 0 {
		return warnStyle.Render("No skill names provided.")
	}

	requested, invalid := config.SplitByReference(tokens, m.available)

	var sb strings.Builder
	if len(invalid) > 0 {
		sb.WriteString(errorStyle.Render(fmt.Sprintf("Invalid catalog number(s): %s", strings.Join(invalid, ", "))) + "\n\n")
	}

	outcome := core.AddRequestedSkills(&m.cfg, requested, m.available)
	if len(outcome.Added) > 0 {
		_ = config.SaveConfig(m.paths, m.cfg)
	}
	sb.WriteString(formatAddOutcome(outcome))

	return sb.String()
}

func (m *Model) actionSearch(query string) (Model, tea.Cmd) {
	lower := strings.ToLower(query)
	var matches []string
	for _, skill := range m.available {
		if strings.Contains(strings.ToLower(skill), lower) {
			matches = append(matches, skill)
		}
	}

	m.searchMatches = matches
	m.view = viewSearchResults
	m.textInput.SetValue("")
	m.textInput.Placeholder = "add by # or name (comma-sep)"
	m.textInput.Focus()
	return *m, m.textInput.Focus()
}

func (m *Model) actionRemoveSkill(raw string) string {
	tokens := config.InputCSV(raw)
	if len(tokens) == 0 {
		return warnStyle.Render("No skill names provided.")
	}

	requested, invalid := config.SplitByReference(tokens, m.cfg.SelectedSkills)

	var sb strings.Builder
	if len(invalid) > 0 {
		sb.WriteString(errorStyle.Render(fmt.Sprintf("Invalid number(s): %s", strings.Join(invalid, ", "))) + "\n\n")
	}

	outcome := core.RemoveSelectedSkills(&m.cfg, requested)
	if len(outcome.RemovedFromSelected) > 0 {
		_ = config.SaveConfig(m.paths, m.cfg)
	}

	if len(outcome.RemovedFromSelected) > 0 {
		sb.WriteString(successStyle.Render("Removed from selection:") + "\n")
		for _, skill := range outcome.RemovedFromSelected {
			sb.WriteString(fmt.Sprintf("  - %s\n", skill))
		}

		if len(outcome.RemovedPaths) > 0 {
			sb.WriteString(infoStyle.Render("\nRemoved from targets:") + "\n")
			for _, p := range outcome.RemovedPaths {
				sb.WriteString(fmt.Sprintf("  - %s\n", p))
			}
		}
		sb.WriteString(successStyle.Render(fmt.Sprintf("\n✓ Removed %d target folder(s).", len(outcome.RemovedPaths))))
	}

	if len(outcome.NotSelected) > 0 {
		sb.WriteString(warnStyle.Render(fmt.Sprintf("\nNot in selected list: %s", strings.Join(outcome.NotSelected, ", "))))
	}

	return sb.String()
}

func (m *Model) actionSync() string {
	if len(m.cfg.SelectedSkills) == 0 {
		return warnStyle.Render("No skills selected. Add skills first.")
	}
	if len(m.cfg.Targets) == 0 {
		return warnStyle.Render("No targets configured. Add targets first.")
	}

	outcome := core.SyncSelectedSkills(m.paths, m.cfg, m.available)
	if len(outcome.TargetResults) == 0 {
		result := errorStyle.Render("Nothing to sync.")
		if len(outcome.MissingInSource) > 0 {
			result += "\n" + formatMissing(outcome.MissingInSource)
		}
		return result
	}

	var sb strings.Builder
	sb.WriteString(infoStyle.Render(fmt.Sprintf("Syncing %d skill(s) → %d target(s)",
		len(m.cfg.SelectedSkills), len(m.cfg.Targets))) + "\n")

	for i, result := range outcome.TargetResults {
		sb.WriteString(fmt.Sprintf("\n%s [%d/%d] %s\n",
			infoStyle.Render("Target:"),
			i+1, len(outcome.TargetResults),
			config.CompactPath(result.Target),
		))
		for _, skill := range result.Synced {
			sb.WriteString(fmt.Sprintf("  %s  %s\n", successStyle.Render("✓"), skill))
		}
		for skill, errMsg := range result.Failed {
			sb.WriteString(fmt.Sprintf("  %s  %s\n", errorStyle.Render("✗"), skill))
			if errMsg != "" {
				sb.WriteString(mutedStyle.Render(fmt.Sprintf("      %s\n", errMsg)))
			}
		}
		sb.WriteString(mutedStyle.Render(fmt.Sprintf("  synced=%d, failed=%d\n",
			len(result.Synced), len(result.Failed))))
	}

	sb.WriteString(fmt.Sprintf("\n%s  Synced: %d  |  Failed: %d",
		successStyle.Render("✓"),
		outcome.TotalSynced(), outcome.TotalFailed()))

	if len(outcome.MissingInSource) > 0 {
		sb.WriteString("\n" + formatMissing(outcome.MissingInSource))
	}

	return sb.String()
}

func (m *Model) actionStatus() string {
	availableSet := make(map[string]bool)
	for _, s := range m.available {
		availableSet[s] = true
	}
	var missing []string
	for _, s := range m.cfg.SelectedSkills {
		if !availableSet[s] {
			missing = append(missing, s)
		}
	}

	var sb strings.Builder
	sb.WriteString(infoStyle.Render("Status") + "\n")
	sb.WriteString(mutedStyle.Render(strings.Repeat("─", 64)) + "\n")
	sb.WriteString(fmt.Sprintf("Repo path        : %s\n", config.CompactPath(m.paths.SourceRepo)))
	sb.WriteString(fmt.Sprintf("Config path      : %s\n", config.CompactPath(m.paths.ConfigPath)))
	sb.WriteString(fmt.Sprintf("Source skills    : %d\n", len(m.available)))
	sb.WriteString(fmt.Sprintf("Selected skills  : %d\n", len(m.cfg.SelectedSkills)))
	sb.WriteString(fmt.Sprintf("Targets          : %d\n", len(m.cfg.Targets)))

	if len(missing) > 0 {
		sb.WriteString(warnStyle.Render("\nSelected but missing in source:") + "\n")
		for _, s := range missing {
			sb.WriteString(fmt.Sprintf("  - %s\n", s))
		}
	}

	sb.WriteString(infoStyle.Render("\nTarget overview") + "\n")
	sb.WriteString(mutedStyle.Render(strings.Repeat("─", 64)) + "\n")

	selectedSet := make(map[string]bool)
	for _, s := range m.cfg.SelectedSkills {
		selectedSet[s] = true
	}

	for _, target := range m.cfg.Targets {
		path := config.ExpandPath(target)
		if _, err := os.Stat(path); err != nil {
			sb.WriteString(fmt.Sprintf("- %s: %s\n", target, warnStyle.Render("missing")))
			continue
		}

		entries, _ := os.ReadDir(path)
		var dirs []string
		for _, e := range entries {
			if e.IsDir() {
				dirs = append(dirs, e.Name())
			}
		}
		selectedPresent := 0
		extras := 0
		for _, name := range dirs {
			if selectedSet[name] {
				selectedPresent++
			} else {
				extras++
			}
		}
		sb.WriteString(fmt.Sprintf("- %s: %s | dirs=%d | selected=%d/%d | extras=%d\n",
			target,
			successStyle.Render("exists"),
			len(dirs), selectedPresent, len(m.cfg.SelectedSkills), extras))
	}

	return sb.String()
}

// --- Format helpers ---

func formatAddOutcome(outcome core.AddOutcome) string {
	var sb strings.Builder

	if len(outcome.Added) > 0 {
		sb.WriteString(successStyle.Render("Added:") + "\n")
		for _, s := range outcome.Added {
			sb.WriteString(fmt.Sprintf("  + %s\n", s))
		}
	}

	if len(outcome.AlreadySelected) > 0 {
		sb.WriteString(warnStyle.Render("\nAlready selected:") + "\n")
		for _, s := range outcome.AlreadySelected {
			sb.WriteString(fmt.Sprintf("  = %s\n", s))
		}
	}

	if len(outcome.Missing) > 0 {
		sb.WriteString(errorStyle.Render("\nNot found:") + "\n")
		for _, m := range outcome.Missing {
			if len(m.Suggestions) > 0 {
				sb.WriteString(fmt.Sprintf("  ! %s (did you mean: %s)\n", m.Name, strings.Join(m.Suggestions, ", ")))
			} else {
				sb.WriteString(fmt.Sprintf("  ! %s\n", m.Name))
			}
		}
	}

	return sb.String()
}

func formatMissing(missing []string) string {
	var sb strings.Builder
	sb.WriteString(warnStyle.Render("\nSelected but missing in source:") + "\n")
	for _, s := range missing {
		sb.WriteString(fmt.Sprintf("  - %s\n", s))
	}
	return sb.String()
}
