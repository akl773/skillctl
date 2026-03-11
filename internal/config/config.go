package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Version is the application version, injected at build time via -ldflags.
var Version = "0.1.1"

// DefaultWorkspaceDir is the default workspace path for skillctl metadata.
var DefaultWorkspaceDir = filepath.Join(homeDir(), ".skillctl")

var defaultRepositoryURLs = []string{
	"https://github.com/vercel-labs/agent-skills.git",
	"https://github.com/callstackincubator/agent-skills.git",
	"https://github.com/tech-leads-club/agent-skills.git",
	"https://github.com/ComposioHQ/awesome-claude-skills.git",
	"https://github.com/sickn33/antigravity-awesome-skills.git",
}

// AppPaths holds all resolved filesystem paths used by the application.
type AppPaths struct {
	WorkspaceDir string
	LocalDir     string
	ConfigPath   string
	StatePath    string
	RepoCacheDir string
}

// RepoPath returns the local clone path for a repository id.
func (p AppPaths) RepoPath(repoID string) string {
	return filepath.Join(p.RepoCacheDir, repoID)
}

// Repository identifies an upstream git repository that contains skills.
type Repository struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

// Config holds user-selected skills, targets, and configured repositories.
type Config struct {
	SelectedSkills      []string     `json:"selected_skills"`
	DisabledSkills      []string     `json:"disabled_skills"`
	Targets             []string     `json:"targets"`
	Repositories        []Repository `json:"repositories"`
	RemovedDefaultRepos []string     `json:"removed_default_repos,omitempty"`
}

// State holds runtime state persisted between sessions.
type State struct {
	LastSyncAt string `json:"last_sync_at,omitempty"`
}

// AvailableSkill describes one discoverable skill from a local repository clone.
type AvailableSkill struct {
	ID         string
	Name       string
	RepoID     string
	RepoURL    string
	SourcePath string
}

// DefaultConfig returns the factory-default configuration.
func DefaultConfig() Config {
	return Config{
		SelectedSkills: []string{},
		DisabledSkills: []string{},
		Targets: []string{
			"~/.claude/skills",
			"~/.config/opencode/skills",
			"~/.gemini/antigravity/skills",
			"~/.cursor/skills/antigravity-awesome-skills/skills",
			"~/.codex/skills",
			"~/.kiro/skills",
		},
		Repositories: DefaultRepositories(),
	}
}

// DefaultRepositories returns the built-in source repositories.
func DefaultRepositories() []Repository {
	repos := make([]Repository, 0, len(defaultRepositoryURLs))
	for _, raw := range defaultRepositoryURLs {
		repo, err := NormalizeRepository(raw)
		if err != nil {
			continue
		}
		repos = append(repos, repo)
	}
	return repos
}

// ProtectedTargetDirNames are directory names that should never be pruned.
var ProtectedTargetDirNames = map[string]bool{
	".system": true,
}

// ResolvePaths builds the full set of application paths.
func ResolvePaths(workspaceDir string) AppPaths {
	root := workspaceDir
	if root == "" {
		root = os.Getenv("SKILLCTL_WORKSPACE")
	}
	if root == "" {
		root = DefaultWorkspaceDir
	}
	root = ExpandPath(root)

	localDir := filepath.Join(root, ".local")
	return AppPaths{
		WorkspaceDir: root,
		LocalDir:     localDir,
		ConfigPath:   filepath.Join(localDir, "skillctl.json"),
		StatePath:    filepath.Join(localDir, "state.json"),
		RepoCacheDir: filepath.Join(localDir, "repos"),
	}
}

