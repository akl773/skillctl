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

	assert.Empty(t, cfg.SelectedSkills)
	assert.Empty(t, cfg.DisabledSkills)
	assert.NotEmpty(t, cfg.Targets)
	assert.GreaterOrEqual(t, len(cfg.Repositories), 4)

	repoIDs := make([]string, 0, len(cfg.Repositories))
	for _, repo := range cfg.Repositories {
		repoIDs = append(repoIDs, repo.ID)
	}

	assert.Contains(t, repoIDs, "vercel-labs-agent-skills")
	assert.Contains(t, repoIDs, "callstackincubator-agent-skills")
	assert.Contains(t, repoIDs, "tech-leads-club-agent-skills")
	assert.Contains(t, repoIDs, "composiohq-awesome-claude-skills")
}

func TestResolvePaths(t *testing.T) {
	t.Run("uses explicit workspace", func(t *testing.T) {
		workspace := t.TempDir()
		paths := ResolvePaths(workspace)

		assert.Equal(t, ExpandPath(workspace), paths.WorkspaceDir)
		assert.Equal(t, filepath.Join(ExpandPath(workspace), ".local"), paths.LocalDir)
		assert.Equal(t, filepath.Join(ExpandPath(workspace), ".local", "repos"), paths.RepoCacheDir)
	})

	t.Run("uses env var when explicit value is empty", func(t *testing.T) {
		workspace := t.TempDir()
		t.Setenv("SKILLCTL_WORKSPACE", workspace)

		paths := ResolvePaths("")
		assert.Equal(t, ExpandPath(workspace), paths.WorkspaceDir)
	})

	t.Run("falls back to default when explicit and env are empty", func(t *testing.T) {
		t.Setenv("SKILLCTL_WORKSPACE", "")

		paths := ResolvePaths("")
		assert.Equal(t, ExpandPath(DefaultWorkspaceDir), paths.WorkspaceDir)
	})
}

func TestEnsureSetup(t *testing.T) {
	t.Run("bootstraps local config and state", func(t *testing.T) {
		workspace := t.TempDir()
		paths := ResolvePaths(workspace)

		err := EnsureSetup(paths)
		require.NoError(t, err)

		cfgFile, err := os.ReadFile(paths.ConfigPath)
		require.NoError(t, err)
		assert.Contains(t, string(cfgFile), "selected_skills")
		assert.Contains(t, string(cfgFile), "repositories")

		stateFile, err := os.ReadFile(paths.StatePath)
		require.NoError(t, err)
		assert.Contains(t, string(stateFile), "{}")

		info, err := os.Stat(paths.RepoCacheDir)
		require.NoError(t, err)
		assert.True(t, info.IsDir())

		err = EnsureSetup(paths)
		require.NoError(t, err)
	})
}

