package core

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"akhilsingh.in/skillctl/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddRequestedSkills(t *testing.T) {
	available := []string{"alpha", "beta", "code-reviewer", "security-auditor"}

	t.Run("adds missing selected skills with case-insensitive resolution", func(t *testing.T) {
		cfg := config.Config{SelectedSkills: []string{"alpha"}}

		outcome := AddRequestedSkills(&cfg, []string{"BETA"}, available)

		assert.Equal(t, []string{"beta"}, outcome.Added)
		assert.Empty(t, outcome.AlreadySelected)
		assert.Empty(t, outcome.Missing)
		assert.Equal(t, []string{"alpha", "beta"}, cfg.SelectedSkills)
	})

	t.Run("reports already selected skills", func(t *testing.T) {
		cfg := config.Config{SelectedSkills: []string{"alpha"}}

		outcome := AddRequestedSkills(&cfg, []string{"alpha", "ALPHA"}, available)

		assert.Empty(t, outcome.Added)
		assert.Equal(t, []string{"alpha", "alpha"}, outcome.AlreadySelected)
		assert.Empty(t, outcome.Missing)
		assert.Equal(t, []string{"alpha"}, cfg.SelectedSkills)
	})

	t.Run("reports missing skills with suggestions", func(t *testing.T) {
		cfg := config.Config{SelectedSkills: []string{"alpha"}}

		outcome := AddRequestedSkills(&cfg, []string{"review"}, available)

		require.Len(t, outcome.Missing, 1)
		assert.Equal(t, "review", outcome.Missing[0].Name)
		assert.Equal(t, []string{"code-reviewer"}, outcome.Missing[0].Suggestions)
		assert.Empty(t, outcome.Added)
		assert.Equal(t, []string{"alpha"}, cfg.SelectedSkills)
	})

	t.Run("deduplicates existing selected skills while preserving order", func(t *testing.T) {
		cfg := config.Config{SelectedSkills: []string{"alpha", "alpha"}}

		outcome := AddRequestedSkills(&cfg, []string{"beta"}, available)

		assert.Equal(t, []string{"beta"}, outcome.Added)
		assert.Equal(t, []string{"alpha", "beta"}, cfg.SelectedSkills)
	})
}

func TestRemoveSelectedSkills(t *testing.T) {
	t.Run("removes selected skills, keeps disabled aligned, and deletes target paths", func(t *testing.T) {
		target := t.TempDir()
		alphaID := "repo/Alpha"
		betaID := "repo/beta"
		alphaPath := filepath.Join(target, config.SkillInstallDirName(alphaID))
		betaPath := filepath.Join(target, config.SkillInstallDirName(betaID))

		require.NoError(t, os.MkdirAll(alphaPath, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(alphaPath, "README.md"), []byte("alpha"), 0o644))
		require.NoError(t, os.MkdirAll(betaPath, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(betaPath, "README.md"), []byte("beta"), 0o644))

		cfg := config.Config{
			SelectedSkills: []string{alphaID, betaID},
			DisabledSkills: []string{alphaID, betaID},
			Targets:        []string{target},
		}

		outcome := RemoveSelectedSkills(&cfg, []string{"REPO/alpha"})

		assert.Equal(t, []string{alphaID}, outcome.RemovedFromSelected)
		assert.Empty(t, outcome.NotSelected)
		assert.Equal(t, []string{betaID}, cfg.SelectedSkills)
		assert.Equal(t, []string{betaID}, cfg.DisabledSkills)
		assert.ElementsMatch(t, []string{alphaPath}, outcome.RemovedPaths)
		assert.False(t, exists(alphaPath))
		assert.True(t, exists(betaPath))
	})

	t.Run("collects not-selected names and returns early when nothing is removed", func(t *testing.T) {
		cfg := config.Config{
			SelectedSkills: []string{"alpha"},
			DisabledSkills: []string{"alpha"},
			Targets:        []string{t.TempDir()},
		}

		outcome := RemoveSelectedSkills(&cfg, []string{"missing"})

		assert.Empty(t, outcome.RemovedFromSelected)
		assert.Equal(t, []string{"missing"}, outcome.NotSelected)
		assert.Empty(t, outcome.RemovedPaths)
		assert.Equal(t, []string{"alpha"}, cfg.SelectedSkills)
		assert.Equal(t, []string{"alpha"}, cfg.DisabledSkills)
	})

	t.Run("supports case-insensitive removal and sorts removed names", func(t *testing.T) {
		target := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(target, config.SkillInstallDirName("repo/beta")), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(target, config.SkillInstallDirName("repo/gamma")), 0o755))

		cfg := config.Config{
			SelectedSkills: []string{"repo/gamma", "repo/beta", "repo/alpha"},
			DisabledSkills: []string{"repo/gamma", "repo/alpha"},
			Targets:        []string{target},
		}

		outcome := RemoveSelectedSkills(&cfg, []string{"REPO/BETA", "repo/gamma"})

		assert.Equal(t, []string{"repo/beta", "repo/gamma"}, outcome.RemovedFromSelected)
		assert.Equal(t, []string{"repo/alpha"}, cfg.SelectedSkills)
		assert.Equal(t, []string{"repo/alpha"}, cfg.DisabledSkills)
	})
}