// EnsureSetup bootstraps workspace directories and config/state files.
func EnsureSetup(paths AppPaths) error {
	if err := os.MkdirAll(paths.WorkspaceDir, 0o755); err != nil {
		return fmt.Errorf("cannot create workspace directory: %w", err)
	}

	if err := os.MkdirAll(paths.RepoCacheDir, 0o755); err != nil {
		return fmt.Errorf("cannot create repository cache directory: %w", err)
	}

	if _, err := os.Stat(paths.ConfigPath); os.IsNotExist(err) {
		cfg := DefaultConfig()
		if err := WriteJSON(paths.ConfigPath, cfg); err != nil {
			return fmt.Errorf("cannot write default config: %w", err)
		}
	}

	if _, err := os.Stat(paths.StatePath); os.IsNotExist(err) {
		if err := WriteJSON(paths.StatePath, State{}); err != nil {
			return fmt.Errorf("cannot write default state: %w", err)
		}
	}

	return nil
}

// LoadConfig reads the config file and returns a validated Config.
func LoadConfig(paths AppPaths) Config {
	var cfg Config
	if err := ReadJSON(paths.ConfigPath, &cfg); err != nil {
		return DefaultConfig()
	}

	cfg.SelectedSkills = UniqueOrdered(cleanStrings(cfg.SelectedSkills))
	cfg.DisabledSkills = UniqueOrdered(cleanStrings(cfg.DisabledSkills))

	selectedByLower := make(map[string]string)
	for _, skill := range cfg.SelectedSkills {
		selectedByLower[strings.ToLower(skill)] = skill
	}

	var normalizedDisabled []string
	seenDisabled := make(map[string]bool)
	for _, skill := range cfg.DisabledSkills {
		resolved, ok := selectedByLower[strings.ToLower(skill)]
		if !ok || seenDisabled[resolved] {
			continue
		}
		normalizedDisabled = append(normalizedDisabled, resolved)
		seenDisabled[resolved] = true
	}
	cfg.DisabledSkills = normalizedDisabled

	cfg.Targets = UniqueOrdered(cleanStrings(cfg.Targets))
	cfg.Repositories = normalizeRepositories(cfg.Repositories)
	cfg.RemovedDefaultRepos = normalizeRepoIDs(cfg.RemovedDefaultRepos)

	cfg, migrated := mergeMissingDefaultRepositories(cfg)
	if migrated {
		_ = SaveConfig(paths, cfg)
	}

	return cfg
}

// SaveConfig persists the config to disk.
func SaveConfig(paths AppPaths, cfg Config) error {
	return WriteJSON(paths.ConfigPath, cfg)
}

// LoadState reads the state file.
func LoadState(paths AppPaths) State {
	var st State
	if err := ReadJSON(paths.StatePath, &st); err != nil {
		return State{}
	}
	return st
}

// SaveState persists the state to disk.
func SaveState(paths AppPaths, st State) error {
	return WriteJSON(paths.StatePath, st)
}

// LoadAvailableSkills discovers skills from all configured local repository clones.
func LoadAvailableSkills(paths AppPaths, cfg Config) []AvailableSkill {
	var skills []AvailableSkill
	seenIDs := make(map[string]bool)

	for _, repo := range cfg.Repositories {
		repoPath := paths.RepoPath(repo.ID)
		if info, err := os.Stat(repoPath); err != nil || !info.IsDir() {
			continue
		}

		_ = filepath.WalkDir(repoPath, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}

			rel, err := filepath.Rel(repoPath, path)
			if err != nil {
				return nil
			}

			if rel == "." {
				return nil
			}

			if hasHiddenPathSegment(rel) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			if d.IsDir() || d.Name() != "SKILL.md" {
				return nil
			}

			skillDir := filepath.Dir(path)
			skillName := filepath.Base(skillDir)
			skillID := repo.ID + "/" + skillName

			if seenIDs[skillID] {
				relDir, relErr := filepath.Rel(repoPath, skillDir)
				if relErr == nil {
					skillID = repo.ID + "/" + strings.ReplaceAll(relDir, string(filepath.Separator), "__")
				}
			}

			if seenIDs[skillID] {
				return nil
			}

			skills = append(skills, AvailableSkill{
				ID:         skillID,
				Name:       skillName,
				RepoID:     repo.ID,
				RepoURL:    repo.URL,
				SourcePath: skillDir,
			})
			seenIDs[skillID] = true
			return nil
		})
	}

	sort.Slice(skills, func(i, j int) bool {
		if skills[i].ID != skills[j].ID {
			return skills[i].ID < skills[j].ID
		}
		return skills[i].SourcePath < skills[j].SourcePath
	})

	return skills
}

