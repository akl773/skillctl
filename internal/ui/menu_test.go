package ui

import (
	"fmt"
	"os"
	"path/filepath"
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

func TestRepoURLPromptSubmitAddsRepository(t *testing.T) {
	paths := config.ResolvePaths(t.TempDir())
	m := NewModel(paths)
	initialRepoCount := len(m.cfg.Repositories)

	m.enterRepoURLPrompt()
	m.commandInput.SetValue("https://github.com/foo/bar")

	updatedModel, cmd := m.handleRepoURLPromptKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := updatedModel.(Model)

	require.NotNil(t, cmd)
	assert.False(t, updated.awaitingRepoURL)
	assert.Equal(t, defaultInputPlaceholder, updated.commandInput.Placeholder)
	assert.Equal(t, "", updated.commandInput.Value())
	require.Len(t, updated.cfg.Repositories, initialRepoCount+1)
	assert.Equal(t, "foo-bar", updated.cfg.Repositories[len(updated.cfg.Repositories)-1].ID)
	assert.Contains(t, updated.outputContent, "Added repository")
	assert.Contains(t, updated.outputContent, "Syncing upstream skill source")
}

func TestRepoURLPromptSubmitInvalidURLKeepsPromptOpen(t *testing.T) {
	paths := config.ResolvePaths(t.TempDir())
	m := NewModel(paths)
	initialRepoCount := len(m.cfg.Repositories)

	m.enterRepoURLPrompt()
	m.commandInput.SetValue("not-a-url")

	updatedModel, _ := m.handleRepoURLPromptKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := updatedModel.(Model)

	assert.True(t, updated.awaitingRepoURL)
	assert.Equal(t, repoURLPromptPlaceholder, updated.commandInput.Placeholder)
	assert.Contains(t, updated.outputContent, "Invalid repository URL")
	assert.Len(t, updated.cfg.Repositories, initialRepoCount)
}

func TestRepoURLPromptEscapeCancelsPrompt(t *testing.T) {
	paths := config.ResolvePaths(t.TempDir())
	m := NewModel(paths)
	initialRepoCount := len(m.cfg.Repositories)

	m.enterRepoURLPrompt()
	m.commandInput.SetValue("https://github.com/foo/bar")

	updatedModel, _ := m.handleRepoURLPromptKey(tea.KeyMsg{Type: tea.KeyEsc})
	updated := updatedModel.(Model)

	assert.False(t, updated.awaitingRepoURL)
	assert.Equal(t, defaultInputPlaceholder, updated.commandInput.Placeholder)
	assert.Equal(t, "", updated.commandInput.Value())
	assert.Len(t, updated.cfg.Repositories, initialRepoCount)
}

func TestImportAgentPickerEnterOpensSkillPicker(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	claudeSkillDir := filepath.Join(home, ".claude", "skills", "my-import")
	require.NoError(t, os.MkdirAll(claudeSkillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(claudeSkillDir, "SKILL.md"), []byte("# skill"), 0o644))

	paths := config.ResolvePaths(t.TempDir())
	m := NewModel(paths)
	m.enterImportAgentPicker()
	require.True(t, m.importAgentPickerOpen)

	updatedModel, _ := m.handleImportAgentPickerKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := updatedModel.(Model)

	assert.False(t, updated.importAgentPickerOpen)
	assert.True(t, updated.importSkillPickerOpen)
	require.NotEmpty(t, updated.importSkillMatches)
}

func TestImportSkillPickerApplyImportsIntoManagedSource(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	claudeSkillDir := filepath.Join(home, ".claude", "skills", "my-import")
	require.NoError(t, os.MkdirAll(claudeSkillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(claudeSkillDir, "SKILL.md"), []byte("# skill"), 0o644))

	paths := config.ResolvePaths(t.TempDir())
	m := NewModel(paths)
	m.cfg.Targets = []string{filepath.Join(t.TempDir(), "targets")}
	require.NoError(t, os.MkdirAll(config.ExpandPath(m.cfg.Targets[0]), 0o755))
	_ = config.SaveConfig(paths, m.cfg)

	m.enterImportAgentPicker()
	require.True(t, m.importAgentPickerOpen)
	m.enterImportSkillPicker()
	require.True(t, m.importSkillPickerOpen)
	require.NotEmpty(t, m.importSkillMatches)

	m.toggleImportSkillSelection()
	m.applyImportSkillSelections()

	assert.False(t, m.importSkillPickerOpen)
	assert.Contains(t, m.outputContent, "Added:")
	assert.Contains(t, m.outputContent, managedImportSourceID+"/")

	managedPath := filepath.Join(paths.LocalDir, "imported-skills")
	assert.DirExists(t, managedPath)
	entries, err := os.ReadDir(managedPath)
	require.NoError(t, err)
	require.NotEmpty(t, entries)
	assert.FileExists(t, filepath.Join(managedPath, entries[0].Name(), "SKILL.md"))
}

func TestDiscoverImportAgentsExcludesSkillctlManagedSkills(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, ".claude", "skills")

	managedTop := filepath.Join(root, "sickn33-antigravity-awesome-skills--code-reviewer")
	require.NoError(t, os.MkdirAll(managedTop, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(managedTop, "SKILL.md"), []byte("# managed"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(managedTop, "nested"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(managedTop, "nested", "SKILL.md"), []byte("# nested"), 0o644))

	codeReviewer := filepath.Join(root, "code-reviewer")
	require.NoError(t, os.MkdirAll(codeReviewer, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(codeReviewer, "SKILL.md"), []byte("# legacy"), 0o644))

	customUnmanaged := filepath.Join(root, "custom-unmanaged")
	require.NoError(t, os.MkdirAll(customUnmanaged, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(customUnmanaged, "SKILL.md"), []byte("# unmanaged"), 0o644))

	paths := config.ResolvePaths(t.TempDir())

	managedTopHash, err := config.ComputeDirectoryHash(managedTop)
	require.NoError(t, err)
	require.NoError(t, config.AddSkillHash(paths, managedTop, managedTopHash, "sickn33-antigravity-awesome-skills/code-reviewer"))

	nestedPath := filepath.Join(managedTop, "nested")
	nestedHash, err := config.ComputeDirectoryHash(nestedPath)
	require.NoError(t, err)
	require.NoError(t, config.AddSkillHash(paths, nestedPath, nestedHash, "sickn33-antigravity-awesome-skills/code-reviewer/nested"))

	m := NewModel(paths)

	agents := m.discoverImportAgents()
	require.Len(t, agents, 1)
	assert.Equal(t, "claude", agents[0].ID)

	relatives := make([]string, 0, len(agents[0].Skills))
	for _, skill := range agents[0].Skills {
		relatives = append(relatives, skill.Relative)
	}

	assert.Contains(t, relatives, "custom-unmanaged")
	assert.NotContains(t, relatives, "sickn33-antigravity-awesome-skills--code-reviewer")
	assert.NotContains(t, relatives, "sickn33-antigravity-awesome-skills--code-reviewer/nested")
}

func TestDiscoverImportAgentsFindsCursorSkillsInDefaultRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cursorSkillDir := filepath.Join(home, ".cursor", "skills", "custom-cursor")
	require.NoError(t, os.MkdirAll(cursorSkillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cursorSkillDir, "SKILL.md"), []byte("# cursor"), 0o644))

	paths := config.ResolvePaths(t.TempDir())
	m := NewModel(paths)

	agents := m.discoverImportAgents()
	require.NotEmpty(t, agents)

	var cursor *importAgentOption
	for i := range agents {
		if agents[i].ID == "cursor" {
			cursor = &agents[i]
			break
		}
	}
	require.NotNil(t, cursor)
	require.NotEmpty(t, cursor.Skills)

	relatives := make([]string, 0, len(cursor.Skills))
	for _, skill := range cursor.Skills {
		relatives = append(relatives, skill.Relative)
	}
	assert.Contains(t, relatives, "custom-cursor")
}