func TestClosestMatches(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		candidates []string
		n          int
		want       []string
	}{
		{
			name:       "substring match",
			query:      "review",
			candidates: []string{"code-reviewer", "security-auditor", "review-helper"},
			n:          3,
			want:       []string{"code-reviewer", "review-helper"},
		},
		{
			name:       "candidate contained in query",
			query:      "code-reviewer-extended",
			candidates: []string{"code-reviewer", "review"},
			n:          3,
			want:       []string{"code-reviewer", "review"},
		},
		{
			name:       "respects max result limit",
			query:      "a",
			candidates: []string{"alpha", "beta", "gamma"},
			n:          1,
			want:       []string{"alpha"},
		},
		{
			name:       "no matches",
			query:      "zzz",
			candidates: []string{"alpha", "beta"},
			n:          3,
			want:       nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, closestMatches(tt.query, tt.candidates, tt.n))
		})
	}
}

func TestGitPullOutcomeSuccess(t *testing.T) {
	assert.True(t, GitPullOutcome{}.Success())
	assert.True(t, GitPullOutcome{Results: []RepoPullResult{{RepoID: "a", ReturnCode: 0}}}.Success())
	assert.False(t, GitPullOutcome{Results: []RepoPullResult{{RepoID: "a", ReturnCode: 1}}}.Success())
}

func TestRunGitPullStreamProcessesReposInParallel(t *testing.T) {
	paths := config.AppPaths{RepoCacheDir: filepath.Join(t.TempDir(), "repos")}
	repositories := []config.Repository{
		{ID: "org/repo-one", URL: "https://example.com/repo-one.git"},
		{ID: "org/repo-two", URL: "https://example.com/repo-two.git"},
		{ID: "org/repo-three", URL: "https://example.com/repo-three.git"},
	}

	gitPath := installFakeGitBinary(t)
	originalGitBinary := gitBinary
	gitBinary = gitPath
	t.Cleanup(func() { gitBinary = originalGitBinary })
	t.Setenv("SKILLCTL_FAKE_GIT_SLEEP_SECS", "1")

	start := time.Now()
	outcome := RunGitPullStream(paths, repositories, nil, nil)
	elapsed := time.Since(start)

	require.Len(t, outcome.Results, len(repositories))
	for i, result := range outcome.Results {
		assert.Equal(t, repositories[i].ID, result.RepoID)
		assert.Equal(t, repositories[i].URL, result.RepoURL)
		assert.Equal(t, "clone", result.Action)
		assert.Equal(t, 0, result.ReturnCode)
	}

	assert.Less(t, elapsed, 3*time.Second)
}

func TestRunGitPullStreamPreservesResultOrderAndAction(t *testing.T) {
	paths := config.AppPaths{RepoCacheDir: filepath.Join(t.TempDir(), "repos")}
	repositories := []config.Repository{
		{ID: "org/already-one", URL: "https://example.com/already-one.git"},
		{ID: "org/new-two", URL: "https://example.com/new-two.git"},
		{ID: "org/already-three", URL: "https://example.com/already-three.git"},
	}

	require.NoError(t, os.MkdirAll(paths.RepoPath(repositories[0].ID), 0o755))
	require.NoError(t, os.MkdirAll(paths.RepoPath(repositories[2].ID), 0o755))

	gitPath := installFakeGitBinary(t)
	originalGitBinary := gitBinary
	gitBinary = gitPath
	t.Cleanup(func() { gitBinary = originalGitBinary })

	outcome := RunGitPullStream(paths, repositories, nil, nil)

	require.Len(t, outcome.Results, len(repositories))

	assert.Equal(t, repositories[0].ID, outcome.Results[0].RepoID)
	assert.Equal(t, "pull", outcome.Results[0].Action)
	assert.Equal(t, 0, outcome.Results[0].ReturnCode)

	assert.Equal(t, repositories[1].ID, outcome.Results[1].RepoID)
	assert.Equal(t, "clone", outcome.Results[1].Action)
	assert.Equal(t, 0, outcome.Results[1].ReturnCode)

	assert.Equal(t, repositories[2].ID, outcome.Results[2].RepoID)
	assert.Equal(t, "pull", outcome.Results[2].Action)
	assert.Equal(t, 0, outcome.Results[2].ReturnCode)
}

