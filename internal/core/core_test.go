package core

import (
	"os"
	"path/filepath"
	"testing"

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
		alphaPath := filepath.Join(target, "Alpha")
		betaPath := filepath.Join(target, "beta")

		require.NoError(t, os.MkdirAll(alphaPath, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(alphaPath, "README.md"), []byte("alpha"), 0o644))
		require.NoError(t, os.WriteFile(betaPath, []byte("beta"), 0o644))

		cfg := config.Config{
			SelectedSkills: []string{"Alpha", "beta"},
			DisabledSkills: []string{"Alpha", "beta"},
			Targets:        []string{target},
		}

		outcome := RemoveSelectedSkills(&cfg, []string{"alpha"})

		assert.Equal(t, []string{"Alpha"}, outcome.RemovedFromSelected)
		assert.Empty(t, outcome.NotSelected)
		assert.Equal(t, []string{"beta"}, cfg.SelectedSkills)
		assert.Equal(t, []string{"beta"}, cfg.DisabledSkills)
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
		require.NoError(t, os.MkdirAll(filepath.Join(target, "beta"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(target, "gamma"), 0o755))

		cfg := config.Config{
			SelectedSkills: []string{"gamma", "beta", "alpha"},
			DisabledSkills: []string{"gamma", "alpha"},
			Targets:        []string{target},
		}

		outcome := RemoveSelectedSkills(&cfg, []string{"BETA", "gamma"})

		assert.Equal(t, []string{"beta", "gamma"}, outcome.RemovedFromSelected)
		assert.Equal(t, []string{"alpha"}, cfg.SelectedSkills)
		assert.Equal(t, []string{"alpha"}, cfg.DisabledSkills)
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
	assert.True(t, GitPullOutcome{ReturnCode: 0}.Success())
	assert.False(t, GitPullOutcome{ReturnCode: 1}.Success())
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