// SkillIDs returns IDs from a catalog list.
func SkillIDs(skills []AvailableSkill) []string {
	ids := make([]string, 0, len(skills))
	for _, skill := range skills {
		ids = append(ids, skill.ID)
	}
	return ids
}

// SkillInstallDirName returns the target directory name for a skill id.
func SkillInstallDirName(skillID string) string {
	repoID := "skill"
	skillName := skillID
	if slash := strings.Index(skillID, "/"); slash >= 0 {
		repoID = skillID[:slash]
		skillName = skillID[slash+1:]
	}

	repoID = sanitizePathComponent(repoID)
	skillName = sanitizePathComponent(skillName)
	if repoID == "" {
		repoID = "skill"
	}
	if skillName == "" {
		skillName = "unknown"
	}
	return repoID + "--" + skillName
}

// NormalizeRepository parses and normalizes a repository reference.
func NormalizeRepository(raw string) (Repository, error) {
	owner, repo, err := parseGitHubRepo(raw)
	if err != nil {
		return Repository{}, err
	}
	return Repository{
		ID:  sanitizeRepoID(owner + "-" + repo),
		URL: fmt.Sprintf("https://github.com/%s/%s.git", owner, repo),
	}, nil
}

// --- JSON helpers ---

// ReadJSON reads a JSON file into the target struct.
func ReadJSON(path string, target interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

// WriteJSON writes a struct as formatted JSON to a file.
func WriteJSON(path string, data interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o600)
}

// --- Path helpers ---

// ExpandPath expands ~ to the home directory and returns an absolute path.
func ExpandPath(raw string) string {
	if strings.HasPrefix(raw, "~/") {
		raw = filepath.Join(homeDir(), raw[2:])
	} else if raw == "~" {
		raw = homeDir()
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		return raw
	}
	return abs
}

// CompactPath replaces the home directory prefix with ~.
func CompactPath(path string) string {
	home := homeDir()
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+string(filepath.Separator)) {
		return "~" + path[len(home):]
	}
	return path
}

// --- String helpers ---

// UniqueOrdered deduplicates a slice while preserving order.
func UniqueOrdered(items []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			out = append(out, item)
		}
	}
	return out
}

// InputCSV splits a comma-separated input string into trimmed tokens.
func InputCSV(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// SplitByReference resolves numeric references (1-indexed) against a list.
// Returns resolved names and invalid tokens.
func SplitByReference(tokens, values []string) (picked, invalid []string) {
	for _, tok := range tokens {
		if isDigit(tok) {
			idx := atoi(tok)
			if idx >= 1 && idx <= len(values) {
				picked = append(picked, values[idx-1])
			} else {
				invalid = append(invalid, tok)
			}
			continue
		}
		picked = append(picked, tok)
	}
	return
}

// ResolveCaseInsensitive finds a case-insensitive match in a list.
func ResolveCaseInsensitive(name string, values []string) string {
	lower := strings.ToLower(name)
	for _, v := range values {
		if strings.ToLower(v) == lower {
			return v
		}
	}
	return ""
}

// --- internal helpers ---

func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return "/"
	}
	return h
}

