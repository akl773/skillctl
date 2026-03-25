package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"akhilsingh.in/skillctl/internal/config"
)

func TestRenderHeader(t *testing.T) {
	paths := config.ResolvePaths(t.TempDir())
	m := NewModel(paths)
	m.cfg.Repositories = []config.Repository{{ID: "repo"}}
	m.available = []config.AvailableSkill{{ID: "repo/alpha"}}
	m.cfg.SelectedSkills = []string{"repo/alpha"}
	m.cfg.Targets = []string{"~/skills"}

	rendered := m.renderHeader(100, false, false)
	assert.Contains(t, rendered, "skillctl")
	assert.Contains(t, rendered, "idle")
	assert.Contains(t, rendered, "workspace")

	m.gitPullRunning = true
	rendered = m.renderHeader(100, false, false)
	assert.Contains(t, rendered, "syncing upstream sources")

	tiny := m.renderHeader(100, false, true)
	assert.Contains(t, tiny, "skillctl")
	assert.NotContains(t, tiny, "workspace")

	compact := m.renderHeader(100, true, false)
	assert.Contains(t, compact, "repos")
	assert.NotContains(t, compact, "workspace")
}

func TestRenderInputRow(t *testing.T) {
	ti := textinput.New()
	ti.SetValue("help")

	m := Model{commandInput: ti}
	row := m.renderInputRow(80)
	assert.Contains(t, row, "start with /")

	m.commandInput.SetValue("/help")
	row = m.renderInputRow(80)
	assert.NotContains(t, row, "start with /")

	m.commandInput.SetValue("help")
	m.skillPickerOpen = true
	row = m.renderInputRow(80)
	assert.NotContains(t, row, "start with /")
}

func TestRenderCommandDropdown(t *testing.T) {
	ti := textinput.New()
	ti.SetValue("/li")

	m := Model{
		commandInput: ti,
		matches: []commandMatch{{
			Command: commandDef{Name: "list", Description: "List selected skills", Usage: "/list"},
		}},
	}

	rendered := m.renderCommandDropdown(80)
	assert.Contains(t, rendered, "/list")
	assert.Contains(t, rendered, "usage: /list")

	m.commandInput.SetValue("list")
	assert.Equal(t, "", m.renderCommandDropdown(80))
}

func TestRenderSkillPickerDropdown(t *testing.T) {
	t.Run("shows warning when no skills", func(t *testing.T) {
		m := Model{skillPickerOpen: true}
		rendered := m.renderSkillPickerDropdown(80)
		assert.Contains(t, rendered, "No skills available yet")
	})

	t.Run("shows selected and pending entries", func(t *testing.T) {
		m := Model{
			skillPickerOpen: true,
			available: []config.AvailableSkill{{
				ID: "repo/alpha", Name: "alpha", RepoID: "repo",
			}},
			skillMatches: []skillMatch{{
				Skill:        config.AvailableSkill{ID: "repo/alpha", Name: "alpha", RepoID: "repo"},
				CatalogIndex: 1,
				Selected:     false,
			}},
			skillPickerSelections: map[string]bool{"repo/alpha": true},
		}

		rendered := m.renderSkillPickerDropdown(80)
		assert.Contains(t, rendered, "alpha")
		assert.Contains(t, rendered, "space toggle")
	})
}

func TestRenderListPickerDropdown(t *testing.T) {
	t.Run("shows no matching when empty", func(t *testing.T) {
		m := Model{listPickerOpen: true}
		rendered := m.renderListPickerDropdown(80)
		assert.Contains(t, rendered, "No matching skills")
	})

	t.Run("shows keep and pending removal entries", func(t *testing.T) {
		m := Model{
			listPickerOpen: true,
			listMatches: []skillMatch{
				{
					Skill:        config.AvailableSkill{ID: "repo/alpha", Name: "alpha", RepoID: "repo"},
					CatalogIndex: 1,
					Selected:     true,
				},
				{
					Skill:        config.AvailableSkill{ID: "repo/beta", Name: "beta", RepoID: "repo"},
					CatalogIndex: 2,
					Selected:     true,
				},
			},
			listPickerRemovals: map[string]bool{"repo/beta": true},
		}

		rendered := m.renderListPickerDropdown(80)
		assert.Contains(t, rendered, "alpha")
		assert.Contains(t, rendered, "beta")
		assert.Contains(t, rendered, "space toggle")
		assert.Contains(t, rendered, "[✓]")
		assert.Contains(t, rendered, "[-]")
	})
}

func TestRenderImportPickers(t *testing.T) {
	agent := importAgentOption{
		ID:   "claude",
		Name: "Claude",
		Skills: []importSkillCandidate{
			{Key: "alpha", Name: "alpha"},
		},
	}

	t.Run("agent picker", func(t *testing.T) {
		m := Model{
			importAgentPickerOpen: true,
			importAgentMatches:    []importAgentOption{agent},
		}
		rendered := m.renderImportAgentPickerDropdown(80)
		assert.Contains(t, rendered, "Import: Agents")
		assert.Contains(t, rendered, "Claude")
	})

	t.Run("skill picker", func(t *testing.T) {
		m := Model{
			importSkillPickerOpen: true,
			importAgentChosen:     agent,
			importSkillMatches: []importSkillMatch{{
				Skill: importSkillCandidate{Key: "alpha", Name: "alpha"},
			}},
			importSkillSelections: map[string]bool{"alpha": true},
		}
		rendered := m.renderImportSkillPickerDropdown(80)
		assert.Contains(t, rendered, "Import: Claude")
		assert.Contains(t, rendered, "alpha")
		assert.Contains(t, rendered, "space toggle")
	})
}

