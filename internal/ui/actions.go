package ui

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"akhilsingh.in/skillctl/internal/config"
	"akhilsingh.in/skillctl/internal/core"
)

// --- Action handlers ---

func (m *Model) actionGitPull() commandResult {
	if m.gitPullRunning {
		return commandResult{Output: warnStyle.Render("A git pull is already running.")}
	}
	if len(m.cfg.Repositories) == 0 {
		return commandResult{Output: warnStyle.Render("No repositories configured. Add one with /repo add <url>.")}
	}

	m.gitPullRunning = true
	m.gitPullOutput.Reset()
	m.gitPullOutput.WriteString(infoStyle.Render("Updating configured repositories...") + "\n")

	return commandResult{Output: m.gitPullOutput.String(), Cmd: startGitPullStreamCmd(m.paths, m.cfg.Repositories)}
}

func (m *Model) actionListSelected() string {
	if len(m.cfg.SelectedSkills) == 0 {
		return warnStyle.Render("No skills selected yet.")
	}

	availableSet := make(map[string]bool)
	for _, s := range m.available {
		availableSet[s.ID] = true
	}

	disabledSet := make(map[string]bool)
	for _, s := range m.cfg.DisabledSkills {
		disabledSet[s] = true
	}

	var sb strings.Builder
	sb.WriteString(infoStyle.Render("🧰 Selected") + "\n")
	sb.WriteString(mutedStyle.Render(strings.Repeat("-", 64)) + "\n")
	sb.WriteString(fmt.Sprintf("%-4s %-40s %-10s %s\n", "#", "Skill", "Mode", "Status"))
	sb.WriteString(mutedStyle.Render(strings.Repeat("-", 64)) + "\n")

	disabledCount := 0
	for i, skill := range m.cfg.SelectedSkills {
		status := successStyle.Render("available")
		if !availableSet[skill] {
			status = errorStyle.Render("missing")
		}

		mode := successStyle.Render("enabled")
		skillLabel := skill
		if disabledSet[skill] {
			disabledCount++
			mode = mutedStyle.Render("disabled")
			skillLabel = mutedStyle.Render(skill)
		}

		sb.WriteString(fmt.Sprintf("%-4d %-40s %-10s %s\n", i+1, skillLabel, mode, status))
	}

	sb.WriteString(mutedStyle.Render(strings.Repeat("-", 64)) + "\n")
	sb.WriteString(infoStyle.Render(fmt.Sprintf("📌 total: %d (disabled: %d)", len(m.cfg.SelectedSkills), disabledCount)))
	sb.WriteString("\n")
	sb.WriteString(mutedStyle.Render("ℹ️ /list toggle <index|name> to enable or disable a skill."))

	return sb.String()
}

func (m *Model) actionToggleSkill(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return errorStyle.Render("Usage: /list toggle <index|name>")
	}

	if len(m.cfg.SelectedSkills) == 0 {
		return warnStyle.Render("No skills selected yet.")
	}

	requested, invalid := config.SplitByReference([]string{raw}, m.cfg.SelectedSkills)
	if len(invalid) > 0 {
		return errorStyle.Render("Invalid selected-skill index.")
	}

	selectedMap := make(map[string]string)
	for _, s := range m.cfg.SelectedSkills {
		selectedMap[strings.ToLower(s)] = s
	}

	resolved, ok := selectedMap[strings.ToLower(requested[0])]
	if !ok {
		return errorStyle.Render("Skill not found in selected list.")
	}

	idx := -1
	for i, s := range m.cfg.DisabledSkills {
		if s == resolved {
			idx = i
			break
		}
	}

	if idx >= 0 {
		m.cfg.DisabledSkills = append(m.cfg.DisabledSkills[:idx], m.cfg.DisabledSkills[idx+1:]...)
		if err := config.SaveConfig(m.paths, m.cfg); err != nil {
			return errorStyle.Render("Failed to save config: " + err.Error())
		}
		return successStyle.Render("Enabled: " + resolved)
	}

	m.cfg.DisabledSkills = append(m.cfg.DisabledSkills, resolved)
	m.cfg.DisabledSkills = config.UniqueOrdered(m.cfg.DisabledSkills)
	if err := config.SaveConfig(m.paths, m.cfg); err != nil {
		return errorStyle.Render("Failed to save config: " + err.Error())
	}
	return warnStyle.Render("Disabled: " + resolved)
}