func TestRunGitPullStreamSkipsLocalSources(t *testing.T) {
	paths := config.AppPaths{RepoCacheDir: filepath.Join(t.TempDir(), "repos")}
	localSource := filepath.Join(t.TempDir(), "imported-skills")
	require.NoError(t, os.MkdirAll(localSource, 0o755))

	repositories := []config.Repository{
		{ID: "skillctl-imported", Type: config.RepositoryTypeLocal, Path: localSource},
	}

	outcome := RunGitPullStream(paths, repositories, nil, nil)
	require.Len(t, outcome.Results, 1)
	assert.Equal(t, "skip", outcome.Results[0].Action)
	assert.Equal(t, 0, outcome.Results[0].ReturnCode)
	assert.Equal(t, localSource, outcome.Results[0].LocalRepoDir)
}

func installFakeGitBinary(t *testing.T) string {
	t.Helper()

	binDir := t.TempDir()
	gitPath := filepath.Join(binDir, "git")
	script := fmt.Sprintf(`#!/bin/sh
set -eu

sleep_secs=${SKILLCTL_FAKE_GIT_SLEEP_SECS:-0}
if [ "$sleep_secs" -gt 0 ]; then
  sleep "$sleep_secs"
fi

if [ "$#" -gt 0 ] && [ "$1" = "clone" ]; then
  target=""
  for arg in "$@"; do
    target="$arg"
  done
  mkdir -p "$target"
  printf 'cloned %%s\n' "$target"
  exit 0
fi

if [ "$#" -ge 3 ] && [ "$1" = "-C" ] && [ "$3" = "pull" ]; then
  printf 'pulled %%s\n' "$2"
  exit 0
fi

printf 'unexpected args: %%s\n' "$*" >&2
exit 1
`)

	require.NoError(t, os.WriteFile(gitPath, []byte(script), 0o755))
	return gitPath
}

func TestSyncSelectedSkills(t *testing.T) {
	t.Run("reports missing selected skill ids", func(t *testing.T) {
		cfg := config.Config{SelectedSkills: []string{"repo/missing"}, Targets: []string{t.TempDir()}}
		outcome := SyncSelectedSkills(cfg, nil)

		assert.Equal(t, []string{"repo/missing"}, outcome.MissingInSource)
		assert.Empty(t, outcome.TargetResults)
	})

	t.Run("syncs available skill to namespaced target directory", func(t *testing.T) {
		target := t.TempDir()
		source := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("content"), 0o644))

		skill := config.AvailableSkill{
			ID:         "repo/alpha",
			RepoID:     "repo",
			Name:       "alpha",
			SourcePath: source,
		}

		cfg := config.Config{
			SelectedSkills: []string{"repo/alpha"},
			Targets:        []string{target},
		}

		outcome := SyncSelectedSkills(cfg, []config.AvailableSkill{skill})
		require.Len(t, outcome.TargetResults, 1)
		assert.Equal(t, []string{"repo/alpha"}, outcome.TargetResults[0].Synced)

		installedPath := filepath.Join(target, config.SkillInstallDirName("repo/alpha"), "SKILL.md")
		assert.True(t, exists(installedPath))
	})
}

func TestSyncOutcomeTotals(t *testing.T) {
	outcome := SyncOutcome{
		TargetResults: []TargetSyncResult{
			{Target: "a", Synced: []string{"s1", "s2"}, Failed: map[string]string{"x": "err"}},
			{Target: "b", Synced: []string{"s3"}, Failed: map[string]string{"y": "err", "z": "err"}},
		},
	}

	assert.Equal(t, 3, outcome.TotalSynced())
	assert.Equal(t, 3, outcome.TotalFailed())
}

func TestExists(t *testing.T) {
	tmp := t.TempDir()
	file := filepath.Join(tmp, "file.txt")
	require.NoError(t, os.WriteFile(file, []byte("ok"), 0o644))

	assert.True(t, exists(file))
	assert.False(t, exists(filepath.Join(tmp, "missing")))
}

func TestRemovePath(t *testing.T) {
	t.Run("removes regular file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "file.txt")
		require.NoError(t, os.WriteFile(path, []byte("ok"), 0o644))

		removePath(path)
		assert.False(t, exists(path))
	})

	t.Run("removes directory recursively", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "skill")
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "nested"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "nested", "data.txt"), []byte("ok"), 0o644))

		removePath(dir)
		assert.False(t, exists(dir))
	})

	t.Run("removes symlink only", func(t *testing.T) {
		tmp := t.TempDir()
		target := filepath.Join(tmp, "target.txt")
		link := filepath.Join(tmp, "link.txt")

		require.NoError(t, os.WriteFile(target, []byte("ok"), 0o644))
		require.NoError(t, os.Symlink(target, link))

		removePath(link)
		assert.False(t, exists(link))
		assert.True(t, exists(target))
	})
}
