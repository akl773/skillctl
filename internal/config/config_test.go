package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.NotEmpty(t, cfg.SelectedSkills)
	assert.Empty(t, cfg.DisabledSkills)
	assert.NotEmpty(t, cfg.Targets)
	assert.Contains(t, cfg.SelectedSkills, "code-reviewer")
	assert.Contains(t, cfg.Targets, "~/.claude/skills")
}

func TestResolvePaths(t *testing.T) {
	t.Run("uses explicit source repo", func(t *testing.T) {
		repo := t.TempDir()
		paths := ResolvePaths(repo)

		assert.Equal(t, ExpandPath(repo), paths.SourceRepo)
		assert.Equal(t, filepath.Join(ExpandPath(repo), ".local"), paths.LocalDir)
		assert.Equal(t, filepath.Join(ExpandPath(repo), "skills"), paths.SourceSkillDir)
	})

	t.Run("uses env var when explicit is empty", func(t *testing.T) {
		repo := t.TempDir()
		t.Setenv("SKILLCTL_SOURCE_REPO", repo)

		paths := ResolvePaths("")
		assert.Equal(t, ExpandPath(repo), paths.SourceRepo)
	})

	t.Run("falls back to default when arg and env are empty", func(t *testing.T) {
		t.Setenv("SKILLCTL_SOURCE_REPO", "")

		paths := ResolvePaths("")
		assert.Equal(t, ExpandPath(DefaultSourceRepo), paths.SourceRepo)
	})
}

func TestEnsureSetup(t *testing.T) {
	t.Run("bootstraps local config and state", func(t *testing.T) {
		repo := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(repo, "skills"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(repo, ".git", "info"), 0o755))

		paths := ResolvePaths(repo)
		err := EnsureSetup(paths)
		require.NoError(t, err)

		cfgFile, err := os.ReadFile(paths.ConfigPath)
		require.NoError(t, err)
		assert.Contains(t, string(cfgFile), "selected_skills")

		stateFile, err := os.ReadFile(paths.StatePath)
		require.NoError(t, err)
		assert.Contains(t, string(stateFile), "{}")

		excludeFile, err := os.ReadFile(paths.GitExcludePath)
		require.NoError(t, err)
		assert.Contains(t, string(excludeFile), ".local/")

		// idempotent and should not duplicate the exclude entry
		err = EnsureSetup(paths)
		require.NoError(t, err)

		excludeFile, err = os.ReadFile(paths.GitExcludePath)
		require.NoError(t, err)
		assert.Equal(t, 1, strings.Count(string(excludeFile), ".local/"))
	})

	t.Run("returns error when skills directory is missing", func(t *testing.T) {
		repo := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(repo, ".git"), 0o755))

		err := EnsureSetup(ResolvePaths(repo))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "skills source directory not found")
	})

	t.Run("returns error when repo is not a git directory", func(t *testing.T) {
		repo := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(repo, "skills"), 0o755))

		err := EnsureSetup(ResolvePaths(repo))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "source repo is not a git repository")
	})
}

func TestLoadAndSaveConfig(t *testing.T) {
	t.Run("load returns default on missing file", func(t *testing.T) {
		paths := ResolvePaths(t.TempDir())
		cfg := LoadConfig(paths)

		assert.Equal(t, DefaultConfig(), cfg)
	})

	t.Run("load returns default on invalid json", func(t *testing.T) {
		repo := t.TempDir()
		paths := ResolvePaths(repo)
		require.NoError(t, os.MkdirAll(paths.LocalDir, 0o755))
		require.NoError(t, os.WriteFile(paths.ConfigPath, []byte("{"), 0o644))

		cfg := LoadConfig(paths)
		assert.Equal(t, DefaultConfig(), cfg)
	})

	t.Run("normalizes selected, disabled, and targets", func(t *testing.T) {
		repo := t.TempDir()
		paths := ResolvePaths(repo)
		require.NoError(t, os.MkdirAll(paths.LocalDir, 0o755))

		raw := Config{
			SelectedSkills: []string{" alpha ", "beta", "alpha", " ", "Gamma"},
			DisabledSkills: []string{"beta", "BETA", "gamma", "missing"},
			Targets:        []string{" ~/one ", "~/two", "~/one", ""},
		}
		require.NoError(t, WriteJSON(paths.ConfigPath, raw))

		cfg := LoadConfig(paths)
		assert.Equal(t, []string{"alpha", "beta", "Gamma"}, cfg.SelectedSkills)
		assert.Equal(t, []string{"beta", "Gamma"}, cfg.DisabledSkills)
		assert.Equal(t, []string{"~/one", "~/two"}, cfg.Targets)
	})

	t.Run("save and load round trip", func(t *testing.T) {
		repo := t.TempDir()
		paths := ResolvePaths(repo)

		want := Config{
			SelectedSkills: []string{"code-reviewer", "security-auditor"},
			DisabledSkills: []string{"security-auditor"},
			Targets:        []string{"~/.claude/skills", "~/.codex/skills"},
		}

		require.NoError(t, SaveConfig(paths, want))
		got := LoadConfig(paths)

		assert.Equal(t, want, got)
	})
}

func TestLoadAndSaveState(t *testing.T) {
	t.Run("load returns zero value when state is missing", func(t *testing.T) {
		paths := ResolvePaths(t.TempDir())
		st := LoadState(paths)
		assert.Equal(t, State{}, st)
	})

	t.Run("save and load state round trip", func(t *testing.T) {
		paths := ResolvePaths(t.TempDir())
		want := State{LastSyncAt: "2026-03-07T09:00:00Z"}

		require.NoError(t, SaveState(paths, want))
		got := LoadState(paths)

		assert.Equal(t, want, got)
	})
}