func (m *Model) actionSearch(query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return warnStyle.Render("Search query cannot be empty.")
	}

	lower := strings.ToLower(query)
	var matches []config.AvailableSkill
	for _, skill := range m.available {
		if strings.Contains(strings.ToLower(skill.ID), lower) ||
			strings.Contains(strings.ToLower(skill.Name), lower) ||
			strings.Contains(strings.ToLower(skill.RepoID), lower) {
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

	catalogIndex := make(map[string]int, len(m.availableIDs))
	for i, skillID := range m.availableIDs {
		catalogIndex[strings.ToLower(skillID)] = i + 1
	}

	var sb strings.Builder
	sb.WriteString(infoStyle.Render(fmt.Sprintf("Found %d match(es) for %q", len(matches), query)) + "\n")

	limit := 40
	if len(matches) < limit {
		limit = len(matches)
	}

	for i := 0; i < limit; i++ {
		marker := " "
		if selectedSet[strings.ToLower(matches[i].ID)] {
			marker = "*"
		}
		idx := catalogIndex[strings.ToLower(matches[i].ID)]
		sb.WriteString(fmt.Sprintf(" %s %4d. %-48s (%s)\n", marker, idx, matches[i].ID, matches[i].RepoID))
	}
	if len(matches) > limit {
		sb.WriteString(mutedStyle.Render(fmt.Sprintf("... and %d more", len(matches)-limit)))
	}

	sb.WriteString("\n")
	sb.WriteString(mutedStyle.Render("Use /add <skill-id> or /add <catalog-number> to add a result."))
	return sb.String()
}

func (m Model) matchAvailableSkills(rawQuery string) []skillMatch {
	query := strings.ToLower(strings.TrimSpace(rawQuery))

	selectedSet := make(map[string]bool, len(m.cfg.SelectedSkills))
	for _, skillID := range m.cfg.SelectedSkills {
		selectedSet[strings.ToLower(skillID)] = true
	}

	type scoredSkillMatch struct {
		match skillMatch
		score int
	}

	scored := make([]scoredSkillMatch, 0, len(m.available))
	for i, skill := range m.available {
		score := 100
		if query != "" {
			computed, ok := scoreSkillSearch(query, skill)
			if !ok {
				continue
			}
			score = computed
		}

		scored = append(scored, scoredSkillMatch{
			match: skillMatch{
				Skill:        skill,
				CatalogIndex: i + 1,
				Selected:     selectedSet[strings.ToLower(skill.ID)],
			},
			score: score,
		})
	}

	if query != "" {
		sort.SliceStable(scored, func(i, j int) bool {
			if scored[i].score != scored[j].score {
				return scored[i].score < scored[j].score
			}
			if scored[i].match.Skill.Name != scored[j].match.Skill.Name {
				return scored[i].match.Skill.Name < scored[j].match.Skill.Name
			}
			return scored[i].match.Skill.ID < scored[j].match.Skill.ID
		})
	}

	matches := make([]skillMatch, 0, len(scored))
	for _, item := range scored {
		matches = append(matches, item.match)
	}

	return matches
}

func scoreSkillSearch(query string, skill config.AvailableSkill) (int, bool) {
	best := 0
	hasMatch := false

	consider := func(text string, weight int) {
		score, ok := scoreFuzzyText(query, strings.ToLower(text))
		if !ok {
			return
		}
		total := score + weight
		if !hasMatch || total < best {
			best = total
			hasMatch = true
		}
	}

	consider(skill.Name, 0)
	consider(skill.ID, 2)
	consider(skill.RepoID, 3)

	return best, hasMatch
}

func scoreFuzzyText(query, text string) (int, bool) {
	if query == "" {
		return 100, true
	}
	if text == "" {
		return 0, false
	}

	if text == query {
		return 0, true
	}
	if strings.HasPrefix(text, query) {
		return 1, true
	}
	if idx := strings.Index(text, query); idx >= 0 {
		return 2 + idx, true
	}

	qi := 0
	last := -1
	gapPenalty := 0
	for i := 0; i < len(text) && qi < len(query); i++ {
		if text[i] != query[qi] {
			continue
		}
		if last >= 0 {
			gapPenalty += i - last - 1
		}
		last = i
		qi++
	}
	if qi != len(query) {
		return 0, false
	}

	return 10 + gapPenalty, true
}

func (m *Model) actionAddSkill(raw string) string {
	tokens := config.InputCSV(raw)
	if len(tokens) == 0 {
		return warnStyle.Render("No skill names provided.")
	}

	requested, invalid := config.SplitByReference(tokens, m.availableIDs)

	var sb strings.Builder
	if len(invalid) > 0 {
		sb.WriteString(errorStyle.Render(fmt.Sprintf("Invalid catalog number(s): %s", strings.Join(invalid, ", "))) + "\n\n")
	}

	outcome := core.AddRequestedSkills(&m.cfg, requested, m.availableIDs)
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

	outcome := core.SyncSelectedSkills(m.cfg, m.available)
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
		availableSet[s.ID] = true
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
	sb.WriteString(fmt.Sprintf("Workspace path   : %s\n", config.CompactPath(m.paths.WorkspaceDir)))
	sb.WriteString(fmt.Sprintf("Config path      : %s\n", config.CompactPath(m.paths.ConfigPath)))
	sb.WriteString(fmt.Sprintf("Repositories     : %d\n", len(m.cfg.Repositories)))
	sb.WriteString(fmt.Sprintf("Catalog skills   : %d\n", len(m.available)))
	sb.WriteString(fmt.Sprintf("Selected skills  : %d\n", len(m.cfg.SelectedSkills)))
	sb.WriteString(fmt.Sprintf("Targets          : %d\n", len(m.cfg.Targets)))

	sb.WriteString(infoStyle.Render("\nRepository overview") + "\n")
	sb.WriteString(mutedStyle.Render(strings.Repeat("-", 64)) + "\n")
	for _, repo := range m.cfg.Repositories {
		localPath := m.paths.RepoPath(repo.ID)
		status := warnStyle.Render("missing")
		if info, err := os.Stat(localPath); err == nil && info.IsDir() {
			status = successStyle.Render("cloned")
		}
		sb.WriteString(fmt.Sprintf("- %s: %s\n", repo.ID, status))
	}

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
		selectedSet[config.SkillInstallDirName(s)] = true
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

func (m *Model) actionListRepos() string {
	if len(m.cfg.Repositories) == 0 {
		return warnStyle.Render("No repositories configured.")
	}

	var sb strings.Builder
	sb.WriteString(infoStyle.Render("Repositories") + "\n")
	sb.WriteString(mutedStyle.Render(strings.Repeat("-", 88)) + "\n")
	sb.WriteString(fmt.Sprintf("%-4s %-26s %-18s %s\n", "#", "ID", "Local", "URL"))
	sb.WriteString(mutedStyle.Render(strings.Repeat("-", 88)) + "\n")

	for i, repo := range m.cfg.Repositories {
		localPath := m.paths.RepoPath(repo.ID)
		status := errorStyle.Render("missing")
		if info, err := os.Stat(localPath); err == nil && info.IsDir() {
			status = successStyle.Render("cloned")
		}

		sb.WriteString(fmt.Sprintf("%-4d %-26s %-18s %s\n", i+1, repo.ID, status, repo.URL))
	}

	sb.WriteString(mutedStyle.Render(strings.Repeat("-", 88)) + "\n")
	sb.WriteString(mutedStyle.Render("Use /repo add <url>, /repo remove <index|id|url>, then /pull to fetch."))
	return sb.String()
}

func (m *Model) actionAddRepo(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return warnStyle.Render("Repository URL cannot be empty.")
	}

	repo, err := config.NormalizeRepository(raw)
	if err != nil {
		return errorStyle.Render("Invalid repository URL: " + err.Error())
	}

	for _, existing := range m.cfg.Repositories {
		if strings.EqualFold(existing.ID, repo.ID) {
			return warnStyle.Render("Repository already exists: " + existing.ID)
		}
		if strings.EqualFold(existing.URL, repo.URL) {
			return warnStyle.Render("Repository already exists: " + existing.URL)
		}
	}

	m.cfg.Repositories = append(m.cfg.Repositories, repo)
	if err := config.SaveConfig(m.paths, m.cfg); err != nil {
		return errorStyle.Render("Failed to save config: " + err.Error())
	}

	return successStyle.Render("Added repository: "+repo.ID) + "\n" + mutedStyle.Render("Run /pull to clone and index its skills.")
}

func (m *Model) actionRemoveRepo(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return warnStyle.Render("Repository index, id, or URL is required.")
	}

	if len(m.cfg.Repositories) == 0 {
		return warnStyle.Render("No repositories configured.")
	}

	repoIndex := -1
	if idx, err := strconv.Atoi(raw); err == nil {
		if idx < 1 || idx > len(m.cfg.Repositories) {
			return errorStyle.Render("Invalid repository index.")
		}
		repoIndex = idx - 1
	} else {
		for i, repo := range m.cfg.Repositories {
			if strings.EqualFold(repo.ID, raw) || strings.EqualFold(repo.URL, raw) {
				repoIndex = i
				break
			}
		}
		if repoIndex < 0 {
			normalized, err := config.NormalizeRepository(raw)
			if err == nil {
				for i, repo := range m.cfg.Repositories {
					if strings.EqualFold(repo.ID, normalized.ID) || strings.EqualFold(repo.URL, normalized.URL) {
						repoIndex = i
						break
					}
				}
			}
		}
		if repoIndex < 0 {
			return errorStyle.Render("Repository not found.")
		}
	}

	repo := m.cfg.Repositories[repoIndex]

	kept := make([]config.Repository, 0, len(m.cfg.Repositories)-1)
	for i, current := range m.cfg.Repositories {
		if i != repoIndex {
			kept = append(kept, current)
		}
	}
	m.cfg.Repositories = kept

	var removedSelected []string
	for _, selected := range m.cfg.SelectedSkills {
		if strings.HasPrefix(strings.ToLower(selected), strings.ToLower(repo.ID)+"/") {
			removedSelected = append(removedSelected, selected)
		}
	}

	removeOutcome := core.RemoveSelectedSkills(&m.cfg, removedSelected)
	_ = os.RemoveAll(m.paths.RepoPath(repo.ID))

	if err := config.SaveConfig(m.paths, m.cfg); err != nil {
		return errorStyle.Render("Failed to save config: " + err.Error())
	}

	m.refresh()

	var sb strings.Builder
	sb.WriteString(successStyle.Render("Removed repository: " + repo.ID))
	if len(removeOutcome.RemovedFromSelected) > 0 {
		sb.WriteString("\n")
		sb.WriteString(infoStyle.Render(fmt.Sprintf("Removed %d selected skill(s) from config.", len(removeOutcome.RemovedFromSelected))))
	}
	if len(removeOutcome.RemovedPaths) > 0 {
		sb.WriteString("\n")
		sb.WriteString(infoStyle.Render(fmt.Sprintf("Removed %d target folder(s).", len(removeOutcome.RemovedPaths))))
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