func TestDiscoverImportAgentsWithHashBasedFiltering(t *testing.T) {
	t.Run("new skill shows up for import", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		root := filepath.Join(home, ".claude", "skills")

		newSkill := filepath.Join(root, "brand-new-skill")
		require.NoError(t, os.MkdirAll(newSkill, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(newSkill, "SKILL.md"), []byte("# new skill"), 0o644))

		paths := config.ResolvePaths(t.TempDir())
		m := NewModel(paths)

		agents := m.discoverImportAgents()
		require.NotEmpty(t, agents)
		assert.Equal(t, "claude", agents[0].ID)
		assert.Contains(t, agents[0].Skills[0].Relative, "brand-new-skill")
	})

	t.Run("unchanged imported skill not shown as duplicate", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		root := filepath.Join(home, ".claude", "skills")

		localSkill := filepath.Join(root, "my-skill")
		require.NoError(t, os.MkdirAll(localSkill, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(localSkill, "SKILL.md"), []byte("# my skill"), 0o644))

		paths := config.ResolvePaths(t.TempDir())

		hash, err := config.ComputeDirectoryHash(localSkill)
		require.NoError(t, err)
		require.NoError(t, config.AddSkillHash(paths, localSkill, hash, "skillctl-imported/my-skill"))

		m := NewModel(paths)
		agents := m.discoverImportAgents()

		for _, agent := range agents {
			for _, skill := range agent.Skills {
				assert.NotEqual(t, "my-skill", skill.Relative, "Unchanged imported skill should not appear")
			}
		}
	})

	t.Run("modified imported skill shows up for re-import", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		root := filepath.Join(home, ".claude", "skills")

		localSkill := filepath.Join(root, "updated-skill")
		require.NoError(t, os.MkdirAll(localSkill, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(localSkill, "SKILL.md"), []byte("# original"), 0o644))

		paths := config.ResolvePaths(t.TempDir())

		originalHash, err := config.ComputeDirectoryHash(localSkill)
		require.NoError(t, err)
		require.NoError(t, config.AddSkillHash(paths, localSkill, originalHash, "skillctl-imported/updated-skill"))

		require.NoError(t, os.WriteFile(filepath.Join(localSkill, "SKILL.md"), []byte("# modified content"), 0o644))

		m := NewModel(paths)
		agents := m.discoverImportAgents()

		found := false
		for _, agent := range agents {
			for _, skill := range agent.Skills {
				if skill.Relative == "updated-skill" {
					found = true
					break
				}
			}
		}
		assert.True(t, found, "Modified skill should appear for re-import")
	})

	t.Run("skill with same name but different content shows up", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		root := filepath.Join(home, ".claude", "skills")

		repoSkill := filepath.Join(root, "sickn33-antigravity-awesome-skills--frontend-design")
		require.NoError(t, os.MkdirAll(repoSkill, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(repoSkill, "SKILL.md"), []byte("# repo version"), 0o644))

		localSkill := filepath.Join(root, "frontend-design")
		require.NoError(t, os.MkdirAll(localSkill, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(localSkill, "SKILL.md"), []byte("# local custom version"), 0o644))

		paths := config.ResolvePaths(t.TempDir())
		m := NewModel(paths)
		m.cfg.SelectedSkills = []string{"sickn33-antigravity-awesome-skills/frontend-design"}

		agents := m.discoverImportAgents()
		require.NotEmpty(t, agents)

		foundLocal := false
		for _, agent := range agents {
			for _, skill := range agent.Skills {
				if skill.Relative == "frontend-design" {
					foundLocal = true
				}
			}
		}
		assert.True(t, foundLocal, "Local skill with same name as repo skill should appear")
	})

	t.Run("handles non-existent skill source gracefully", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		paths := config.ResolvePaths(t.TempDir())

		hashStore := config.LoadSkillHashStore(paths)
		hashStore["/nonexistent/path"] = config.SkillHashRecord{
			Hash:       "abc123",
			ImportedAs: "skillctl-imported/test",
		}
		require.NoError(t, config.SaveSkillHashStore(paths, hashStore))

		m := NewModel(paths)
		agents := m.discoverImportAgents()

		assert.Empty(t, agents, "Should handle gracefully with no agent directories")
	})
}