func TestLoadAvailableSkills(t *testing.T) {
	repo := t.TempDir()
	paths := ResolvePaths(repo)
	require.NoError(t, os.MkdirAll(paths.SourceSkillDir, 0o755))

	require.NoError(t, os.MkdirAll(filepath.Join(paths.SourceSkillDir, "zeta"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(paths.SourceSkillDir, "alpha"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(paths.SourceSkillDir, ".hidden"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(paths.SourceSkillDir, "README.md"), []byte("file"), 0o644))

	skills := LoadAvailableSkills(paths)
	assert.Equal(t, []string{"alpha", "zeta"}, skills)
}

func TestReadWriteJSON(t *testing.T) {
	type sample struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}

	path := filepath.Join(t.TempDir(), "nested", "sample.json")
	want := sample{Name: "skillctl", Count: 3}

	require.NoError(t, WriteJSON(path, want))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(string(data), "\n"))

	var got sample
	require.NoError(t, ReadJSON(path, &got))
	assert.Equal(t, want, got)
}

func TestExpandPath(t *testing.T) {
	home := homeDir()
	rel := filepath.Join(".", "tmp")

	tests := []struct {
		name string
		in   string
		out  string
	}{
		{name: "tilde", in: "~", out: home},
		{name: "tilde child", in: "~/skills", out: filepath.Join(home, "skills")},
		{name: "absolute stays absolute", in: home, out: home},
		{name: "relative becomes absolute", in: rel, out: mustAbs(t, rel)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.out, ExpandPath(tt.in))
		})
	}
}

func TestCompactPath(t *testing.T) {
	home := homeDir()

	tests := []struct {
		name string
		in   string
		out  string
	}{
		{name: "exact home", in: home, out: "~"},
		{name: "child under home", in: filepath.Join(home, "skills"), out: "~/skills"},
		{name: "outside home unchanged", in: "/tmp/skills", out: "/tmp/skills"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.out, CompactPath(tt.in))
		})
	}
}

func TestUniqueOrdered(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		out  []string
	}{
		{name: "nil input", in: nil, out: nil},
		{name: "no duplicates", in: []string{"a", "b"}, out: []string{"a", "b"}},
		{name: "removes duplicates preserves order", in: []string{"b", "a", "b", "c", "a"}, out: []string{"b", "a", "c"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.out, UniqueOrdered(tt.in))
		})
	}
}

func TestInputCSV(t *testing.T) {
	tests := []struct {
		name string
		in   string
		out  []string
	}{
		{name: "empty", in: "", out: nil},
		{name: "spaces only", in: "   ", out: nil},
		{name: "single", in: "alpha", out: []string{"alpha"}},
		{name: "multiple trimmed", in: " alpha, beta ,gamma ", out: []string{"alpha", "beta", "gamma"}},
		{name: "ignores empty tokens", in: "alpha,,beta, ", out: []string{"alpha", "beta"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.out, InputCSV(tt.in))
		})
	}
}

func TestSplitByReference(t *testing.T) {
	values := []string{"alpha", "beta", "gamma"}

	tests := []struct {
		name        string
		tokens      []string
		wantPicked  []string
		wantInvalid []string
	}{
		{
			name:        "numeric references",
			tokens:      []string{"1", "3"},
			wantPicked:  []string{"alpha", "gamma"},
			wantInvalid: nil,
		},
		{
			name:        "mixed references and names",
			tokens:      []string{"2", "delta"},
			wantPicked:  []string{"beta", "delta"},
			wantInvalid: nil,
		},
		{
			name:        "invalid references",
			tokens:      []string{"0", "4"},
			wantPicked:  nil,
			wantInvalid: []string{"0", "4"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			picked, invalid := SplitByReference(tt.tokens, values)
			assert.Equal(t, tt.wantPicked, picked)
			assert.Equal(t, tt.wantInvalid, invalid)
		})
	}
}

func TestResolveCaseInsensitive(t *testing.T) {
	values := []string{"Alpha", "beta", "GAMMA"}

	assert.Equal(t, "Alpha", ResolveCaseInsensitive("alpha", values))
	assert.Equal(t, "beta", ResolveCaseInsensitive("BETA", values))
	assert.Equal(t, "", ResolveCaseInsensitive("delta", values))
}

func TestCleanStrings(t *testing.T) {
	in := []string{" alpha ", "", "  ", "beta"}
	out := cleanStrings(in)
	assert.Equal(t, []string{"alpha", "beta"}, out)
}

func TestIsDigit(t *testing.T) {
	assert.True(t, isDigit("1"))
	assert.True(t, isDigit("12345"))
	assert.False(t, isDigit(""))
	assert.False(t, isDigit("a1"))
	assert.False(t, isDigit("-10"))
}

func TestAtoi(t *testing.T) {
	assert.Equal(t, 0, atoi("0"))
	assert.Equal(t, 7, atoi("7"))
	assert.Equal(t, 1234, atoi("1234"))
}

func TestWriteJSONProducesValidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	want := Config{SelectedSkills: []string{"alpha"}, Targets: []string{"~/t"}}

	require.NoError(t, WriteJSON(path, want))

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var got Config
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, want.SelectedSkills, got.SelectedSkills)
	assert.Equal(t, want.Targets, got.Targets)
}

func mustAbs(t *testing.T, p string) string {
	t.Helper()
	abs, err := filepath.Abs(p)
	require.NoError(t, err)
	return abs
}
