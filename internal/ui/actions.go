package ui

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"akhilsingh.in/skillctl/internal/config"
	"akhilsingh.in/skillctl/internal/core"
)

// --- Action handlers ---

func (m *Model) actionGitPull() commandResult {
	if m.gitPullRunning {
		return commandResult{Output: warnStyle.Render("An upstream sync is already running.")}
	}
	if len(m.cfg.Repositories) == 0 {
		return commandResult{Output: warnStyle.Render("No repositories configured. Add one with /add <url>.")}
	}

	m.gitPullRunning = true
	m.gitPullSilent = false
	m.gitPullOutput.Reset()
	m.gitPullOutput.WriteString(infoStyle.Render("Syncing upstream skill sources...") + "\n")

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
	sb.WriteString(mutedStyle.Render("ℹ️ Use /skills <index|name> to add or remove skills."))

	return sb.String()
}

func (m *Model) actionSearch(query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return warnStyle.Render("Search query cannot be empty.")
	}

	matches := m.matchAvailableSkills(query)
	if len(matches) == 0 {
		return warnStyle.Render("No matching skills found.")
	}

	var sb strings.Builder
	sb.WriteString(infoStyle.Render(fmt.Sprintf("Found %d match(es) for %q", len(matches), query)) + "\n")

	limit := 40
	if len(matches) < limit {
		limit = len(matches)
	}

	for i := 0; i < limit; i++ {
		marker := " "
		if matches[i].Selected {
			marker = "*"
		}
		sb.WriteString(fmt.Sprintf(" %s %4d. %-48s (%s)\n", marker, matches[i].CatalogIndex, matches[i].Skill.ID, matches[i].Skill.RepoID))
	}
	if len(matches) > limit {
		sb.WriteString(mutedStyle.Render(fmt.Sprintf("... and %d more", len(matches)-limit)))
	}

	sb.WriteString("\n")
	sb.WriteString(mutedStyle.Render("Use /skills <skill-id> or /skills <catalog-number> to toggle a result."))
	return sb.String()
}

func (m *Model) actionToggleSkillSelection(raw string) string {
	tokens := config.InputCSV(raw)
	if len(tokens) == 0 {
		return warnStyle.Render("No skill names provided.")
	}

	requested, invalid := config.SplitByReference(tokens, m.availableIDs)
	requested = config.UniqueOrdered(requested)

	selectedMap := make(map[string]string, len(m.cfg.SelectedSkills))
	for _, skill := range m.cfg.SelectedSkills {
		selectedMap[strings.ToLower(skill)] = skill
	}

	addRequested := make([]string, 0, len(requested))
	removeRequested := make([]string, 0, len(requested))
	for _, token := range requested {
		if selected, ok := selectedMap[strings.ToLower(token)]; ok {
			removeRequested = append(removeRequested, selected)
			continue
		}
		addRequested = append(addRequested, token)
	}

	var sb strings.Builder
	if len(invalid) > 0 {
		sb.WriteString(errorStyle.Render(fmt.Sprintf("Invalid catalog number(s): %s", strings.Join(invalid, ", "))) + "\n\n")
	}

	sb.WriteString(m.applySkillSelectionChanges(addRequested, removeRequested))
	return sb.String()
}

func (m *Model) applySkillSelectionChanges(addRequested, removeRequested []string) string {
	addRequested = config.UniqueOrdered(addRequested)
	removeRequested = config.UniqueOrdered(removeRequested)

	addOutcome := core.AddOutcome{}
	removeOutcome := core.RemoveOutcome{}

	if len(addRequested) > 0 {
		addOutcome = core.AddRequestedSkills(&m.cfg, addRequested, m.availableIDs)
	}
	if len(removeRequested) > 0 {
		removeOutcome = core.RemoveSelectedSkills(&m.cfg, removeRequested)
	}

	if len(addOutcome.Added) > 0 || len(removeOutcome.RemovedFromSelected) > 0 {
		_ = config.SaveConfig(m.paths, m.cfg)
	}

	addText := strings.TrimSpace(formatAddOutcome(addOutcome))
	removeText := strings.TrimSpace(formatRemoveOutcome(removeOutcome))
	autoSyncText := ""
	if len(addOutcome.Added) > 0 {
		autoSyncOutcome := core.SyncSelectedSkills(m.cfg, m.available)
		autoSyncText = strings.TrimSpace(formatSyncOutcome(m.cfg, autoSyncOutcome))
		if autoSyncText != "" {
			autoSyncText = infoStyle.Render("Auto-deploy after selection") + "\n" + autoSyncText
		}
	}

	parts := []string{}
	if addText != "" {
		parts = append(parts, addText)
	}
	if removeText != "" {
		parts = append(parts, removeText)
	}
	if autoSyncText != "" {
		parts = append(parts, autoSyncText)
	}

	switch len(parts) {
	case 0:
		return infoStyle.Render("No selection changes.")
	default:
		return strings.Join(parts, "\n\n")
	}
}

