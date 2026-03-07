package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"akhilsingh.in/skillctl/internal/config"
	"akhilsingh.in/skillctl/internal/core"
)

// menuItem represents a single item in the main menu.
type menuItem struct {
	title string
	icon  string
	desc  string
}

// viewState tracks which screen is active.
type viewState int

const (
	viewMenu viewState = iota
	viewResult
	viewInput
	viewSearchResults
	viewTargets
	viewTargetInput
)

// inputAction specifies what the text input is being used for.
type inputAction int

const (
	inputAddSkill inputAction = iota
	inputSearch
	inputRemoveSkill
	inputAddTarget
	inputRemoveTarget
)

// Model is the root bubbletea model.
type Model struct {
	paths     config.AppPaths
	cfg       config.Config
	available []string
	width     int
	height    int

	// menu state
	cursor   int
	items    []menuItem
	view     viewState
	result   string
	quitting bool

	// input state
	textInput   textinput.Model
	inputAction inputAction

	// search state
	searchMatches []string
}

// NewModel creates a new UI model.
func NewModel(paths config.AppPaths) Model {
	ti := textinput.New()
	ti.CharLimit = 256
	ti.Width = 60

	items := []menuItem{
		{title: "Git Pull", icon: "⬇", desc: "Pull latest from source repo"},
		{title: "List Selected", icon: "📋", desc: "View your selected skills"},
		{title: "Search Skills", icon: "🔍", desc: "Search the skill catalog"},
		{title: "Add Skill", icon: "➕", desc: "Add skills to your selection"},
		{title: "Remove Skill", icon: "➖", desc: "Remove skills from selection + targets"},
		{title: "Sync Skills", icon: "🔄", desc: "Rsync selected skills to all targets"},
		{title: "Manage Targets", icon: "📁", desc: "Add or remove target folders"},
		{title: "Status", icon: "ℹ️", desc: "View full status overview"},
		{title: "Exit", icon: "👋", desc: "Quit skillctl"},
	}

	return Model{
		paths:     paths,
		cfg:       config.LoadConfig(paths),
		available: config.LoadAvailableSkills(paths),
		items:     items,
		textInput: ti,
	}
}

// Init is the bubbletea init function.
func (m Model) Init() tea.Cmd {
	return tea.WindowSize()
}

// Update handles all messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// pass messages to text input when active
	if m.view == viewInput || m.view == viewSearchResults || m.view == viewTargetInput {
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Global quit
	if key == "ctrl+c" {
		m.quitting = true
		return m, tea.Quit
	}

	switch m.view {
	case viewMenu:
		return m.handleMenuKey(key)
	case viewResult:
		return m.handleResultKey(key)
	case viewInput:
		return m.handleInputKey(msg)
	case viewSearchResults:
		return m.handleSearchResultsKey(msg)
	case viewTargets:
		return m.handleTargetsKey(key)
	case viewTargetInput:
		return m.handleTargetInputKey(msg)
	}

	return m, nil
}

func (m Model) handleMenuKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
	case "home":
		m.cursor = 0
	case "end":
		m.cursor = len(m.items) - 1
	case "enter":
		return m.executeMenuItem()
	case "q":
		m.quitting = true
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) handleResultKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "enter", "escape", "backspace", "q":
		m.view = viewMenu
		m.result = ""
	}
	return m, nil
}

func (m Model) handleInputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "enter":
		return m.submitInput()
	case "escape":
		m.view = viewMenu
		return m, nil
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m Model) handleSearchResultsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "enter":
		return m.submitSearchAdd()
	case "escape":
		m.view = viewMenu
		return m, nil
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m Model) handleTargetsKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "a":
		m.view = viewTargetInput
		m.inputAction = inputAddTarget
		m.textInput.SetValue("")
		m.textInput.Focus()
		m.textInput.Placeholder = "~/path/to/target"
		return m, m.textInput.Focus()
	case "r":
		m.view = viewTargetInput
		m.inputAction = inputRemoveTarget
		m.textInput.SetValue("")
		m.textInput.Focus()
		m.textInput.Placeholder = "target number"
		return m, m.textInput.Focus()
	case "escape", "backspace", "q":
		m.view = viewMenu
	}
	return m, nil
}