func cleanStrings(items []string) []string {
	var out []string
	for _, s := range items {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func normalizeRepositories(repos []Repository) []Repository {
	if len(repos) == 0 {
		return nil
	}

	var out []Repository
	seenIDs := make(map[string]bool)
	seenURLs := make(map[string]bool)

	for _, repo := range repos {
		normalized, err := NormalizeRepository(repo.URL)
		if err != nil {
			continue
		}

		if repo.ID != "" {
			normalized.ID = sanitizeRepoID(strings.ToLower(strings.TrimSpace(repo.ID)))
		}
		if normalized.ID == "" {
			continue
		}

		urlKey := strings.ToLower(normalized.URL)
		idKey := strings.ToLower(normalized.ID)
		if seenURLs[urlKey] || seenIDs[idKey] {
			continue
		}

		seenURLs[urlKey] = true
		seenIDs[idKey] = true
		out = append(out, normalized)
	}

	return out
}

func normalizeRepoIDs(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}

	var out []string
	seen := make(map[string]bool)
	for _, id := range ids {
		normalized := sanitizeRepoID(strings.ToLower(strings.TrimSpace(id)))
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		out = append(out, normalized)
	}

	if len(out) == 0 {
		return nil
	}

	return out
}

func mergeMissingDefaultRepositories(cfg Config) (Config, bool) {
	defaults := DefaultRepositories()
	if len(defaults) == 0 {
		return cfg, false
	}

	removed := make(map[string]bool, len(cfg.RemovedDefaultRepos))
	for _, id := range cfg.RemovedDefaultRepos {
		removed[strings.ToLower(id)] = true
	}

	seenIDs := make(map[string]bool, len(cfg.Repositories))
	seenURLs := make(map[string]bool, len(cfg.Repositories))
	for _, repo := range cfg.Repositories {
		seenIDs[strings.ToLower(repo.ID)] = true
		seenURLs[strings.ToLower(repo.URL)] = true
	}

	migrated := false
	for _, repo := range defaults {
		idKey := strings.ToLower(repo.ID)
		urlKey := strings.ToLower(repo.URL)
		if removed[idKey] || seenIDs[idKey] || seenURLs[urlKey] {
			continue
		}

		cfg.Repositories = append(cfg.Repositories, repo)
		seenIDs[idKey] = true
		seenURLs[urlKey] = true
		migrated = true
	}

	return cfg, migrated
}

func hasHiddenPathSegment(relPath string) bool {
	parts := strings.Split(relPath, string(filepath.Separator))
	for _, part := range parts {
		if strings.HasPrefix(part, ".") {
			return true
		}
	}
	return false
}

func parseGitHubRepo(raw string) (owner string, repo string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", fmt.Errorf("repository URL cannot be empty")
	}

	rest := raw
	if strings.HasPrefix(rest, "git@github.com:") {
		rest = strings.TrimPrefix(rest, "git@github.com:")
	} else if strings.HasPrefix(rest, "https://github.com/") {
		rest = strings.TrimPrefix(rest, "https://github.com/")
	} else if strings.HasPrefix(rest, "http://github.com/") {
		rest = strings.TrimPrefix(rest, "http://github.com/")
	} else if strings.HasPrefix(rest, "github.com/") {
		rest = strings.TrimPrefix(rest, "github.com/")
	}

	rest = strings.TrimSpace(rest)
	rest = strings.TrimSuffix(rest, ".git")
	rest = strings.Trim(rest, "/")

	parts := strings.Split(rest, "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", fmt.Errorf("invalid github repository: %s", raw)
	}

	owner = strings.ToLower(strings.TrimSpace(parts[0]))
	repo = strings.ToLower(strings.TrimSpace(parts[1]))
	return owner, repo, nil
}

func sanitizeRepoID(s string) string {
	if s == "" {
		return ""
	}

	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(s) {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}

	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "repo"
	}
	return out
}

func sanitizePathComponent(s string) string {
	if s == "" {
		return ""
	}

	var b strings.Builder
	lastDash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '.' ||
			r == '_' ||
			r == '-' {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}

	return strings.Trim(b.String(), "-")
}

func isDigit(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}