func TestImportStoresHashForImportedSkills(t *testing.T) {
	t.Run("import stores hash record", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		claudeSkillDir := filepath.Join(home, ".claude", "skills", "hash-test-skill")
		require.NoError(t, os.MkdirAll(claudeSkillDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(claudeSkillDir, "SKILL.md"), []byte("# skill for hashing"), 0o644))

		paths := config.ResolvePaths(t.TempDir())
		m := NewModel(paths)
		m.cfg.Targets = []string{filepath.Join(t.TempDir(), "targets")}
		require.NoError(t, os.MkdirAll(config.ExpandPath(m.cfg.Targets[0]), 0o755))
		_ = config.SaveConfig(paths, m.cfg)

		m.enterImportAgentPicker()
		m.enterImportSkillPicker()
		m.toggleImportSkillSelection()
		m.applyImportSkillSelections()

		hashStore := config.LoadSkillHashStore(paths)
		record, exists := hashStore[claudeSkillDir]
		assert.True(t, exists, "Hash should be stored after import")
		assert.NotEmpty(t, record.Hash, "Hash should not be empty")
		assert.Equal(t, "skillctl-imported/hash-test-skill", record.ImportedAs)
	})
}

func TestListPickerWindowScrollsWithCursor(t *testing.T) {
	matches := make([]skillMatch, 0, 8)
	for i := 0; i < 8; i++ {
		matches = append(matches, skillMatch{
			Skill: config.AvailableSkill{
				ID: fmt.Sprintf("repo/skill-%d", i+1),
			},
			CatalogIndex: i + 1,
			Selected:     true,
		})
	}

	m := Model{
		listMatches: matches,
		height:      70,
	}

	for i := 0; i < 6; i++ {
		m.moveListPicker(1)
	}

	assert.Equal(t, 6, m.listCursor)
	assert.Equal(t, 1, m.listOffset)
	visible := m.visibleListMatches()
	assert.Len(t, visible, 6)
	assert.Equal(t, "repo/skill-2", visible[0].Skill.ID)
	assert.Equal(t, "repo/skill-7", visible[5].Skill.ID)

	m.moveListPicker(1)
	assert.Equal(t, 7, m.listCursor)
	assert.Equal(t, 2, m.listOffset)

	m.moveListPicker(1)
	assert.Equal(t, 0, m.listCursor)
	assert.Equal(t, 0, m.listOffset)
}

