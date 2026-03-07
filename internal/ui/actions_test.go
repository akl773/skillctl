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