func (m Model) handleTargetInputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "enter":
		return m.submitTargetInput()
	case "escape":
		m.view = viewTargets
		return m, nil
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m Model) executeMenuItem() (tea.Model, tea.Cmd) {
	switch m.cursor {
	case 0: // Git Pull
		m.result = m.actionGitPull()
		m.view = viewResult
	case 1: // List Selected
		m.result = m.actionListSelected()
		m.view = viewResult
	case 2: // Search
		m.view = viewInput
		m.inputAction = inputSearch
		m.textInput.SetValue("")
		m.textInput.Placeholder = "search terms..."
		m.textInput.Focus()
		return m, m.textInput.Focus()
	case 3: // Add
		m.view = viewInput
		m.inputAction = inputAddSkill
		m.textInput.SetValue("")
		m.textInput.Placeholder = "skill names (comma-separated)"
		m.textInput.Focus()
		return m, m.textInput.Focus()
	case 4: // Remove
		m.view = viewInput
		m.inputAction = inputRemoveSkill
		m.textInput.SetValue("")
		m.textInput.Placeholder = "skill name or number"
		m.textInput.Focus()
		return m, m.textInput.Focus()
	case 5: // Sync
		m.result = m.actionSync()
		m.view = viewResult
	case 6: // Manage Targets
		m.view = viewTargets
	case 7: // Status
		m.result = m.actionStatus()
		m.view = viewResult
	case 8: // Exit
		m.quitting = true
		return m, tea.Quit
	}

	return m, nil
}

func (m Model) submitInput() (tea.Model, tea.Cmd) {
	value := m.textInput.Value()
	if value == "" {
		m.view = viewMenu
		return m, nil
	}

	switch m.inputAction {
	case inputAddSkill:
		m.result = m.actionAddSkill(value)
		m.view = viewResult
	case inputSearch:
		return m.actionSearch(value)
	case inputRemoveSkill:
		m.result = m.actionRemoveSkill(value)
		m.view = viewResult
	}

	return m, nil
}

func (m Model) submitSearchAdd() (tea.Model, tea.Cmd) {
	value := m.textInput.Value()
	if value == "" {
		m.view = viewMenu
		return m, nil
	}

	tokens := config.InputCSV(value)
	requested, invalid := config.SplitByReference(tokens, m.searchMatches)

	var sb strings.Builder
	if len(invalid) > 0 {
		sb.WriteString(fmt.Sprintf("Invalid number(s): %s\n\n", strings.Join(invalid, ", ")))
	}

	outcome := core.AddRequestedSkills(&m.cfg, requested, m.available)
	if len(outcome.Added) > 0 {
		_ = config.SaveConfig(m.paths, m.cfg)
	}
	sb.WriteString(formatAddOutcome(outcome))

	m.result = sb.String()
	m.view = viewResult
	return m, nil
}

func (m Model) submitTargetInput() (tea.Model, tea.Cmd) {
	value := m.textInput.Value()
	if value == "" {
		m.view = viewTargets
		return m, nil
	}

	switch m.inputAction {
	case inputAddTarget:
		normalized := config.CompactPath(config.ExpandPath(value))
		for _, t := range m.cfg.Targets {
			if t == normalized {
				m.result = "Target already exists: " + normalized
				m.view = viewResult
				return m, nil
			}
		}
		m.cfg.Targets = append(m.cfg.Targets, normalized)
		_ = config.SaveConfig(m.paths, m.cfg)
		m.result = "Added target: " + normalized
		m.view = viewResult

	case inputRemoveTarget:
		tokens, _ := config.SplitByReference([]string{value}, m.cfg.Targets)
		if len(tokens) == 0 {
			m.result = "Invalid target number"
			m.view = viewResult
			return m, nil
		}
		removed := tokens[0]
		var kept []string
		for _, t := range m.cfg.Targets {
			if t != removed {
				kept = append(kept, t)
			}
		}
		m.cfg.Targets = kept
		_ = config.SaveConfig(m.paths, m.cfg)
		m.result = "Removed target: " + removed
		m.view = viewResult
	}

	return m, nil
}

// View renders the current state.
func (m Model) View() string {
	if m.quitting {
		return renderGoodbye(m.width)
	}

	switch m.view {
	case viewMenu:
		return m.renderMainMenu()
	case viewResult:
		return m.renderResultView()
	case viewInput:
		return m.renderInputView()
	case viewSearchResults:
		return m.renderSearchResultsView()
	case viewTargets:
		return m.renderTargetsView()
	case viewTargetInput:
		return m.renderTargetInputView()
	}

	return ""
}

// refresh reloads config and available skills.
func (m *Model) refresh() {
	m.cfg = config.LoadConfig(m.paths)
	m.available = config.LoadAvailableSkills(m.paths)
}
