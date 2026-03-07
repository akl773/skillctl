package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"akhilsingh.in/skillctl/internal/config"
)

// --- Outcome types ---

// GitPullOutcome captures the result of a git pull operation.
type GitPullOutcome struct {
	ReturnCode int
	Stdout     string
	Stderr     string
}

// Success returns true if the git pull exited cleanly.
func (o GitPullOutcome) Success() bool { return o.ReturnCode == 0 }

// AddOutcome captures the result of adding skills to the selection.
type AddOutcome struct {
	Added           []string
	AlreadySelected []string
	Missing         []MissingSuggestion
}

// MissingSuggestion pairs a missing skill name with close-match suggestions.
type MissingSuggestion struct {
	Name        string
	Suggestions []string
}

// RemoveOutcome captures the result of removing skills.
type RemoveOutcome struct {
	RemovedFromSelected []string
	NotSelected         []string
	RemovedPaths        []string
}

// TargetSyncResult captures sync results for a single target directory.
type TargetSyncResult struct {
	Target string
	Synced []string
	Failed map[string]string
}

// SyncOutcome captures the result of syncing skills to all targets.
type SyncOutcome struct {
	MissingInSource []string
	TargetResults   []TargetSyncResult
}

// TotalSynced returns the total number of skills synced across all targets.
func (o SyncOutcome) TotalSynced() int {
	n := 0
	for _, r := range o.TargetResults {
		n += len(r.Synced)
	}
	return n
}

// TotalFailed returns the total number of failed syncs across all targets.
func (o SyncOutcome) TotalFailed() int {
	n := 0
	for _, r := range o.TargetResults {
		n += len(r.Failed)
	}
	return n
}

// --- Operations ---

// RunGitPull executes git pull --ff-only on the source repository.
func RunGitPull(paths config.AppPaths) GitPullOutcome {
	cmd := exec.Command("git", "-C", paths.SourceRepo, "pull", "--ff-only")
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	rc := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			rc = exitErr.ExitCode()
		} else {
			rc = 1
		}
	}

	return GitPullOutcome{
		ReturnCode: rc,
		Stdout:     stdout.String(),
		Stderr:     stderr.String(),
	}
}

// AddRequestedSkills adds the requested skills to the config's selected list.
func AddRequestedSkills(cfg *config.Config, requested, available []string) AddOutcome {
	selectedLower := make(map[string]bool)
	for _, s := range cfg.SelectedSkills {
		selectedLower[strings.ToLower(s)] = true
	}

	var outcome AddOutcome
	for _, name := range requested {
		resolved := config.ResolveCaseInsensitive(name, available)
		if resolved == "" {
			suggestions := closestMatches(name, available, 3)
			outcome.Missing = append(outcome.Missing, MissingSuggestion{
				Name:        name,
				Suggestions: suggestions,
			})
			continue
		}

		if selectedLower[strings.ToLower(resolved)] {
			outcome.AlreadySelected = append(outcome.AlreadySelected, resolved)
			continue
		}

		cfg.SelectedSkills = append(cfg.SelectedSkills, resolved)
		selectedLower[strings.ToLower(resolved)] = true
		outcome.Added = append(outcome.Added, resolved)
	}

	cfg.SelectedSkills = config.UniqueOrdered(cfg.SelectedSkills)
	return outcome
}

// RemoveSelectedSkills removes skills from selection and deletes them from targets.
func RemoveSelectedSkills(cfg *config.Config, requested []string) RemoveOutcome {
	selectedMap := make(map[string]string)
	for _, s := range cfg.SelectedSkills {
		selectedMap[strings.ToLower(s)] = s
	}

	var outcome RemoveOutcome
	removeSet := make(map[string]bool)

	for _, tok := range requested {
		resolved, ok := selectedMap[strings.ToLower(tok)]
		if !ok {
			outcome.NotSelected = append(outcome.NotSelected, tok)
			continue
		}
		removeSet[resolved] = true
	}

	if len(removeSet) == 0 {
		return outcome
	}

	// Update selected list
	var kept []string
	for _, s := range cfg.SelectedSkills {
		if !removeSet[s] {
			kept = append(kept, s)
		}
	}
	cfg.SelectedSkills = kept

	// Collect removed names
	for name := range removeSet {
		outcome.RemovedFromSelected = append(outcome.RemovedFromSelected, name)
	}
	sort.Strings(outcome.RemovedFromSelected)

	// Remove from target directories
	for _, targetRaw := range cfg.Targets {
		targetPath := config.ExpandPath(targetRaw)
		for skill := range removeSet {
			skillPath := filepath.Join(targetPath, skill)
			if exists(skillPath) {
				removePath(skillPath)
				outcome.RemovedPaths = append(outcome.RemovedPaths, config.CompactPath(skillPath))
			}
		}
	}

	return outcome
}

// SyncSelectedSkills rsyncs selected skills to all configured targets.
func SyncSelectedSkills(paths config.AppPaths, cfg config.Config, available []string) SyncOutcome {
	availableSet := make(map[string]bool)
	for _, s := range available {
		availableSet[s] = true
	}

	var syncable []string
	var missing []string
	for _, s := range cfg.SelectedSkills {
		if availableSet[s] {
			syncable = append(syncable, s)
		} else {
			missing = append(missing, s)
		}
	}

	outcome := SyncOutcome{MissingInSource: missing}
	if len(syncable) == 0 || len(cfg.Targets) == 0 {
		return outcome
	}

	for _, targetRaw := range cfg.Targets {
		targetPath := config.ExpandPath(targetRaw)
		_ = os.MkdirAll(targetPath, 0o755)

		result := TargetSyncResult{
			Target: targetPath,
			Failed: make(map[string]string),
		}

		for _, skill := range syncable {
			src := filepath.Join(paths.SourceSkillDir, skill)
			dst := filepath.Join(targetPath, skill)
			_ = os.MkdirAll(dst, 0o755)

			cmd := exec.Command("rsync", "-a", "--delete", src+"/", dst+"/")
			out, err := cmd.CombinedOutput()
			if err != nil {
				result.Failed[skill] = strings.TrimSpace(string(out))
			} else {
				result.Synced = append(result.Synced, skill)
			}
		}

		outcome.TargetResults = append(outcome.TargetResults, result)
	}

	return outcome
}

// --- Internal helpers ---

// closestMatches returns up to n close matches for name from candidates.
// Uses a simple substring-based approach; could be upgraded to Levenshtein.
func closestMatches(name string, candidates []string, n int) []string {
	lower := strings.ToLower(name)
	var matches []string
	for _, c := range candidates {
		if strings.Contains(strings.ToLower(c), lower) || strings.Contains(lower, strings.ToLower(c)) {
			matches = append(matches, c)
			if len(matches) >= n {
				break
			}
		}
	}
	return matches
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func removePath(path string) {
	info, err := os.Lstat(path)
	if err != nil {
		return
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		_ = os.Remove(path)
	} else {
		_ = os.RemoveAll(path)
	}
}
