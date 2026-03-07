package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"akhilsingh.in/skillctl/internal/config"
)

func TestSkillPickerWindowScrollsWithCursor(t *testing.T) {
	matches := make([]skillMatch, 0, 8)
	for i := 0; i < 8; i++ {
		matches = append(matches, skillMatch{
			Skill: config.AvailableSkill{
				ID: fmt.Sprintf("repo/skill-%d", i+1),
			},
			CatalogIndex: i + 1,
		})
	}

	m := Model{
		skillMatches: matches,
		height:       70, // maxDropdownItems = 6
	}

	for i := 0; i < 6; i++ {
		m.moveSkillPicker(1)
	}

	assert.Equal(t, 6, m.skillCursor)
	assert.Equal(t, 1, m.skillOffset)
	visible := m.visibleSkillMatches()
	assert.Len(t, visible, 6)
	assert.Equal(t, "repo/skill-2", visible[0].Skill.ID)
	assert.Equal(t, "repo/skill-7", visible[5].Skill.ID)

	m.moveSkillPicker(1)
	assert.Equal(t, 7, m.skillCursor)
	assert.Equal(t, 2, m.skillOffset)

	m.moveSkillPicker(1)
	assert.Equal(t, 0, m.skillCursor)
	assert.Equal(t, 0, m.skillOffset)
}

func TestInitSchedulesAutoGitPullWhenReposConfigured(t *testing.T) {
	m := Model{
		cfg: config.Config{
			Repositories: []config.Repository{
				{ID: "repo", URL: "https://github.com/example/repo.git"},
			},
		},
	}

	cmd := m.Init()
	require.NotNil(t, cmd)

	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	require.True(t, ok)

	foundAutoPull := false
	for _, sub := range batch {
		if sub == nil {
			continue
		}
		if _, ok := sub().(autoGitPullMsg); ok {
			foundAutoPull = true
			break
		}
	}

	assert.True(t, foundAutoPull)
}

func TestInitSkipsAutoGitPullWithoutRepos(t *testing.T) {
	m := Model{}

	cmd := m.Init()
	require.NotNil(t, cmd)

	msg := cmd()
	_, isBatch := msg.(tea.BatchMsg)
	_, isAutoPull := msg.(autoGitPullMsg)

	assert.False(t, isBatch)
	assert.False(t, isAutoPull)
}

func TestUpdateAutoGitPullStartsBackgroundPull(t *testing.T) {
	m := Model{
		cfg: config.Config{
			Repositories: []config.Repository{
				{ID: "repo", URL: "https://github.com/example/repo.git"},
			},
		},
		gitPullOutput: new(strings.Builder),
	}

	updatedModel, cmd := m.Update(autoGitPullMsg{})
	updated := updatedModel.(Model)

	assert.True(t, updated.gitPullRunning)
	assert.True(t, updated.gitPullSilent)
	assert.NotNil(t, cmd)
	assert.Empty(t, updated.gitPullOutput.String())
	assert.Empty(t, updated.outputContent)
}

func TestUpdateAutoGitPullNoopWithoutRepos(t *testing.T) {
	m := Model{
		gitPullOutput: new(strings.Builder),
	}

	updatedModel, cmd := m.Update(autoGitPullMsg{})
	updated := updatedModel.(Model)

	assert.False(t, updated.gitPullRunning)
	assert.Nil(t, cmd)
	assert.Empty(t, updated.outputContent)
}

func TestToggleSkillPickerSelectionTracksPendingState(t *testing.T) {
	m := Model{
		skillMatches: []skillMatch{
			{Skill: config.AvailableSkill{ID: "repo/alpha"}, Selected: false},
			{Skill: config.AvailableSkill{ID: "repo/beta"}, Selected: true},
		},
		skillPickerSelections: map[string]bool{},
	}

	m.skillCursor = 0
	m.toggleSkillPickerSelection()
	assert.Equal(t, map[string]bool{"repo/alpha": true}, m.skillPickerSelections)

	m.toggleSkillPickerSelection()
	assert.Empty(t, m.skillPickerSelections)

	m.skillCursor = 1
	m.toggleSkillPickerSelection()
	assert.Equal(t, map[string]bool{"repo/beta": false}, m.skillPickerSelections)

	m.toggleSkillPickerSelection()
	assert.Empty(t, m.skillPickerSelections)
}

func TestApplySkillPickerSelectionsAppliesAddAndRemove(t *testing.T) {
	paths := config.ResolvePaths(t.TempDir())
	m := NewModel(paths)
	m.cfg = config.Config{SelectedSkills: []string{"repo/alpha"}}
	m.available = []config.AvailableSkill{
		{ID: "repo/alpha", Name: "alpha", RepoID: "repo"},
		{ID: "repo/beta", Name: "beta", RepoID: "repo"},
	}
	m.availableIDs = []string{"repo/alpha", "repo/beta"}
	m.skillPickerOpen = true
	m.skillPickerSelections = map[string]bool{
		"repo/alpha": false,
		"repo/beta":  true,
	}

	m.applySkillPickerSelections()

	assert.False(t, m.skillPickerOpen)
	assert.Equal(t, []string{"repo/beta"}, m.cfg.SelectedSkills)
	require.NotEmpty(t, m.outputContent)
	output := m.outputContent
	assert.Contains(t, output, "Added:")
	assert.Contains(t, output, "repo/beta")
	assert.Contains(t, output, "Removed from selection:")
	assert.Contains(t, output, "repo/alpha")

	saved := config.LoadConfig(paths)
	assert.Equal(t, []string{"repo/beta"}, saved.SelectedSkills)
}

func TestEscapeClearsInputBeforeViewport(t *testing.T) {
	paths := config.ResolvePaths(t.TempDir())
	m := NewModel(paths)
	m.setOutput("/list", "some output")
	m.commandInput.SetValue("/hel")

	updatedModel, _ := m.handleCommandKey(tea.KeyMsg{Type: tea.KeyEsc})
	updated := updatedModel.(Model)

	assert.Equal(t, "", updated.commandInput.Value())
	assert.Equal(t, "/list", updated.outputLabel)
	assert.Equal(t, "some output", updated.outputContent)
}

func TestEscapeClearsViewportWhenInputEmpty(t *testing.T) {
	paths := config.ResolvePaths(t.TempDir())
	m := NewModel(paths)
	m.setOutput("/list", "some output")
	m.commandInput.SetValue("")

	updatedModel, _ := m.handleCommandKey(tea.KeyMsg{Type: tea.KeyEsc})
	updated := updatedModel.(Model)

	assert.Equal(t, "", updated.commandInput.Value())
	assert.Equal(t, "", updated.outputLabel)
	assert.Equal(t, "", updated.outputContent)
}
