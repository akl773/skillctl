package ui

import (
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
