package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"akhilsingh.in/skillctl/internal/config"
)

func TestMatchAvailableSkills(t *testing.T) {
	m := Model{
		cfg: config.Config{
			SelectedSkills: []string{"repo/react-best-practices"},
		},
		available: []config.AvailableSkill{
			{ID: "repo/react-best-practices", Name: "React Best Practices", RepoID: "repo"},
			{ID: "repo/go-security", Name: "Go Security", RepoID: "repo"},
			{ID: "other/ui-review", Name: "UI Review", RepoID: "other"},
		},
	}

	t.Run("empty query returns all with catalog indexes", func(t *testing.T) {
		matches := m.matchAvailableSkills("")
		require.Len(t, matches, 3)
		assert.Equal(t, "repo/react-best-practices", matches[0].Skill.ID)
		assert.Equal(t, 1, matches[0].CatalogIndex)
		assert.True(t, matches[0].Selected)
		assert.Equal(t, 2, matches[1].CatalogIndex)
		assert.Equal(t, 3, matches[2].CatalogIndex)
	})

	t.Run("matches by id and name", func(t *testing.T) {
		matches := m.matchAvailableSkills("security")
		require.Len(t, matches, 1)
		assert.Equal(t, "repo/go-security", matches[0].Skill.ID)
		assert.False(t, matches[0].Selected)
	})

	t.Run("matches by repo id", func(t *testing.T) {
		matches := m.matchAvailableSkills("other")
		require.Len(t, matches, 1)
		assert.Equal(t, "other/ui-review", matches[0].Skill.ID)
	})

	t.Run("fuzzy matches non-contiguous query", func(t *testing.T) {
		matches := m.matchAvailableSkills("rctbp")
		require.NotEmpty(t, matches)
		assert.Equal(t, "repo/react-best-practices", matches[0].Skill.ID)
	})

	t.Run("separator-insensitive normalized prefix and equality", func(t *testing.T) {
		testModel := Model{
			available: []config.AvailableSkill{
				{ID: "design/ui-ux", Name: "UI-UX", RepoID: "design"},
				{ID: "design/ui-upgrade", Name: "UI Upgrade", RepoID: "design"},
			},
		}

		matches := testModel.matchAvailableSkills("ui ux")
		require.NotEmpty(t, matches)
		assert.Equal(t, "design/ui-ux", matches[0].Skill.ID)
	})

	t.Run("prefix ranks ahead of fuzzy fallback", func(t *testing.T) {
		testModel := Model{
			available: []config.AvailableSkill{
				{ID: "repo/react-toolkit", Name: "React Toolkit", RepoID: "repo"},
				{ID: "repo/rapid-engineering-and-coding-techniques", Name: "Rapid Engineering and Coding Techniques", RepoID: "repo"},
			},
		}

		matches := testModel.matchAvailableSkills("react")
		require.Len(t, matches, 2)
		assert.Equal(t, "repo/react-toolkit", matches[0].Skill.ID)
	})

	t.Run("rank tier dominates field weighting", func(t *testing.T) {
		testModel := Model{
			available: []config.AvailableSkill{
				{ID: "repo/a", Name: "Best UI Patterns", RepoID: "repo"},
				{ID: "ui/kit", Name: "Toolkit", RepoID: "repo"},
			},
		}

		matches := testModel.matchAvailableSkills("ui")
		require.Len(t, matches, 2)
		assert.Equal(t, "ui/kit", matches[0].Skill.ID)
		assert.Equal(t, "repo/a", matches[1].Skill.ID)
	})
}

func TestActionSearchUsesRankedMatching(t *testing.T) {
	m := Model{
		available: []config.AvailableSkill{
			{ID: "design/ui-ux", Name: "UI-UX", RepoID: "design"},
			{ID: "design/user-interface-ux", Name: "User Interface UX", RepoID: "design"},
		},
	}

	output := m.actionSearch("ui ux")
	first := strings.Index(output, "design/ui-ux")
	second := strings.Index(output, "design/user-interface-ux")

	assert.NotEqual(t, -1, first)
	assert.NotEqual(t, -1, second)
	assert.Less(t, first, second)
}

