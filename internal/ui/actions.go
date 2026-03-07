package ui

import (
	"fmt"
	"os"
	"strconv"
	"strings"

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
		sb.WriteString(successStyle.Render("\nOK: repository is up to date."))
		m.refresh()
	} else {
		sb.WriteString(errorStyle.Render("\nERROR: git pull failed. Resolve git issues before syncing."))
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
	sb.WriteString(mutedStyle.Render(strings.Repeat("-", 64)) + "\n")
	sb.WriteString(fmt.Sprintf("%-4s %-48s %s\n", "#", "Skill", "Status"))
	sb.WriteString(mutedStyle.Render(strings.Repeat("-", 64)) + "\n")

	for i, skill := range m.cfg.SelectedSkills {
		status := successStyle.Render("available")
		if !availableSet[skill] {
			status = errorStyle.Render("missing")
		}
		sb.WriteString(fmt.Sprintf("%-4d %-48s %s\n", i+1, skill, status))
	}

	sb.WriteString(mutedStyle.Render(strings.Repeat("-", 64)) + "\n")
	sb.WriteString(infoStyle.Render(fmt.Sprintf("Total selected: %d", len(m.cfg.SelectedSkills))))

	return sb.String()
}

func (m *Model) actionSearch(query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return warnStyle.Render("Search query cannot be empty.")
	}

	lower := strings.ToLower(query)
	var matches []string
	for _, skill := range m.available {
		if strings.Contains(strings.ToLower(skill), lower) {
			matches = append(matches, skill)
		}
	}

	if len(matches) == 0 {
		return warnStyle.Render("No matching skills found.")
	}

	selectedSet := make(map[string]bool)
	for _, s := range m.cfg.SelectedSkills {
		selectedSet[strings.ToLower(s)] = true
	}

	var sb strings.Builder
	sb.WriteString(infoStyle.Render(fmt.Sprintf("Found %d match(es) for %q", len(matches), query)) + "\n")

	limit := 40
	if len(matches) < limit {
		limit = len(matches)
	}

	for i := 0; i < limit; i++ {
		marker := " "
		if selectedSet[strings.ToLower(matches[i])] {
			marker = "*"
		}
		sb.WriteString(fmt.Sprintf(" %s %2d. %s\n", marker, i+1, matches[i]))
	}
	if len(matches) > limit {
		sb.WriteString(mutedStyle.Render(fmt.Sprintf("... and %d more", len(matches)-limit)))
	}

	sb.WriteString("\n")
	sb.WriteString(mutedStyle.Render("Use /add <skill-name> to add a result."))
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
		sb.WriteString(successStyle.Render(fmt.Sprintf("\nOK: removed %d target folder(s).", len(outcome.RemovedPaths))))
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
	sb.WriteString(infoStyle.Render(fmt.Sprintf("Syncing %d skill(s) -> %d target(s)",
		len(m.cfg.SelectedSkills), len(m.cfg.Targets))) + "\n")

	for i, result := range outcome.TargetResults {
		sb.WriteString(fmt.Sprintf("\n%s [%d/%d] %s\n",
			infoStyle.Render("Target:"),
			i+1, len(outcome.TargetResults),
			config.CompactPath(result.Target),
		))
		for _, skill := range result.Synced {
			sb.WriteString(fmt.Sprintf("  %s  %s\n", successStyle.Render("+"), skill))
		}
		for skill, errMsg := range result.Failed {
			sb.WriteString(fmt.Sprintf("  %s  %s\n", errorStyle.Render("x"), skill))
			if errMsg != "" {
				sb.WriteString(mutedStyle.Render(fmt.Sprintf("      %s\n", errMsg)))
			}
		}
		sb.WriteString(mutedStyle.Render(fmt.Sprintf("  synced=%d, failed=%d\n",
			len(result.Synced), len(result.Failed))))
	}

	sb.WriteString(fmt.Sprintf("\n%s  Synced: %d  |  Failed: %d",
		successStyle.Render("OK"),
		outcome.TotalSynced(), outcome.TotalFailed()))

	if len(outcome.MissingInSource) > 0 {
		sb.WriteString("\n" + formatMissing(outcome.MissingInSource))
	}

	return sb.String()
}

func (m *Model) actionListTargets() string {
	if len(m.cfg.Targets) == 0 {
		return warnStyle.Render("No targets configured.")
	}

	var sb strings.Builder
	sb.WriteString(infoStyle.Render("Target Folders") + "\n")
	sb.WriteString(mutedStyle.Render(strings.Repeat("-", 64)) + "\n")

	for i, target := range m.cfg.Targets {
		path := config.ExpandPath(target)
		status := errorStyle.Render("missing")
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			status = successStyle.Render(fmt.Sprintf("exists (%d dirs)", countDirs(path)))
		}
		sb.WriteString(fmt.Sprintf("%2d. %-54s %s\n", i+1, target, status))
	}

	sb.WriteString(mutedStyle.Render(strings.Repeat("-", 64)) + "\n")
	sb.WriteString(mutedStyle.Render("Use /target add <path> or /target remove <index|path>."))
	return sb.String()
}

func (m *Model) actionAddTarget(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return warnStyle.Render("Target path cannot be empty.")
	}

	normalized := config.CompactPath(config.ExpandPath(raw))
	for _, t := range m.cfg.Targets {
		if t == normalized {
			return warnStyle.Render("Target already exists: " + normalized)
		}
	}

	m.cfg.Targets = append(m.cfg.Targets, normalized)
	m.cfg.Targets = config.UniqueOrdered(m.cfg.Targets)
	if err := config.SaveConfig(m.paths, m.cfg); err != nil {
		return errorStyle.Render("Failed to save config: " + err.Error())
	}

	return successStyle.Render("Added target: " + normalized)
}

func (m *Model) actionRemoveTarget(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return warnStyle.Render("Target index or path is required.")
	}

	if len(m.cfg.Targets) == 0 {
		return warnStyle.Render("No targets configured.")
	}

	toRemove := ""
	if idx, err := strconv.Atoi(raw); err == nil {
		if idx < 1 || idx > len(m.cfg.Targets) {
			return errorStyle.Render("Invalid target index.")
		}
		toRemove = m.cfg.Targets[idx-1]
	} else {
		normalized := config.CompactPath(config.ExpandPath(raw))
		for _, t := range m.cfg.Targets {
			if t == normalized || t == raw {
				toRemove = t
				break
			}
		}
		if toRemove == "" {
			return errorStyle.Render("Target not found.")
		}
	}

	kept := make([]string, 0, len(m.cfg.Targets)-1)
	for _, t := range m.cfg.Targets {
		if t != toRemove {
			kept = append(kept, t)
		}
	}
	m.cfg.Targets = kept

	if err := config.SaveConfig(m.paths, m.cfg); err != nil {
		return errorStyle.Render("Failed to save config: " + err.Error())
	}

	return successStyle.Render("Removed target: " + toRemove)
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
	sb.WriteString(mutedStyle.Render(strings.Repeat("-", 64)) + "\n")
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
	sb.WriteString(mutedStyle.Render(strings.Repeat("-", 64)) + "\n")

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
		for _, missing := range outcome.Missing {
			if len(missing.Suggestions) > 0 {
				sb.WriteString(fmt.Sprintf("  ! %s (did you mean: %s)\n", missing.Name, strings.Join(missing.Suggestions, ", ")))
			} else {
				sb.WriteString(fmt.Sprintf("  ! %s\n", missing.Name))
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