func TestRenderHelpBar(t *testing.T) {
	m := Model{width: 100, height: 40}
	assert.Contains(t, m.renderHelpBar(100), "type / for commands")

	m.listPickerOpen = true
	assert.Contains(t, m.renderHelpBar(100), "space toggle")

	m.listPickerOpen = false
	m.skillPickerOpen = true
	assert.Contains(t, m.renderHelpBar(100), "space toggle")

	m.skillPickerOpen = false
	m.importAgentPickerOpen = true
	assert.Contains(t, m.renderHelpBar(100), "enter continue")

	m.importAgentPickerOpen = false
	m.importSkillPickerOpen = true
	assert.Contains(t, m.renderHelpBar(100), "enter import")

	m.importSkillPickerOpen = false
	m.gitPullRunning = true
	assert.Contains(t, m.renderHelpBar(100), "upstream sync running")
}

func TestRenderChatViewportContentAndWelcomeState(t *testing.T) {
	paths := config.ResolvePaths(t.TempDir())
	m := NewModel(paths)

	empty := m.renderChatViewportContent(80)
	assert.Contains(t, empty, "Welcome to skillctl")

	m.outputLabel = "/list"
	m.outputContent = "some output"
	filled := m.renderChatViewportContent(80)
	assert.Contains(t, filled, "/list")
	assert.Contains(t, filled, "some output")
}

func TestRenderOutputSections(t *testing.T) {
	m := Model{}
	label := m.renderOutputLabel("/help\n", 8)
	assert.Contains(t, label, "/help")
	assert.NotContains(t, label, "/help\n")

	body := m.renderOutputContent("line\n", 8)
	assert.Contains(t, body, "line")
	assert.NotContains(t, body, "line\n")

	assert.Equal(t, "Goodbye.\n", renderGoodbye(40))
}

func TestViewRendersWorkspaceOrGoodbye(t *testing.T) {
	paths := config.ResolvePaths(t.TempDir())
	m := NewModel(paths)
	m.width = 100
	m.height = 40

	workspace := m.View()
	assert.Contains(t, workspace, "skillctl")

	m.quitting = true
	assert.Equal(t, "Goodbye.\n", m.View())
}

func TestViewHelpers(t *testing.T) {
	t.Run("countDirs", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(root, "a"), 0o755))
		require.NoError(t, os.Mkdir(filepath.Join(root, "b"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, "file.txt"), []byte("x"), 0o644))

		assert.Equal(t, 2, countDirs(root))
		assert.Equal(t, 0, countDirs(filepath.Join(root, "missing")))
	})

	t.Run("skill naming", func(t *testing.T) {
		skill := config.AvailableSkill{ID: "repo/alpha", Name: "", RepoID: ""}
		assert.Equal(t, "alpha", skillDisplayName(skill))
		assert.Equal(t, "repo", skillNamespace(skill))

		skill = config.AvailableSkill{ID: "repo/alpha", Name: "Alpha", RepoID: "custom"}
		assert.Equal(t, "Alpha", skillDisplayName(skill))
		assert.Equal(t, "custom", skillNamespace(skill))
	})

	t.Run("truncate and math", func(t *testing.T) {
		assert.Equal(t, "abcdef", truncateASCII("abcdef", 10))
		assert.Equal(t, "abc...", truncateASCII("abcdefghij", 6))
		assert.Equal(t, "ab", truncateASCII("abcd", 2))

		assert.Equal(t, 1, min(1, 2))
		assert.Equal(t, 2, max(1, 2))
		assert.Equal(t, "", pluralSuffix(1))
		assert.Equal(t, "s", pluralSuffix(2))
	})
}

func TestRenderChatWorkspaceShowsHighestPriorityDropdown(t *testing.T) {
	ti := textinput.New()
	ti.SetValue("/li")

	m := Model{
		width:        100,
		height:       40,
		contentWidth: 80,
		commandInput: ti,
		chatViewport: viewport.New(80, 12),
		matches: []commandMatch{{
			Command: commandDef{Name: "list", Description: "List selected skills", Usage: "/list"},
		}},
		skillPickerOpen: true,
		skillMatches: []skillMatch{{
			Skill:        config.AvailableSkill{ID: "repo/alpha", Name: "alpha", RepoID: "repo"},
			CatalogIndex: 1,
		}},
		available: []config.AvailableSkill{{ID: "repo/alpha", Name: "alpha", RepoID: "repo"}},
	}

	// renderChatWorkspace reads chatViewport.View(); keep it non-empty.
	m.outputContent = strings.Repeat("x", 4)
	m.refreshChatViewport(false)

	rendered := m.renderChatWorkspace()
	assert.Contains(t, rendered, "match(es) for \"/li\"")
	assert.NotContains(t, rendered, "usage: /list")
}