func TestActionToggleSkillSelection(t *testing.T) {
	t.Run("toggles add/remove for mixed inputs", func(t *testing.T) {
		paths := config.ResolvePaths(t.TempDir())
		m := Model{
			paths: paths,
			cfg: config.Config{
				SelectedSkills: []string{"repo/alpha"},
			},
			availableIDs: []string{"repo/alpha", "repo/beta", "repo/gamma"},
		}

		output := m.actionToggleSkillSelection("2,repo/alpha,repo/missing")

		assert.Equal(t, []string{"repo/beta"}, m.cfg.SelectedSkills)
		assert.Contains(t, output, "Added:")
		assert.Contains(t, output, "repo/beta")
		assert.Contains(t, output, "Not found:")
		assert.Contains(t, output, "repo/missing")
		assert.Contains(t, output, "Removed from selection:")
		assert.Contains(t, output, "repo/alpha")

		assert.FileExists(t, paths.ConfigPath)
		saved := config.LoadConfig(paths)
		assert.Equal(t, []string{"repo/beta"}, saved.SelectedSkills)
	})

	t.Run("reports invalid indexes while applying valid toggles", func(t *testing.T) {
		paths := config.ResolvePaths(t.TempDir())
		m := Model{
			paths: paths,
			cfg: config.Config{
				SelectedSkills: []string{"repo/alpha"},
			},
			availableIDs: []string{"repo/alpha", "repo/beta"},
		}

		output := m.actionToggleSkillSelection("4,1")

		assert.Empty(t, m.cfg.SelectedSkills)
		assert.Contains(t, output, "Invalid catalog number(s): 4")
		assert.Contains(t, output, "Removed from selection:")
		assert.Contains(t, output, "repo/alpha")
	})
}

func TestApplySkillSelectionChangesAutoSyncsAfterAdd(t *testing.T) {
	paths := config.ResolvePaths(t.TempDir())
	sourceDir := filepath.Join(t.TempDir(), "source-skill")
	require.NoError(t, os.MkdirAll(sourceDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "SKILL.md"), []byte("content"), 0o644))

	targetDir := filepath.Join(t.TempDir(), "targets")
	require.NoError(t, os.MkdirAll(targetDir, 0o755))

	m := Model{
		paths: paths,
		cfg: config.Config{
			SelectedSkills: []string{},
			Targets:        []string{targetDir},
		},
		available: []config.AvailableSkill{
			{ID: "repo/alpha", Name: "alpha", RepoID: "repo", SourcePath: sourceDir},
		},
		availableIDs: []string{"repo/alpha"},
	}

	output := m.applySkillSelectionChanges([]string{"repo/alpha"}, nil)

	assert.Contains(t, output, "Added:")
	assert.Contains(t, output, "Auto-deploy after selection")
	assert.Contains(t, output, "Syncing 1 skill(s) -> 1 target(s)")

	installedPath := filepath.Join(targetDir, config.SkillInstallDirName("repo/alpha"), "SKILL.md")
	assert.FileExists(t, installedPath)
}

func TestMatchSelectedSkills(t *testing.T) {
	m := Model{
		cfg: config.Config{
			SelectedSkills: []string{"repo/alpha", "repo/beta"},
		},
		available: []config.AvailableSkill{
			{ID: "repo/alpha", Name: "Alpha Skill", RepoID: "repo"},
			{ID: "repo/beta", Name: "Beta Skill", RepoID: "repo"},
			{ID: "repo/gamma", Name: "Gamma Skill", RepoID: "repo"},
		},
	}

	t.Run("returns only selected skills", func(t *testing.T) {
		matches := m.matchSelectedSkills("")
		require.Len(t, matches, 2)
		assert.Equal(t, "repo/alpha", matches[0].Skill.ID)
		assert.Equal(t, "repo/beta", matches[1].Skill.ID)
		assert.True(t, matches[0].Selected)
		assert.True(t, matches[1].Selected)
		assert.Equal(t, 1, matches[0].CatalogIndex)
		assert.Equal(t, 2, matches[1].CatalogIndex)
	})

	t.Run("filters by query", func(t *testing.T) {
		matches := m.matchSelectedSkills("alpha")
		require.Len(t, matches, 1)
		assert.Equal(t, "repo/alpha", matches[0].Skill.ID)
	})

	t.Run("handles missing skills", func(t *testing.T) {
		missingModel := Model{
			cfg: config.Config{
				SelectedSkills: []string{"repo/missing"},
			},
			available: []config.AvailableSkill{},
		}

		matches := missingModel.matchSelectedSkills("")
		require.Len(t, matches, 1)
		assert.Equal(t, "repo/missing", matches[0].Skill.ID)
		assert.True(t, matches[0].Selected)
	})
}

func TestActionAddRepoSkipsSecondSyncWhenPullAlreadyRunning(t *testing.T) {
	paths := config.ResolvePaths(t.TempDir())
	m := Model{
		paths:          paths,
		cfg:            config.Config{},
		gitPullRunning: true,
		gitPullOutput:  new(strings.Builder),
	}

	result := m.actionAddRepo("https://github.com/foo/bar")

	assert.Nil(t, result.Cmd)
	assert.Contains(t, result.Output, "Added repository: foo-bar")
	assert.Contains(t, result.Output, "An upstream sync is already running")
	require.Len(t, m.cfg.Repositories, 1)
	assert.Equal(t, "foo-bar", m.cfg.Repositories[0].ID)

	saved := config.LoadConfig(paths)
	found := false
	for _, repo := range saved.Repositories {
		if repo.ID == "foo-bar" {
			found = true
			break
		}
	}
	assert.True(t, found)
}