func TestToggleListPickerRemovalTracksPendingState(t *testing.T) {
	m := Model{
		listMatches: []skillMatch{
			{Skill: config.AvailableSkill{ID: "repo/alpha"}, Selected: true},
			{Skill: config.AvailableSkill{ID: "repo/beta"}, Selected: true},
		},
		listPickerRemovals: map[string]bool{},
	}

	m.listCursor = 0
	m.toggleListPickerRemoval()
	assert.Equal(t, map[string]bool{"repo/alpha": true}, m.listPickerRemovals)

	m.toggleListPickerRemoval()
	assert.Empty(t, m.listPickerRemovals)
}

func TestApplyListPickerRemovalsRemovesSkills(t *testing.T) {
	paths := config.ResolvePaths(t.TempDir())
	m := NewModel(paths)
	m.cfg = config.Config{SelectedSkills: []string{"repo/alpha", "repo/beta"}}
	m.available = []config.AvailableSkill{
		{ID: "repo/alpha", Name: "alpha", RepoID: "repo"},
		{ID: "repo/beta", Name: "beta", RepoID: "repo"},
	}
	m.availableIDs = []string{"repo/alpha", "repo/beta"}
	m.listPickerOpen = true
	m.listPickerRemovals = map[string]bool{
		"repo/alpha": true,
	}

	m.applyListPickerRemovals()

	assert.False(t, m.listPickerOpen)
	assert.Equal(t, []string{"repo/beta"}, m.cfg.SelectedSkills)
	require.NotEmpty(t, m.outputContent)
	assert.Contains(t, m.outputContent, "Removed from selection:")
	assert.Contains(t, m.outputContent, "repo/alpha")
}

func TestEnterListPickerWithNoSelectedSkillsShowsWarning(t *testing.T) {
	paths := config.ResolvePaths(t.TempDir())
	m := NewModel(paths)
	m.cfg = config.Config{SelectedSkills: nil}

	m.enterListPicker()

	assert.False(t, m.listPickerOpen)
	assert.Contains(t, m.outputContent, "No skills selected yet")
}