func (m Model) matchAvailableSkills(rawQuery string) []skillMatch {
	query := buildSearchQuery(rawQuery)

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
		if query.raw != "" {
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

	if query.raw != "" {
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

type searchQuery struct {
	raw        string
	normalized string
	compact    string
}

func buildSearchQuery(raw string) searchQuery {
	query := strings.ToLower(strings.TrimSpace(raw))
	return searchQuery{
		raw:        query,
		normalized: normalizeSearchText(query),
		compact:    compactSearchText(query),
	}
}

func scoreSkillSearch(query searchQuery, skill config.AvailableSkill) (int, bool) {
	best := 0
	hasMatch := false

	consider := func(text string, weight int) {
		score, ok := scoreSearchText(query, text, weight)
		if !ok {
			return
		}
		if !hasMatch || score < best {
			best = score
			hasMatch = true
		}
	}

	consider(skill.Name, 0)
	consider(skill.ID, 2)
	consider(skill.RepoID, 3)

	return best, hasMatch
}

func scoreSearchText(query searchQuery, rawText string, fieldWeight int) (int, bool) {
	if query.raw == "" {
		return 100 + fieldWeight, true
	}

	text := strings.ToLower(strings.TrimSpace(rawText))
	if text == "" {
		return 0, false
	}

	normalized := normalizeSearchText(text)
	compact := compactSearchText(text)

	best := 0
	hasMatch := false

	consider := func(rankTier, detail int) {
		total := rankTier*10000 + fieldWeight*1000 + detail
		if !hasMatch || total < best {
			best = total
			hasMatch = true
		}
	}

	if text == query.raw {
		consider(0, 0)
	}
	if query.normalized != "" && normalized == query.normalized {
		consider(1, 0)
	}
	if query.compact != "" && compact == query.compact {
		consider(1, 1)
	}

	if strings.HasPrefix(text, query.raw) {
		consider(2, len(text)-len(query.raw))
	}
	if query.normalized != "" && strings.HasPrefix(normalized, query.normalized) {
		consider(3, len(normalized)-len(query.normalized))
	}
	if query.compact != "" && strings.HasPrefix(compact, query.compact) {
		consider(3, len(compact)-len(query.compact)+1)
	}

	if idx := strings.Index(text, query.raw); idx >= 0 {
		consider(4, idx)
	}

	if query.compact != "" {
		if penalty, ok := scoreFuzzySubsequence(query.compact, compact); ok {
			consider(5, penalty)
		}
	}

	return best, hasMatch
}

func scoreFuzzySubsequence(query, text string) (int, bool) {
	if query == "" {
		return 0, true
	}
	if text == "" {
		return 0, false
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

	spanPenalty := 0
	if last >= 0 {
		spanPenalty = len(text) - last - 1
	}

	return gapPenalty + spanPenalty, true
}

func normalizeSearchText(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(raw))
	lastWasSep := true
	for _, r := range raw {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastWasSep = false
			continue
		}
		if !lastWasSep {
			b.WriteByte(' ')
			lastWasSep = true
		}
	}

	return strings.TrimSpace(b.String())
}

func compactSearchText(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(raw))
	for _, r := range raw {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}

	return b.String()
}

func (m *Model) actionSync() string {
	if len(m.cfg.SelectedSkills) == 0 {
		return warnStyle.Render("No skills selected. Add skills first.")
	}
	if len(m.cfg.Targets) == 0 {
		return warnStyle.Render("No targets configured. Add targets first.")
	}

	outcome := core.SyncSelectedSkills(m.cfg, m.available)
	return formatSyncOutcome(m.cfg, outcome)
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
		if repo.SourceType() == config.RepositoryTypeLocal {
			localPath = config.ExpandPath(repo.Path)
			status = warnStyle.Render("unavailable")
			if info, err := os.Stat(localPath); err == nil && info.IsDir() {
				status = successStyle.Render("ready")
			}
		} else if info, err := os.Stat(localPath); err == nil && info.IsDir() {
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
	sb.WriteString(fmt.Sprintf("%-4s %-26s %-18s %s\n", "#", "ID", "Status", "Source"))
	sb.WriteString(mutedStyle.Render(strings.Repeat("-", 88)) + "\n")

	for i, repo := range m.cfg.Repositories {
		localPath := m.paths.RepoPath(repo.ID)
		status := errorStyle.Render("missing")
		source := repo.URL
		if repo.SourceType() == config.RepositoryTypeLocal {
			localPath = config.ExpandPath(repo.Path)
			source = config.CompactPath(localPath)
			if info, err := os.Stat(localPath); err == nil && info.IsDir() {
				status = successStyle.Render("local")
			}
		} else if info, err := os.Stat(localPath); err == nil && info.IsDir() {
			status = successStyle.Render("cloned")
		}

		sb.WriteString(fmt.Sprintf("%-4d %-26s %-18s %s\n", i+1, repo.ID, status, source))
	}

	sb.WriteString(mutedStyle.Render(strings.Repeat("-", 88)) + "\n")
	sb.WriteString(mutedStyle.Render("Use /add <url> and /repo remove <index|id|url>. Repositories auto-update on launch; run /pull to sync upstream sources now."))
	return sb.String()
}

func (m *Model) actionAddRepo(raw string) commandResult {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return commandResult{Output: warnStyle.Render("Repository URL cannot be empty.")}
	}

	repo, err := config.NormalizeRepository(raw)
	if err != nil {
		return commandResult{Output: errorStyle.Render("Invalid repository URL: " + err.Error())}
	}

	for _, existing := range m.cfg.Repositories {
		if strings.EqualFold(existing.ID, repo.ID) {
			return commandResult{Output: warnStyle.Render("Repository already exists: " + existing.ID)}
		}
		if strings.EqualFold(existing.URL, repo.URL) {
			return commandResult{Output: warnStyle.Render("Repository already exists: " + existing.URL)}
		}
	}

	m.cfg.Repositories = append(m.cfg.Repositories, repo)
	m.cfg.RemovedDefaultRepos = removeStringCaseInsensitive(m.cfg.RemovedDefaultRepos, repo.ID)
	if err := config.SaveConfig(m.paths, m.cfg); err != nil {
		return commandResult{Output: errorStyle.Render("Failed to save config: " + err.Error())}
	}

	if m.gitPullRunning {
		return commandResult{
			Output: successStyle.Render("Added repository: "+repo.ID) + "\n" + mutedStyle.Render("An upstream sync is already running. Saved the repository and skipped starting another sync."),
		}
	}

	m.gitPullRunning = true
	m.gitPullSilent = false
	m.gitPullOutput.Reset()
	m.gitPullOutput.WriteString(successStyle.Render("Added repository: "+repo.ID) + "\n")
	m.gitPullOutput.WriteString(infoStyle.Render("Syncing upstream skill source...") + "\n")

	return commandResult{
		Output: m.gitPullOutput.String(),
		Cmd:    startGitPullStreamCmd(m.paths, []config.Repository{repo}),
	}
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
	defaultRepoIDs := defaultRepoIDSet()

	kept := make([]config.Repository, 0, len(m.cfg.Repositories)-1)
	for i, current := range m.cfg.Repositories {
		if i != repoIndex {
			kept = append(kept, current)
		}
	}
	m.cfg.Repositories = kept
	if defaultRepoIDs[strings.ToLower(repo.ID)] {
		m.cfg.RemovedDefaultRepos = config.UniqueOrdered(append(m.cfg.RemovedDefaultRepos, repo.ID))
	}

	var removedSelected []string
	for _, selected := range m.cfg.SelectedSkills {
		if strings.HasPrefix(strings.ToLower(selected), strings.ToLower(repo.ID)+"/") {
			removedSelected = append(removedSelected, selected)
		}
	}

	removeOutcome := core.RemoveSelectedSkills(&m.cfg, removedSelected)
	if repo.SourceType() == config.RepositoryTypeLocal {
		_ = os.RemoveAll(config.ExpandPath(repo.Path))
	} else {
		_ = os.RemoveAll(m.paths.RepoPath(repo.ID))
	}

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

func defaultRepoIDSet() map[string]bool {
	set := make(map[string]bool)
	for _, repo := range config.DefaultRepositories() {
		set[strings.ToLower(repo.ID)] = true
	}
	return set
}

func removeStringCaseInsensitive(items []string, value string) []string {
	if len(items) == 0 {
		return nil
	}

	needle := strings.ToLower(strings.TrimSpace(value))
	kept := make([]string, 0, len(items))
	for _, item := range items {
		if strings.ToLower(strings.TrimSpace(item)) == needle {
			continue
		}
		kept = append(kept, item)
	}

	if len(kept) == 0 {
		return nil
	}

	return kept
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

func formatRemoveOutcome(outcome core.RemoveOutcome) string {
	var sb strings.Builder

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
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(warnStyle.Render(fmt.Sprintf("Not in selected list: %s", strings.Join(outcome.NotSelected, ", "))))
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

func formatSyncOutcome(cfg config.Config, outcome core.SyncOutcome) string {
	if len(outcome.TargetResults) == 0 {
		result := errorStyle.Render("Nothing to sync.")
		if len(outcome.MissingInSource) > 0 {
			result += "\n" + formatMissing(outcome.MissingInSource)
		}
		return result
	}

	var sb strings.Builder
	sb.WriteString(infoStyle.Render(fmt.Sprintf("Syncing %d skill(s) -> %d target(s)",
		len(cfg.SelectedSkills), len(cfg.Targets))) + "\n")

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