func TestLoadAndSaveConfig(t *testing.T) {
	t.Run("load returns default on missing file", func(t *testing.T) {
		paths := ResolvePaths(t.TempDir())
		cfg := LoadConfig(paths)

		assert.Equal(t, DefaultConfig(), cfg)
	})

	t.Run("load returns default on invalid json", func(t *testing.T) {
		workspace := t.TempDir()
		paths := ResolvePaths(workspace)
		require.NoError(t, os.MkdirAll(paths.LocalDir, 0o755))
		require.NoError(t, os.WriteFile(paths.ConfigPath, []byte("{"), 0o644))

		cfg := LoadConfig(paths)
		assert.Equal(t, DefaultConfig(), cfg)
	})

	t.Run("normalizes selected, disabled, targets, and repositories", func(t *testing.T) {
		workspace := t.TempDir()
		paths := ResolvePaths(workspace)
		require.NoError(t, os.MkdirAll(paths.LocalDir, 0o755))

		raw := Config{
			SelectedSkills: []string{" alpha ", "beta", "alpha", " ", "Gamma"},
			DisabledSkills: []string{"beta", "BETA", "gamma", "missing"},
			Targets:        []string{" ~/one ", "~/two", "~/one", ""},
			Repositories: []Repository{
				{URL: "https://github.com/vercel-labs/agent-skills"},
				{URL: "git@github.com:vercel-labs/agent-skills.git"},
				{ID: "Custom_ID", URL: "github.com/callstackincubator/agent-skills"},
				{URL: "bad/repo/format/extra"},
			},
		}
		require.NoError(t, WriteJSON(paths.ConfigPath, raw))

		cfg := LoadConfig(paths)
		assert.Equal(t, []string{"alpha", "beta", "Gamma"}, cfg.SelectedSkills)
		assert.Equal(t, []string{"beta", "Gamma"}, cfg.DisabledSkills)
		assert.Equal(t, []string{"~/one", "~/two"}, cfg.Targets)
		require.Len(t, cfg.Repositories, 2)
		assert.Equal(t, "vercel-labs-agent-skills", cfg.Repositories[0].ID)
		assert.Equal(t, "https://github.com/vercel-labs/agent-skills.git", cfg.Repositories[0].URL)
		assert.Equal(t, "custom-id", cfg.Repositories[1].ID)
		assert.Equal(t, "https://github.com/callstackincubator/agent-skills.git", cfg.Repositories[1].URL)
	})

	t.Run("save and load round trip", func(t *testing.T) {
		workspace := t.TempDir()
		paths := ResolvePaths(workspace)

		want := Config{
			SelectedSkills: []string{"vercel-labs-agent-skills/react-best-practices"},
			DisabledSkills: nil,
			Targets:        []string{"~/.claude/skills", "~/.codex/skills"},
			Repositories: []Repository{
				{ID: "vercel-labs-agent-skills", URL: "https://github.com/vercel-labs/agent-skills.git"},
			},
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
	workspace := t.TempDir()
	paths := ResolvePaths(workspace)
	require.NoError(t, EnsureSetup(paths))

	repoA, err := NormalizeRepository("https://github.com/example/alpha")
	require.NoError(t, err)
	repoB, err := NormalizeRepository("https://github.com/example/beta")
	require.NoError(t, err)

	require.NoError(t, os.MkdirAll(filepath.Join(paths.RepoPath(repoA.ID), "skills", "alpha"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(paths.RepoPath(repoA.ID), "skills", "alpha", "SKILL.md"), []byte("alpha"), 0o644))

	require.NoError(t, os.MkdirAll(filepath.Join(paths.RepoPath(repoA.ID), "nested", "beta"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(paths.RepoPath(repoA.ID), "nested", "beta", "SKILL.md"), []byte("beta"), 0o644))

	require.NoError(t, os.MkdirAll(filepath.Join(paths.RepoPath(repoA.ID), "other", "alpha"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(paths.RepoPath(repoA.ID), "other", "alpha", "SKILL.md"), []byte("dup"), 0o644))

	require.NoError(t, os.MkdirAll(filepath.Join(paths.RepoPath(repoA.ID), ".claude", "skills", "hidden"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(paths.RepoPath(repoA.ID), ".claude", "skills", "hidden", "SKILL.md"), []byte("hidden"), 0o644))

	require.NoError(t, os.MkdirAll(filepath.Join(paths.RepoPath(repoB.ID), "pkg", "gamma"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(paths.RepoPath(repoB.ID), "pkg", "gamma", "SKILL.md"), []byte("gamma"), 0o644))

	cfg := Config{Repositories: []Repository{repoA, repoB}}
	skills := LoadAvailableSkills(paths, cfg)
	ids := SkillIDs(skills)

	assert.Len(t, ids, 4)
	assert.Contains(t, ids, repoA.ID+"/alpha")
	assert.Contains(t, ids, repoA.ID+"/beta")
	assert.Contains(t, ids, repoB.ID+"/gamma")

	var duplicateID string
	for _, id := range ids {
		if strings.HasPrefix(id, repoA.ID+"/") && strings.HasSuffix(id, "__alpha") {
			duplicateID = id
			break
		}
	}
	assert.NotEmpty(t, duplicateID)
}

func TestNormalizeRepository(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantID  string
		wantURL string
		wantErr bool
	}{
		{
			name:    "https",
			in:      "https://github.com/vercel-labs/agent-skills",
			wantID:  "vercel-labs-agent-skills",
			wantURL: "https://github.com/vercel-labs/agent-skills.git",
		},
		{
			name:    "ssh",
			in:      "git@github.com:callstackincubator/agent-skills.git",
			wantID:  "callstackincubator-agent-skills",
			wantURL: "https://github.com/callstackincubator/agent-skills.git",
		},
		{
			name:    "bare github path",
			in:      "github.com/tech-leads-club/agent-skills",
			wantID:  "tech-leads-club-agent-skills",
			wantURL: "https://github.com/tech-leads-club/agent-skills.git",
		},
		{
			name:    "invalid",
			in:      "not-a-repo",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, err := NormalizeRepository(tt.in)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantID, repo.ID)
			assert.Equal(t, tt.wantURL, repo.URL)
		})
	}
}

func TestSkillInstallDirName(t *testing.T) {
	assert.Equal(t,
		"vercel-labs-agent-skills--react-best-practices",
		SkillInstallDirName("vercel-labs-agent-skills/react-best-practices"),
	)
	assert.Equal(t,
		"skill--unknown",
		SkillInstallDirName(""),
	)
	assert.Equal(t,
		"repo--name-with-spaces",
		SkillInstallDirName("repo/name with spaces"),
	)
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
