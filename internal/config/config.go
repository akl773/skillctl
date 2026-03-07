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

// DefaultSourceRepo is the default path to the skills source repository.
var DefaultSourceRepo = filepath.Join(homeDir(), ".skills-curated")

// AppPaths holds all resolved filesystem paths used by the application.
type AppPaths struct {
	SourceRepo     string
	LocalDir       string
	ConfigPath     string
	StatePath      string
	SourceSkillDir string
	GitExcludePath string
}

// Config holds the user's selected skills and target directories.
type Config struct {
	SelectedSkills []string `json:"selected_skills"`
	DisabledSkills []string `json:"disabled_skills"`
	Targets        []string `json:"targets"`
}

// State holds runtime state persisted between sessions.
type State struct {
	LastSyncAt string `json:"last_sync_at,omitempty"`
}

// DefaultConfig returns the factory-default configuration.
func DefaultConfig() Config {
	return Config{
		SelectedSkills: []string{
			"ui-ux-pro-max",
			"code-reviewer",
			"security-auditor",
			"architect-review",
			"vibe-code-auditor",
			"seo-audit",
		},
		DisabledSkills: []string{},
		Targets: []string{
			"~/.claude/skills",
			"~/.config/opencode/skills",
			"~/.gemini/antigravity/skills",
			"~/.cursor/skills/antigravity-awesome-skills/skills",
			"~/.codex/skills",
			"~/.kiro/skills",
		},
	}
}

// ProtectedTargetDirNames are directory names that should never be pruned.
var ProtectedTargetDirNames = map[string]bool{
	".system": true,
}

// ResolvePaths builds the full set of application paths.
func ResolvePaths(sourceRepo string) AppPaths {
	repo := sourceRepo
	if repo == "" {
		repo = os.Getenv("SKILLCTL_SOURCE_REPO")
	}
	if repo == "" {
		repo = DefaultSourceRepo
	}
	repo = ExpandPath(repo)

	localDir := filepath.Join(repo, ".local")
	return AppPaths{
		SourceRepo:     repo,
		LocalDir:       localDir,
		ConfigPath:     filepath.Join(localDir, "skillctl.json"),
		StatePath:      filepath.Join(localDir, "state.json"),
		SourceSkillDir: filepath.Join(repo, "skills"),
		GitExcludePath: filepath.Join(repo, ".git", "info", "exclude"),
	}
}

// EnsureSetup validates the source repo and bootstraps config/state files.
func EnsureSetup(paths AppPaths) error {
	info, err := os.Stat(paths.SourceSkillDir)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("skills source directory not found: %s", CompactPath(paths.SourceSkillDir))
	}

	gitDir := filepath.Join(paths.SourceRepo, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		return fmt.Errorf("source repo is not a git repository: %s", CompactPath(paths.SourceRepo))
	}

	if err := os.MkdirAll(paths.LocalDir, 0o755); err != nil {
		return fmt.Errorf("cannot create local directory: %w", err)
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

	ensureLocalExclude(paths)
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

// LoadAvailableSkills scans the source skills directory and returns sorted names.
func LoadAvailableSkills(paths AppPaths) []string {
	entries, err := os.ReadDir(paths.SourceSkillDir)
	if err != nil {
		return nil
	}

	var skills []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			skills = append(skills, e.Name())
		}
	}
	sort.Strings(skills)
	return skills
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
	return os.WriteFile(path, b, 0o644)
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

func ensureLocalExclude(paths AppPaths) {
	excludePath := paths.GitExcludePath
	_ = os.MkdirAll(filepath.Dir(excludePath), 0o755)

	content, _ := os.ReadFile(excludePath)
	lines := strings.Split(string(content), "\n")

	for _, line := range lines {
		if line == ".local/" {
			return
		}
	}

	text := string(content)
	if len(text) > 0 && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	text += ".local/\n"
	_ = os.WriteFile(excludePath, []byte(text), 0o644)
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
