package core

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"akhilsingh.in/skillctl/internal/config"
)

var (
	gitBinary   = resolveCommandPath("git")
	rsyncBinary = resolveCommandPath("rsync")
)

// --- Outcome types ---

// RepoPullResult captures clone/pull output for a single repository.
type RepoPullResult struct {
	RepoID       string
	RepoURL      string
	Action       string
	ReturnCode   int
	Stdout       string
	Stderr       string
	LocalRepoDir string
}

// Success returns true if this repository action exited cleanly.
func (r RepoPullResult) Success() bool { return r.ReturnCode == 0 }

// GitPullOutcome captures the result of updating all configured repositories.
type GitPullOutcome struct {
	Results []RepoPullResult
}

// Success returns true if all repository actions exited cleanly.
func (o GitPullOutcome) Success() bool {
	for _, result := range o.Results {
		if !result.Success() {
			return false
		}
	}
	return true
}

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

// RunGitPull updates all configured repositories without streaming output.
func RunGitPull(paths config.AppPaths, repositories []config.Repository) GitPullOutcome {
	return RunGitPullStream(paths, repositories, nil, nil)
}

// RunGitPullStream updates all configured repositories and streams output chunks.
func RunGitPullStream(
	paths config.AppPaths,
	repositories []config.Repository,
	onStdout func(string),
	onStderr func(string),
) GitPullOutcome {
	_ = os.MkdirAll(paths.RepoCacheDir, 0o755)

	outcome := GitPullOutcome{Results: make([]RepoPullResult, len(repositories))}
	var wg sync.WaitGroup

	for i, repo := range repositories {
		i := i
		repo := repo
		wg.Add(1)

		go func() {
			defer wg.Done()

			repoPath := paths.RepoPath(repo.ID)
			action := "pull"

			if !exists(repoPath) {
				action = "clone"
			}

			if onStdout != nil {
				onStdout(fmt.Sprintf("\n[%s] %s\n", repo.ID, strings.ToUpper(action)))
			}

			var cmd *exec.Cmd
			if action == "clone" {
				_ = os.MkdirAll(filepath.Dir(repoPath), 0o755)
				cmd = exec.Command(gitBinary, "clone", "--depth", "1", "--progress", repo.URL, repoPath)
			} else {
				cmd = exec.Command(gitBinary, "-C", repoPath, "pull", "--ff-only", "--progress")
			}

			rc, stdout, stderr := runCommandStream(cmd, onStdout, onStderr)
			outcome.Results[i] = RepoPullResult{
				RepoID:       repo.ID,
				RepoURL:      repo.URL,
				Action:       action,
				ReturnCode:   rc,
				Stdout:       stdout,
				Stderr:       stderr,
				LocalRepoDir: repoPath,
			}
		}()
	}

	wg.Wait()
	return outcome
}

func runCommandStream(cmd *exec.Cmd, onStdout func(string), onStderr func(string)) (int, string, string) {
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return 1, "", err.Error()
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return 1, "", err.Error()
	}

	if err := cmd.Start(); err != nil {
		return 1, "", err.Error()
	}

	var stdout, stderr strings.Builder
	var wg sync.WaitGroup
	wg.Add(2)

	go copyStream(stdoutPipe, &stdout, onStdout, &wg)
	go copyStream(stderrPipe, &stderr, onStderr, &wg)

	err = cmd.Wait()
	wg.Wait()

	rc := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			rc = exitErr.ExitCode()
		} else {
			rc = 1
		}
	}

	return rc, stdout.String(), stderr.String()
}

func copyStream(r io.Reader, dst *strings.Builder, onChunk func(string), wg *sync.WaitGroup) {
	defer wg.Done()
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			chunk := string(buf[:n])
			dst.WriteString(chunk)
			if onChunk != nil {
				onChunk(chunk)
			}
		}
		if err != nil {
			if err == io.EOF {
				return
			}
			if dst.Len() == 0 || !strings.HasSuffix(dst.String(), "\n") {
				dst.WriteString("\n")
			}
			dst.WriteString(err.Error())
			if onChunk != nil {
				onChunk("\n" + err.Error())
			}
			return
		}
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

	// Keep disabled list aligned with selected skills
	var disabledKept []string
	for _, s := range cfg.DisabledSkills {
		if !removeSet[s] {
			disabledKept = append(disabledKept, s)
		}
	}
	cfg.DisabledSkills = disabledKept

	// Collect removed names
	for name := range removeSet {
		outcome.RemovedFromSelected = append(outcome.RemovedFromSelected, name)
	}
	sort.Strings(outcome.RemovedFromSelected)

	// Remove from target directories
	for _, targetRaw := range cfg.Targets {
		targetPath := config.ExpandPath(targetRaw)
		for skill := range removeSet {
			installDir := config.SkillInstallDirName(skill)
			skillPath := filepath.Join(targetPath, installDir)
			if exists(skillPath) {
				removePath(skillPath)
				outcome.RemovedPaths = append(outcome.RemovedPaths, config.CompactPath(skillPath))
			}
		}
	}

	return outcome
}

// SyncSelectedSkills rsyncs selected skills to all configured targets.
func SyncSelectedSkills(cfg config.Config, available []config.AvailableSkill) SyncOutcome {
	availableByID := make(map[string]config.AvailableSkill, len(available))
	for _, skill := range available {
		availableByID[skill.ID] = skill
	}

	var syncable []config.AvailableSkill
	var missing []string
	for _, skillID := range cfg.SelectedSkills {
		skill, ok := availableByID[skillID]
		if ok {
			syncable = append(syncable, skill)
		} else {
			missing = append(missing, skillID)
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
			dst := filepath.Join(targetPath, config.SkillInstallDirName(skill.ID))
			_ = os.MkdirAll(dst, 0o755)

			cmd := exec.Command(rsyncBinary, "-a", "--delete", skill.SourcePath+"/", dst+"/")
			out, err := cmd.CombinedOutput()
			if err != nil {
				result.Failed[skill.ID] = strings.TrimSpace(string(out))
			} else {
				result.Synced = append(result.Synced, skill.ID)
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

func resolveCommandPath(name string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		return name
	}
	return path
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
