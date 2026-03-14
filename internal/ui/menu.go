package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"akhilsingh.in/skillctl/internal/config"
)

const maxPaletteItems = 24

const (
	defaultContentWidth      = 90
	minContentWidth          = 10
	defaultInputPlaceholder  = "Type / for commands..."
	skillPickerPlaceholder   = "Type to search skills..."
	importAgentPlaceholder   = "Type to search agents..."
	importSkillPlaceholder   = "Type to search skills..."
	repoURLPromptPlaceholder = "Type repository URL and press Enter..."
	managedImportSourceID    = "skillctl-imported"
)

type skillMatch struct {
	Skill        config.AvailableSkill
	CatalogIndex int
	Selected     bool
}

type importAgentOption struct {
	ID     string
	Name   string
	Path   string
	Skills []importSkillCandidate
}

type importSkillCandidate struct {
	Key      string
	Name     string
	Source   string
	Relative string
}

type importSkillMatch struct {
	Skill importSkillCandidate
}

// Model is the root bubbletea model.
type Model struct {
	paths        config.AppPaths
	cfg          config.Config
	available    []config.AvailableSkill
	availableIDs []string
	width        int
	height       int

	contentWidth int
	quitting     bool

	chatViewport  viewport.Model
	outputLabel   string
	outputContent string

	gitPullRunning bool
	gitPullSilent  bool
	gitPullEvents  <-chan tea.Msg
	gitPullOutput  *strings.Builder

	commandInput  textinput.Model
	commands      []commandDef
	matches       []commandMatch
	paletteCursor int

	skillPickerOpen       bool
	skillMatches          []skillMatch
	skillCursor           int
	skillOffset           int
	skillPickerSelections map[string]bool
	awaitingRepoURL       bool
	importAgentPickerOpen bool
	importAgentOptions    []importAgentOption
	importAgentMatches    []importAgentOption
	importAgentCursor     int
	importAgentOffset     int
	importSkillPickerOpen bool
	importAgentChosen     importAgentOption
	importSkillMatches    []importSkillMatch
	importSkillCursor     int
	importSkillOffset     int
	importSkillSelections map[string]bool

	history      []string
	historyIndex int
}

// NewModel creates a new UI model.
func NewModel(paths config.AppPaths) Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 512
	ti.Width = 72
	ti.Placeholder = defaultInputPlaceholder
	ti.SetValue("")
	ti.CursorEnd()
	_ = ti.Focus()

	vp := viewport.New(defaultContentWidth, 12)

	cfg := config.LoadConfig(paths)
	available := config.LoadAvailableSkills(paths, cfg)
	availableIDs := config.SkillIDs(available)

	m := Model{
		paths:         paths,
		cfg:           cfg,
		available:     available,
		availableIDs:  availableIDs,
		contentWidth:  defaultContentWidth,
		chatViewport:  vp,
		gitPullOutput: new(strings.Builder),
		commandInput:  ti,
		commands:      builtInCommands(),
		historyIndex:  0,
	}
	m.recomputeMatches()
	m.refreshChatViewport(false)
	return m
}

func (m Model) Init() tea.Cmd {
	if len(m.cfg.Repositories) == 0 {
		return tea.WindowSize()
	}

	return tea.Batch(
		tea.WindowSize(),
		startAutoGitPullCmd(),
	)
}

// Update handles all messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.applyLayout(false)
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case autoGitPullMsg:
		if len(m.cfg.Repositories) == 0 {
			return m, nil
		}

		if m.gitPullRunning {
			return m, nil
		}

		m.gitPullRunning = true
		m.gitPullSilent = true
		m.gitPullEvents = nil
		m.gitPullOutput.Reset()
		return m, startGitPullStreamCmd(m.paths, m.cfg.Repositories)

	case gitPullStreamStartedMsg:
		m.gitPullEvents = msg.events
		if m.gitPullEvents == nil {
			m.gitPullRunning = false
			return m, nil
		}
		return m, waitForGitPullEventCmd(m.gitPullEvents)

	case gitPullChunkMsg:
		if m.gitPullSilent {
			if m.gitPullEvents != nil {
				return m, waitForGitPullEventCmd(m.gitPullEvents)
			}
			return m, nil
		}

		if msg.isStderr {
			m.gitPullOutput.WriteString(mutedStyle.Render(msg.chunk))
		} else {
			m.gitPullOutput.WriteString(msg.chunk)
		}
		m.updateOutputContent(m.gitPullOutput.String())
		if m.gitPullEvents != nil {
			return m, waitForGitPullEventCmd(m.gitPullEvents)
		}
		return m, nil

	case gitPullDoneMsg:
		silent := m.gitPullSilent
		if msg.outcome.Success() {
			if !silent {
				m.gitPullOutput.WriteString("\n" + successStyle.Render("OK: repositories are up to date."))
				m.updateOutputContent(m.gitPullOutput.String())
			}
			m.refresh()
		} else if silent {
			m.setOutput("", errorStyle.Render("Background upstream sync failed. Run /pull to inspect details and retry."))
		} else {
			m.gitPullOutput.WriteString("\n" + errorStyle.Render("ERROR: one or more upstream repository updates failed. Resolve git issues before syncing."))
			m.updateOutputContent(m.gitPullOutput.String())
		}
		m.gitPullRunning = false
		m.gitPullSilent = false
		m.gitPullEvents = nil
		return m, nil
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		m.quitting = true
		return m, tea.Quit
	}

	return m.handleCommandKey(msg)
}

func (m Model) handleCommandKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if m.skillPickerOpen {
		return m.handleSkillPickerKey(msg)
	}

	if m.importAgentPickerOpen {
		return m.handleImportAgentPickerKey(msg)
	}

	if m.importSkillPickerOpen {
		return m.handleImportSkillPickerKey(msg)
	}

	if m.awaitingRepoURL {
		return m.handleRepoURLPromptKey(msg)
	}

	switch key {
	case "enter":
		return m.submitCommand()
	case "tab":
		if m.paletteOpen() {
			m.autocompleteSelected()
			m.applyLayout(false)
		}
		return m, nil
	case "up":
		if m.paletteOpen() {
			m.movePalette(-1)
			return m, nil
		}
		m.historyPrev()
		m.applyLayout(false)
		return m, nil
	case "down":
		if m.paletteOpen() {
			m.movePalette(1)
			return m, nil
		}
		m.historyNext()
		m.applyLayout(false)
		return m, nil
	case "ctrl+p":
		m.historyPrev()
		m.applyLayout(false)
		return m, nil
	case "ctrl+n":
		m.historyNext()
		m.applyLayout(false)
		return m, nil
	case "pgup", "pgdown", "home", "end":
		var cmd tea.Cmd
		m.chatViewport, cmd = m.chatViewport.Update(msg)
		return m, cmd
	case "esc":
		if m.commandInput.Value() != "" {
			m.commandInput.SetValue("")
			m.commandInput.CursorEnd()
			m.historyIndex = len(m.history)
			m.recomputeMatches()
			m.applyLayout(false)
		} else {
			m.clearOutput()
			m.applyLayout(false)
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.commandInput, cmd = m.commandInput.Update(msg)
	m.historyIndex = len(m.history)
	m.recomputeMatches()
	m.applyLayout(false)
	return m, cmd
}

func (m Model) handleSkillPickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "enter":
		m.applySkillPickerSelections()
		m.historyIndex = len(m.history)
		m.applyLayout(true)
		return m, nil
	case " ", "space":
		m.toggleSkillPickerSelection()
		m.applyLayout(false)
		return m, nil
	case "up":
		m.moveSkillPicker(-1)
		m.applyLayout(false)
		return m, nil
	case "down":
		m.moveSkillPicker(1)
		m.applyLayout(false)
		return m, nil
	case "tab":
		m.moveSkillPicker(1)
		m.applyLayout(false)
		return m, nil
	case "pgup", "pgdown", "home", "end":
		var cmd tea.Cmd
		m.chatViewport, cmd = m.chatViewport.Update(msg)
		return m, cmd
	case "esc":
		m.exitSkillPicker(true)
		m.historyIndex = len(m.history)
		m.applyLayout(false)
		return m, nil
	}

	var cmd tea.Cmd
	prev := m.commandInput.Value()
	m.commandInput, cmd = m.commandInput.Update(msg)
	if m.commandInput.Value() != prev {
		m.skillCursor = 0
		m.skillOffset = 0
	}
	m.recomputeSkillMatches()
	m.applyLayout(false)
	return m, cmd
}

func (m Model) handleRepoURLPromptKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "enter":
		return m.submitRepoURL()
	case "esc":
		m.exitRepoURLPrompt(true)
		m.historyIndex = len(m.history)
		m.applyLayout(false)
		return m, nil
	case "up":
		m.historyPrev()
		m.applyLayout(false)
		return m, nil
	case "down":
		m.historyNext()
		m.applyLayout(false)
		return m, nil
	case "ctrl+p":
		m.historyPrev()
		m.applyLayout(false)
		return m, nil
	case "ctrl+n":
		m.historyNext()
		m.applyLayout(false)
		return m, nil
	}

	var cmd tea.Cmd
	m.commandInput, cmd = m.commandInput.Update(msg)
	m.historyIndex = len(m.history)
	m.applyLayout(false)
	return m, cmd
}

func (m Model) handleImportAgentPickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "enter":
		m.enterImportSkillPicker()
		m.historyIndex = len(m.history)
		m.applyLayout(false)
		return m, nil
	case "up":
		m.moveImportAgentPicker(-1)
		m.applyLayout(false)
		return m, nil
	case "down", "tab":
		m.moveImportAgentPicker(1)
		m.applyLayout(false)
		return m, nil
	case "pgup", "pgdown", "home", "end":
		var cmd tea.Cmd
		m.chatViewport, cmd = m.chatViewport.Update(msg)
		return m, cmd
	case "esc":
		m.exitImportAgentPicker(true)
		m.historyIndex = len(m.history)
		m.applyLayout(false)
		return m, nil
	}

	var cmd tea.Cmd
	prev := m.commandInput.Value()
	m.commandInput, cmd = m.commandInput.Update(msg)
	if m.commandInput.Value() != prev {
		m.importAgentCursor = 0
		m.importAgentOffset = 0
	}
	m.recomputeImportAgentMatches()
	m.applyLayout(false)
	return m, cmd
}

func (m Model) handleImportSkillPickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "enter":
		m.applyImportSkillSelections()
		m.historyIndex = len(m.history)
		m.applyLayout(true)
		return m, nil
	case " ", "space":
		m.toggleImportSkillSelection()
		m.applyLayout(false)
		return m, nil
	case "up":
		m.moveImportSkillPicker(-1)
		m.applyLayout(false)
		return m, nil
	case "down", "tab":
		m.moveImportSkillPicker(1)
		m.applyLayout(false)
		return m, nil
	case "pgup", "pgdown", "home", "end":
		var cmd tea.Cmd
		m.chatViewport, cmd = m.chatViewport.Update(msg)
		return m, cmd
	case "esc":
		m.exitImportSkillPicker(true)
		m.historyIndex = len(m.history)
		m.applyLayout(false)
		return m, nil
	}

	var cmd tea.Cmd
	prev := m.commandInput.Value()
	m.commandInput, cmd = m.commandInput.Update(msg)
	if m.commandInput.Value() != prev {
		m.importSkillCursor = 0
		m.importSkillOffset = 0
	}
	m.recomputeImportSkillMatches()
	m.applyLayout(false)
	return m, cmd
}

func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress {
		return m, nil
	}

	switch msg.Button {
	case tea.MouseButtonWheelUp:
		if m.skillPickerOpen {
			m.moveSkillPicker(-1)
			return m, nil
		}
		if m.importAgentPickerOpen {
			m.moveImportAgentPicker(-1)
			return m, nil
		}
		if m.importSkillPickerOpen {
			m.moveImportSkillPicker(-1)
			return m, nil
		}
		if m.paletteOpen() {
			m.movePalette(-1)
			return m, nil
		}
		var cmd tea.Cmd
		m.chatViewport, cmd = m.chatViewport.Update(msg)
		return m, cmd
	case tea.MouseButtonWheelDown:
		if m.skillPickerOpen {
			m.moveSkillPicker(1)
			return m, nil
		}
		if m.importAgentPickerOpen {
			m.moveImportAgentPicker(1)
			return m, nil
		}
		if m.importSkillPickerOpen {
			m.moveImportSkillPicker(1)
			return m, nil
		}
		if m.paletteOpen() {
			m.movePalette(1)
			return m, nil
		}
		var cmd tea.Cmd
		m.chatViewport, cmd = m.chatViewport.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) submitCommand() (tea.Model, tea.Cmd) {
	raw := strings.TrimSpace(m.commandInput.Value())
	if raw == "" {
		m.setOutput("", "Type / to open commands.")
		m.applyLayout(true)
		return m, nil
	}

	if !hasCommandPrefix(raw) {
		m.setOutput("", "Commands start with '/'. Try /help.")
		m.commandInput.SetValue("")
		m.commandInput.CursorEnd()
		m.recomputeMatches()
		m.historyIndex = len(m.history)
		m.applyLayout(true)
		return m, nil
	}

	if commandFragment(raw) == "" && m.paletteOpen() {
		m.autocompleteSelected()
		raw = strings.TrimSpace(m.commandInput.Value())
	}

	if commandFragment(raw) == "" {
		raw = "/help"
	}

	cmd, args, ok := resolveCommand(m.commands, raw)
	if !ok {
		if m.paletteOpen() {
			m.autocompleteSelected()
			raw = strings.TrimSpace(m.commandInput.Value())
			cmd, args, ok = resolveCommand(m.commands, raw)
			if !ok {
				m.applyLayout(false)
				return m, nil
			}
		} else {
			m.setOutput("", "Unknown command.\nTry /help.")
			m.commandInput.SetValue("")
			m.commandInput.CursorEnd()
			m.recomputeMatches()
			m.historyIndex = len(m.history)
			m.applyLayout(true)
			return m, nil
		}
	}

	raw = strings.TrimSpace(raw)

	m.appendHistory(raw)
	result := cmd.Run(&m, args)
	m.setOutput(raw, result.Output)

	if !result.KeepInput {
		m.commandInput.Placeholder = defaultInputPlaceholder
		m.commandInput.SetValue("")
		m.commandInput.CursorEnd()
	}
	m.recomputeMatches()
	m.historyIndex = len(m.history)
	m.applyLayout(true)

	if result.Quit {
		m.quitting = true
		return m, tea.Quit
	}

	return m, result.Cmd
}

func (m Model) submitRepoURL() (tea.Model, tea.Cmd) {
	raw := strings.TrimSpace(m.commandInput.Value())
	if raw == "" {
		m.setOutput("", warnStyle.Render("Repository URL cannot be empty."))
		m.applyLayout(true)
		return m, nil
	}

	m.appendHistory(raw)
	if _, err := config.NormalizeRepository(raw); err != nil {
		m.setOutput("/add", errorStyle.Render("Invalid repository URL: "+err.Error()))
		m.applyLayout(true)
		return m, nil
	}

	result := m.actionAddRepo(raw)
	m.setOutput("/add", result.Output)

	m.exitRepoURLPrompt(true)
	m.historyIndex = len(m.history)
	m.applyLayout(true)
	return m, result.Cmd
}

func (m *Model) recomputeMatches() {
	if m.skillPickerOpen {
		m.matches = nil
		m.paletteCursor = 0
		m.recomputeSkillMatches()
		return
	}

	if m.importAgentPickerOpen {
		m.matches = nil
		m.paletteCursor = 0
		m.recomputeImportAgentMatches()
		return
	}

	if m.importSkillPickerOpen {
		m.matches = nil
		m.paletteCursor = 0
		m.recomputeImportSkillMatches()
		return
	}

	if !hasCommandPrefix(m.commandInput.Value()) {
		m.matches = nil
		m.paletteCursor = 0
		return
	}

	fragment := commandFragment(m.commandInput.Value())
	m.matches = matchCommands(m.commands, fragment)
	visible := m.visibleMatches()
	if len(visible) == 0 {
		m.paletteCursor = 0
		return
	}
	if fragment == "" {
		for i, match := range visible {
			if match.Command.Name == "help" {
				m.paletteCursor = i
				return
			}
		}
	}
	if m.paletteCursor >= len(visible) {
		m.paletteCursor = len(visible) - 1
	}
	if m.paletteCursor < 0 {
		m.paletteCursor = 0
	}
}

func (m Model) visibleMatches() []commandMatch {
	if len(m.matches) <= maxPaletteItems {
		return m.matches
	}
	return m.matches[:maxPaletteItems]
}

func (m Model) visibleSkillMatches() []skillMatch {
	limit := m.maxDropdownItems()
	if limit < 1 {
		limit = maxPaletteItems
	}
	if len(m.skillMatches) == 0 {
		return nil
	}

	start := m.skillOffset
	if start < 0 {
		start = 0
	}
	if start > len(m.skillMatches) {
		start = len(m.skillMatches)
	}

	end := start + limit
	if end > len(m.skillMatches) {
		end = len(m.skillMatches)
	}

	return m.skillMatches[start:end]
}

func (m Model) paletteOpen() bool {
	if m.anyPickerOpen() {
		return false
	}
	return hasCommandPrefix(m.commandInput.Value()) && len(m.visibleMatches()) > 0
}

func (m Model) anyPickerOpen() bool {
	return m.skillPickerOpen || m.importAgentPickerOpen || m.importSkillPickerOpen
}

func (m *Model) movePalette(delta int) {
	visible := m.visibleMatches()
	if len(visible) == 0 {
		return
	}
	m.paletteCursor += delta
	if m.paletteCursor < 0 {
		m.paletteCursor = len(visible) - 1
	}
	if m.paletteCursor >= len(visible) {
		m.paletteCursor = 0
	}
}

func (m *Model) moveSkillPicker(delta int) {
	if len(m.skillMatches) == 0 {
		return
	}

	m.skillCursor += delta
	if m.skillCursor < 0 {
		m.skillCursor = len(m.skillMatches) - 1
	}
	if m.skillCursor >= len(m.skillMatches) {
		m.skillCursor = 0
	}

	m.clampSkillWindow()
}

func (m *Model) moveImportAgentPicker(delta int) {
	if len(m.importAgentMatches) == 0 {
		return
	}

	m.importAgentCursor += delta
	if m.importAgentCursor < 0 {
		m.importAgentCursor = len(m.importAgentMatches) - 1
	}
	if m.importAgentCursor >= len(m.importAgentMatches) {
		m.importAgentCursor = 0
	}

	m.clampImportAgentWindow()
}

func (m *Model) moveImportSkillPicker(delta int) {
	if len(m.importSkillMatches) == 0 {
		return
	}

	m.importSkillCursor += delta
	if m.importSkillCursor < 0 {
		m.importSkillCursor = len(m.importSkillMatches) - 1
	}
	if m.importSkillCursor >= len(m.importSkillMatches) {
		m.importSkillCursor = 0
	}

	m.clampImportSkillWindow()
}

func (m *Model) enterSkillPicker() {
	m.exitImportAgentPicker(true)
	m.exitImportSkillPicker(true)
	m.skillPickerOpen = true
	m.skillCursor = 0
	m.skillOffset = 0
	m.skillPickerSelections = make(map[string]bool)
	m.commandInput.Placeholder = skillPickerPlaceholder
	m.commandInput.SetValue("")
	m.commandInput.CursorEnd()
	m.recomputeSkillMatches()
	m.applyLayout(false)
}

func (m *Model) enterImportAgentPicker() {
	m.exitSkillPicker(true)
	m.exitImportSkillPicker(true)

	agents := m.discoverImportAgents()
	if len(agents) == 0 {
		m.setOutput("/import", warnStyle.Render("No unmanaged local skills found in supported agent folders."))
		m.applyLayout(true)
		return
	}

	m.importAgentPickerOpen = true
	m.importAgentOptions = agents
	m.importAgentMatches = agents
	m.importAgentCursor = 0
	m.importAgentOffset = 0
	m.commandInput.Placeholder = importAgentPlaceholder
	m.commandInput.SetValue("")
	m.commandInput.CursorEnd()
	m.recomputeImportAgentMatches()
	m.setOutput("/import", infoStyle.Render("Select an agent to import unmanaged local skills. Press Enter to continue, Esc to cancel."))
	m.applyLayout(false)
}

func (m *Model) enterImportSkillPicker() {
	if len(m.importAgentMatches) == 0 {
		m.setOutput("/import", warnStyle.Render("No matching agents found."))
		m.applyLayout(true)
		return
	}

	if m.importAgentCursor < 0 || m.importAgentCursor >= len(m.importAgentMatches) {
		m.importAgentCursor = 0
	}

	agent := m.importAgentMatches[m.importAgentCursor]
	m.importAgentChosen = agent
	m.importAgentPickerOpen = false
	m.importAgentMatches = nil
	m.importAgentCursor = 0
	m.importAgentOffset = 0

	m.importSkillPickerOpen = true
	m.importSkillCursor = 0
	m.importSkillOffset = 0
	m.importSkillSelections = make(map[string]bool)
	m.commandInput.Placeholder = importSkillPlaceholder
	m.commandInput.SetValue("")
	m.commandInput.CursorEnd()
	m.recomputeImportSkillMatches()
	m.setOutput("/import", infoStyle.Render(fmt.Sprintf("Select unmanaged local skills from %s. Space toggles, Enter imports, Esc cancels.", agent.Name)))
	m.applyLayout(false)
}

func (m *Model) enterRepoURLPrompt() {
	m.awaitingRepoURL = true
	m.commandInput.Placeholder = repoURLPromptPlaceholder
	m.commandInput.SetValue("")
	m.commandInput.CursorEnd()
	m.setOutput("/add", infoStyle.Render("Enter repository URL and press Enter. Press Esc to cancel."))
	m.recomputeMatches()
	m.applyLayout(false)
}

func (m *Model) exitRepoURLPrompt(clearInput bool) {
	m.awaitingRepoURL = false
	m.commandInput.Placeholder = defaultInputPlaceholder
	if clearInput {
		m.commandInput.SetValue("")
		m.commandInput.CursorEnd()
	}
	m.recomputeMatches()
	m.applyLayout(false)
}

func (m *Model) exitImportAgentPicker(clearInput bool) {
	m.importAgentPickerOpen = false
	m.importAgentOptions = nil
	m.importAgentMatches = nil
	m.importAgentCursor = 0
	m.importAgentOffset = 0
	if !m.importSkillPickerOpen && !m.skillPickerOpen {
		m.commandInput.Placeholder = defaultInputPlaceholder
	}
	if clearInput {
		m.commandInput.SetValue("")
		m.commandInput.CursorEnd()
	}
	m.recomputeMatches()
	m.applyLayout(false)
}

func (m *Model) exitImportSkillPicker(clearInput bool) {
	m.importSkillPickerOpen = false
	m.importAgentChosen = importAgentOption{}
	m.importSkillMatches = nil
	m.importSkillCursor = 0
	m.importSkillOffset = 0
	m.importSkillSelections = nil
	if !m.importAgentPickerOpen && !m.skillPickerOpen {
		m.commandInput.Placeholder = defaultInputPlaceholder
	}
	if clearInput {
		m.commandInput.SetValue("")
		m.commandInput.CursorEnd()
	}
	m.recomputeMatches()
	m.applyLayout(false)
}

func (m *Model) exitSkillPicker(clearInput bool) {
	m.skillPickerOpen = false
	m.skillMatches = nil
	m.skillCursor = 0
	m.skillOffset = 0
	m.skillPickerSelections = nil
	m.commandInput.Placeholder = defaultInputPlaceholder
	if clearInput {
		m.commandInput.SetValue("")
		m.commandInput.CursorEnd()
	}
	m.recomputeMatches()
	m.applyLayout(false)
}

func (m *Model) recomputeSkillMatches() {
	m.skillMatches = m.matchAvailableSkills(m.commandInput.Value())
	if len(m.skillMatches) == 0 {
		m.skillCursor = 0
		m.skillOffset = 0
		return
	}

	m.clampSkillWindow()
}

func (m *Model) recomputeImportAgentMatches() {
	query := strings.ToLower(strings.TrimSpace(m.commandInput.Value()))
	if query == "" {
		m.importAgentMatches = append([]importAgentOption(nil), m.importAgentOptions...)
	} else {
		filtered := make([]importAgentOption, 0, len(m.importAgentOptions))
		for _, agent := range m.importAgentOptions {
			if strings.Contains(strings.ToLower(agent.Name), query) || strings.Contains(strings.ToLower(agent.ID), query) {
				filtered = append(filtered, agent)
			}
		}
		m.importAgentMatches = filtered
	}

	if len(m.importAgentMatches) == 0 {
		m.importAgentCursor = 0
		m.importAgentOffset = 0
		return
	}
	m.clampImportAgentWindow()
}

func (m *Model) recomputeImportSkillMatches() {
	query := strings.ToLower(strings.TrimSpace(m.commandInput.Value()))
	matches := make([]importSkillMatch, 0, len(m.importAgentChosen.Skills))
	for _, candidate := range m.importAgentChosen.Skills {
		if query != "" {
			name := strings.ToLower(candidate.Name)
			rel := strings.ToLower(candidate.Relative)
			if !strings.Contains(name, query) && !strings.Contains(rel, query) {
				continue
			}
		}
		matches = append(matches, importSkillMatch{Skill: candidate})
	}
	m.importSkillMatches = matches

	if len(m.importSkillMatches) == 0 {
		m.importSkillCursor = 0
		m.importSkillOffset = 0
		return
	}
	m.clampImportSkillWindow()
}

func (m *Model) applyImportSkillSelections() {
	if len(m.importSkillSelections) == 0 {
		m.setOutput("/import", infoStyle.Render("No selection changes."))
		m.exitImportSkillPicker(true)
		return
	}

	sourcePath := filepath.Join(m.paths.LocalDir, "imported-skills")
	repo, repoAdded, repoErr := m.ensureManagedImportSource(sourcePath)
	if repoErr != nil {
		m.setOutput("/import", errorStyle.Render("Failed to prepare managed local source: "+repoErr.Error()))
		m.exitImportSkillPicker(true)
		return
	}

	selected := m.selectedImportSkills()
	if len(selected) == 0 {
		m.setOutput("/import", infoStyle.Render("No selection changes."))
		m.exitImportSkillPicker(true)
		return
	}

	if err := os.MkdirAll(sourcePath, 0o755); err != nil {
		m.setOutput("/import", errorStyle.Render("Failed to create managed local source: "+err.Error()))
		m.exitImportSkillPicker(true)
		return
	}

	importedIDs := make([]string, 0, len(selected))
	usedNames := make(map[string]bool)
	for _, candidate := range selected {
		dirName := sanitizeImportDirName(candidate.Relative)
		if dirName == "" {
			dirName = sanitizeImportDirName(candidate.Name)
		}
		if dirName == "" {
			continue
		}
		base := dirName
		seq := 2
		for usedNames[strings.ToLower(dirName)] {
			dirName = fmt.Sprintf("%s-%d", base, seq)
			seq++
		}
		usedNames[strings.ToLower(dirName)] = true

		dstPath := filepath.Join(sourcePath, dirName)
		if err := os.RemoveAll(dstPath); err != nil {
			m.setOutput("/import", errorStyle.Render("Failed to update imported skill: "+err.Error()))
			m.exitImportSkillPicker(true)
			return
		}
		if err := copyDir(candidate.Source, dstPath); err != nil {
			m.setOutput("/import", errorStyle.Render("Failed to import skill: "+err.Error()))
			m.exitImportSkillPicker(true)
			return
		}
		importedIDs = append(importedIDs, repo.ID+"/"+dirName)
	}

	m.refresh()
	output := m.applySkillSelectionChanges(importedIDs, nil)
	if repoAdded {
		output = successStyle.Render("Created managed local import source.") + "\n\n" + output
	}
	m.setOutput("/import", output)
	m.exitImportSkillPicker(true)
}

func (m Model) selectedImportSkills() []importSkillCandidate {
	if len(m.importSkillSelections) == 0 {
		return nil
	}

	selected := make([]importSkillCandidate, 0, len(m.importSkillSelections))
	for _, match := range m.importSkillMatches {
		if !m.importSkillSelections[strings.ToLower(match.Skill.Key)] {
			continue
		}
		selected = append(selected, match.Skill)
	}
	if len(selected) > 0 {
		return selected
	}

	for _, candidate := range m.importAgentChosen.Skills {
		if m.importSkillSelections[strings.ToLower(candidate.Key)] {
			selected = append(selected, candidate)
		}
	}
	return selected
}

func (m *Model) toggleImportSkillSelection() {
	if len(m.importSkillMatches) == 0 {
		m.setOutput("/import", warnStyle.Render("No matching skills found."))
		return
	}

	if m.importSkillCursor < 0 || m.importSkillCursor >= len(m.importSkillMatches) {
		m.importSkillCursor = 0
	}

	chosen := m.importSkillMatches[m.importSkillCursor]
	key := strings.ToLower(chosen.Skill.Key)
	current := m.importSkillSelections[key]
	m.importSkillSelections[key] = !current
	if !m.importSkillSelections[key] {
		delete(m.importSkillSelections, key)
	}
}

func (m *Model) clampImportAgentWindow() {
	count := len(m.importAgentMatches)
	if count == 0 {
		m.importAgentCursor = 0
		m.importAgentOffset = 0
		return
	}

	if m.importAgentCursor < 0 {
		m.importAgentCursor = 0
	}
	if m.importAgentCursor >= count {
		m.importAgentCursor = count - 1
	}

	limit := m.maxDropdownItems()
	if limit < 1 {
		limit = maxPaletteItems
	}
	if limit > count {
		limit = count
	}

	maxOffset := count - limit
	if maxOffset < 0 {
		maxOffset = 0
	}

	if m.importAgentOffset < 0 {
		m.importAgentOffset = 0
	}
	if m.importAgentOffset > maxOffset {
		m.importAgentOffset = maxOffset
	}

	if m.importAgentCursor < m.importAgentOffset {
		m.importAgentOffset = m.importAgentCursor
	}
	if m.importAgentCursor >= m.importAgentOffset+limit {
		m.importAgentOffset = m.importAgentCursor - limit + 1
	}
}

func (m *Model) clampImportSkillWindow() {
	count := len(m.importSkillMatches)
	if count == 0 {
		m.importSkillCursor = 0
		m.importSkillOffset = 0
		return
	}

	if m.importSkillCursor < 0 {
		m.importSkillCursor = 0
	}
	if m.importSkillCursor >= count {
		m.importSkillCursor = count - 1
	}

	limit := m.maxDropdownItems()
	if limit < 1 {
		limit = maxPaletteItems
	}
	if limit > count {
		limit = count
	}

	maxOffset := count - limit
	if maxOffset < 0 {
		maxOffset = 0
	}

	if m.importSkillOffset < 0 {
		m.importSkillOffset = 0
	}
	if m.importSkillOffset > maxOffset {
		m.importSkillOffset = maxOffset
	}

	if m.importSkillCursor < m.importSkillOffset {
		m.importSkillOffset = m.importSkillCursor
	}
	if m.importSkillCursor >= m.importSkillOffset+limit {
		m.importSkillOffset = m.importSkillCursor - limit + 1
	}
}

func (m Model) visibleImportAgentMatches() []importAgentOption {
	limit := m.maxDropdownItems()
	if limit < 1 {
		limit = maxPaletteItems
	}
	if len(m.importAgentMatches) == 0 {
		return nil
	}

	start := m.importAgentOffset
	if start < 0 {
		start = 0
	}
	if start > len(m.importAgentMatches) {
		start = len(m.importAgentMatches)
	}

	end := start + limit
	if end > len(m.importAgentMatches) {
		end = len(m.importAgentMatches)
	}

	return m.importAgentMatches[start:end]
}

func (m Model) visibleImportSkillMatches() []importSkillMatch {
	limit := m.maxDropdownItems()
	if limit < 1 {
		limit = maxPaletteItems
	}
	if len(m.importSkillMatches) == 0 {
		return nil
	}

	start := m.importSkillOffset
	if start < 0 {
		start = 0
	}
	if start > len(m.importSkillMatches) {
		start = len(m.importSkillMatches)
	}

	end := start + limit
	if end > len(m.importSkillMatches) {
		end = len(m.importSkillMatches)
	}

	return m.importSkillMatches[start:end]
}

func (m *Model) ensureManagedImportSource(sourcePath string) (config.Repository, bool, error) {
	for _, repo := range m.cfg.Repositories {
		if strings.EqualFold(repo.ID, managedImportSourceID) {
			return repo, false, nil
		}
	}

	repo := config.Repository{
		ID:   managedImportSourceID,
		Type: config.RepositoryTypeLocal,
		Path: sourcePath,
	}
	m.cfg.Repositories = append(m.cfg.Repositories, repo)
	if err := config.SaveConfig(m.paths, m.cfg); err != nil {
		return config.Repository{}, false, err
	}
	return repo, true, nil
}

func sanitizeImportDirName(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.ReplaceAll(raw, string(filepath.Separator), "__")
	raw = strings.ReplaceAll(raw, "/", "__")
	raw = strings.ToLower(raw)
	if raw == "" {
		return ""
	}

	var b strings.Builder
	lastDash := false
	for _, r := range raw {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.'
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func copyDir(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("source is not a directory: %s", src)
	}

	if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}

		fileInfo, err := entry.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(srcPath)
		if err != nil {
			return err
		}
		if err := os.WriteFile(dstPath, data, fileInfo.Mode().Perm()); err != nil {
			return err
		}
	}

	return nil
}

func (m *Model) applySkillPickerSelections() {
	if len(m.available) == 0 {
		m.setOutput("", warnStyle.Render("No skills available yet. Upstream sources sync automatically on launch; use /pull to retry now."))
		m.exitSkillPicker(true)
		return
	}

	if len(m.skillPickerSelections) == 0 {
		m.setOutput("", infoStyle.Render("No selection changes."))
		m.exitSkillPicker(true)
		return
	}

	selectedByLower := make(map[string]string, len(m.cfg.SelectedSkills))
	for _, skill := range m.cfg.SelectedSkills {
		selectedByLower[strings.ToLower(skill)] = skill
	}

	availableByLower := make(map[string]string, len(m.availableIDs))
	for _, skillID := range m.availableIDs {
		availableByLower[strings.ToLower(skillID)] = skillID
	}

	addRequested := make([]string, 0, len(m.skillPickerSelections))
	removeRequested := make([]string, 0, len(m.skillPickerSelections))
	for lowerID, shouldSelect := range m.skillPickerSelections {
		if shouldSelect {
			if _, exists := selectedByLower[lowerID]; exists {
				continue
			}
			if resolved, ok := availableByLower[lowerID]; ok {
				addRequested = append(addRequested, resolved)
			}
			continue
		}

		if resolved, exists := selectedByLower[lowerID]; exists {
			removeRequested = append(removeRequested, resolved)
		}
	}

	sort.Strings(addRequested)
	sort.Strings(removeRequested)

	output := m.applySkillSelectionChanges(addRequested, removeRequested)
	if strings.TrimSpace(output) == "" {
		output = infoStyle.Render("No selection changes.")
	}
	m.setOutput("", output)
	m.exitSkillPicker(true)
}

func (m *Model) toggleSkillPickerSelection() {
	if len(m.skillMatches) == 0 {
		if len(m.available) == 0 {
			m.setOutput("", warnStyle.Render("No skills available yet. Upstream sources sync automatically on launch; use /pull to retry now."))
		} else {
			m.setOutput("", warnStyle.Render("No matching skills found."))
		}
		return
	}

	if m.skillCursor < 0 || m.skillCursor >= len(m.skillMatches) {
		m.skillCursor = 0
	}

	chosen := m.skillMatches[m.skillCursor]
	lowerID := strings.ToLower(chosen.Skill.ID)
	current := m.skillPickerSelected(chosen.Skill.ID, chosen.Selected)
	next := !current
	if next == chosen.Selected {
		delete(m.skillPickerSelections, lowerID)
		return
	}

	m.skillPickerSelections[lowerID] = next
}

func (m Model) skillPickerSelected(skillID string, fallback bool) bool {
	if len(m.skillPickerSelections) == 0 {
		return fallback
	}
	selected, ok := m.skillPickerSelections[strings.ToLower(skillID)]
	if !ok {
		return fallback
	}
	return selected
}

func (m *Model) clampSkillWindow() {
	count := len(m.skillMatches)
	if count == 0 {
		m.skillCursor = 0
		m.skillOffset = 0
		return
	}

	if m.skillCursor < 0 {
		m.skillCursor = 0
	}
	if m.skillCursor >= count {
		m.skillCursor = count - 1
	}

	limit := m.maxDropdownItems()
	if limit < 1 {
		limit = maxPaletteItems
	}
	if limit > count {
		limit = count
	}

	maxOffset := count - limit
	if maxOffset < 0 {
		maxOffset = 0
	}

	if m.skillOffset < 0 {
		m.skillOffset = 0
	}
	if m.skillOffset > maxOffset {
		m.skillOffset = maxOffset
	}

	if m.skillCursor < m.skillOffset {
		m.skillOffset = m.skillCursor
	}
	if m.skillCursor >= m.skillOffset+limit {
		m.skillOffset = m.skillCursor - limit + 1
	}

	if m.skillOffset < 0 {
		m.skillOffset = 0
	}
	if m.skillOffset > maxOffset {
		m.skillOffset = maxOffset
	}
}

func (m *Model) autocompleteSelected() {
	visible := m.visibleMatches()
	if len(visible) == 0 {
		return
	}
	if m.paletteCursor < 0 || m.paletteCursor >= len(visible) {
		m.paletteCursor = 0
	}
	chosen := visible[m.paletteCursor].Command.Name
	m.commandInput.SetValue("/" + chosen + " ")
	m.commandInput.CursorEnd()
	m.recomputeMatches()
}

func (m *Model) appendHistory(cmd string) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return
	}
	if len(m.history) == 0 || m.history[len(m.history)-1] != cmd {
		m.history = append(m.history, cmd)
	}
	m.historyIndex = len(m.history)
}

func (m *Model) historyPrev() {
	if len(m.history) == 0 {
		return
	}
	if m.historyIndex <= 0 {
		m.historyIndex = 0
	} else {
		m.historyIndex--
	}
	m.commandInput.SetValue(m.history[m.historyIndex])
	m.commandInput.CursorEnd()
	m.recomputeMatches()
}

func (m *Model) historyNext() {
	if len(m.history) == 0 {
		m.commandInput.SetValue("")
		m.commandInput.CursorEnd()
		m.recomputeMatches()
		return
	}
	if m.historyIndex >= len(m.history)-1 {
		m.historyIndex = len(m.history)
		m.commandInput.SetValue("")
		m.commandInput.CursorEnd()
		m.recomputeMatches()
		return
	}
	m.historyIndex++
	m.commandInput.SetValue(m.history[m.historyIndex])
	m.commandInput.CursorEnd()
	m.recomputeMatches()
}

func (m *Model) setOutput(label, content string) {
	m.outputLabel = strings.TrimRight(label, "\n")
	m.outputContent = strings.TrimRight(content, "\n")
	m.refreshChatViewport(true)
}

func (m *Model) clearOutput() {
	m.outputLabel = ""
	m.outputContent = ""
	m.refreshChatViewport(true)
}

func (m *Model) updateOutputContent(content string) {
	m.outputContent = strings.TrimRight(content, "\n")
	m.refreshChatViewport(true)
}

func (m *Model) applyLayout(stickBottom bool) {
	if m.width <= 0 {
		m.contentWidth = defaultContentWidth
	} else {
		gutter := 8
		if m.compactLayout() {
			gutter = 2
		}

		maxAllowed := m.width - gutter
		if maxAllowed < 18 {
			maxAllowed = m.width - 1
		}
		if maxAllowed < minContentWidth {
			maxAllowed = minContentWidth
		}

		m.contentWidth = maxAllowed
	}

	inputInset := 4
	if m.compactLayout() {
		inputInset = 2
	}

	inputWidth := m.contentWidth - inputInset
	if inputWidth < 10 {
		inputWidth = 10
	}
	if inputWidth >= m.contentWidth {
		inputWidth = max(8, m.contentWidth-1)
	}
	m.commandInput.Width = inputWidth

	m.refreshChatViewport(stickBottom)
}

func (m Model) maxDropdownItems() int {
	limit := 6
	if m.height > 0 {
		dynamic := m.height / 7
		if dynamic < 2 {
			dynamic = 2
		}
		if dynamic > 6 {
			dynamic = 6
		}
		limit = dynamic
	}
	if limit > maxPaletteItems {
		limit = maxPaletteItems
	}
	return limit
}

func (m Model) computeViewportHeight() int {
	if m.height <= 0 {
		return 12
	}

	reserved := 11
	if m.compactLayout() {
		reserved = 6
	}
	if m.skillPickerOpen {
		reserved += len(m.visibleSkillMatches()) + 3
	} else if m.importAgentPickerOpen {
		reserved += len(m.visibleImportAgentMatches()) + 3
	} else if m.importSkillPickerOpen {
		reserved += len(m.visibleImportSkillMatches()) + 3
	} else if m.paletteOpen() {
		reserved += min(len(m.visibleMatches()), m.maxDropdownItems()) + 2
	}

	h := m.height - reserved
	if h < 1 {
		h = 1
	}
	return h
}

func (m *Model) refreshChatViewport(stickBottom bool) {
	width := m.contentWidth
	if width <= 0 {
		width = defaultContentWidth
	}
	if width < 10 {
		width = 10
	}
	if m.width > 0 {
		maxWidth := m.width - 1
		if maxWidth > 0 && width > maxWidth {
			width = maxWidth
		}
	}

	height := m.computeViewportHeight()
	if height < 1 {
		height = 1
	}

	m.chatViewport.Width = width
	m.chatViewport.Height = height
	m.chatViewport.SetContent(m.renderChatViewportContent(width))
	if stickBottom {
		m.chatViewport.GotoBottom()
	}
}

func (m Model) compactLayout() bool {
	if m.width > 0 && m.width < 96 {
		return true
	}
	if m.height > 0 && m.height < 22 {
		return true
	}
	return false
}

func (m Model) tinyLayout() bool {
	if m.width > 0 && m.width < 72 {
		return true
	}
	if m.height > 0 && m.height < 14 {
		return true
	}
	return false
}

func (m Model) discoverImportAgents() []importAgentOption {
	agentTargets := []struct {
		id   string
		name string
		path string
	}{
		{id: "claude", name: "Claude", path: "~/.claude/skills"},
		{id: "opencode", name: "OpenCode", path: "~/.config/opencode/skills"},
		{id: "gemini", name: "Gemini", path: "~/.gemini/antigravity/skills"},
		{id: "cursor", name: "Cursor", path: "~/.cursor/skills"},
		{id: "codex", name: "Codex", path: "~/.codex/skills"},
		{id: "kiro", name: "Kiro", path: "~/.kiro/skills"},
	}

	managed := make(map[string]bool, len(m.cfg.SelectedSkills))
	for _, skillID := range m.cfg.SelectedSkills {
		installDir := strings.ToLower(config.SkillInstallDirName(skillID))
		if installDir != "" {
			managed[installDir] = true
		}

		leaf := selectedSkillLeafName(skillID)
		if leaf != "" {
			managed[strings.ToLower(leaf)] = true

			sanitizedLeaf := strings.TrimPrefix(config.SkillInstallDirName("skill/"+leaf), "skill--")
			if sanitizedLeaf != "" {
				managed[strings.ToLower(sanitizedLeaf)] = true
			}
		}
	}

	agents := make([]importAgentOption, 0, len(agentTargets))
	for _, agent := range agentTargets {
		root := config.ExpandPath(agent.path)
		info, err := os.Stat(root)
		if err != nil || !info.IsDir() {
			continue
		}

		skills := discoverUnmanagedSkills(root, managed)
		if len(skills) == 0 {
			continue
		}

		agents = append(agents, importAgentOption{
			ID:     agent.id,
			Name:   agent.name,
			Path:   root,
			Skills: skills,
		})
	}

	sort.SliceStable(agents, func(i, j int) bool {
		return agents[i].Name < agents[j].Name
	})
	return agents
}

func discoverUnmanagedSkills(root string, managed map[string]bool) []importSkillCandidate {
	candidates := make([]importSkillCandidate, 0)
	seen := make(map[string]bool)

	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		if rel == "." {
			return nil
		}
		if hasHiddenSegment(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || d.Name() != "SKILL.md" {
			return nil
		}

		skillDir := filepath.Dir(path)
		skillRel, relSkillErr := filepath.Rel(root, skillDir)
		if relSkillErr != nil {
			return nil
		}
		topLevel := topLevelSegment(skillRel)
		if topLevel == "" {
			return nil
		}
		if managed[strings.ToLower(topLevel)] {
			return nil
		}

		key := strings.ToLower(skillRel)
		if seen[key] {
			return nil
		}
		seen[key] = true

		candidates = append(candidates, importSkillCandidate{
			Key:      key,
			Name:     filepath.Base(skillDir),
			Source:   skillDir,
			Relative: filepath.ToSlash(skillRel),
		})
		return nil
	})

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Name != candidates[j].Name {
			return candidates[i].Name < candidates[j].Name
		}
		return candidates[i].Relative < candidates[j].Relative
	})

	return candidates
}

func hasHiddenSegment(relPath string) bool {
	parts := strings.Split(filepath.ToSlash(relPath), "/")
	for _, part := range parts {
		if strings.HasPrefix(part, ".") {
			return true
		}
	}
	return false
}

func selectedSkillLeafName(skillID string) string {
	skillID = strings.TrimSpace(skillID)
	if skillID == "" {
		return ""
	}
	if slash := strings.Index(skillID, "/"); slash >= 0 && slash+1 < len(skillID) {
		return strings.TrimSpace(skillID[slash+1:])
	}
	return skillID
}

func topLevelSegment(relPath string) string {
	relPath = strings.TrimSpace(filepath.ToSlash(relPath))
	if relPath == "" {
		return ""
	}
	parts := strings.Split(relPath, "/")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}

func hasCommandPrefix(raw string) bool {
	raw = strings.TrimLeft(raw, " ")
	return strings.HasPrefix(raw, "/")
}

func commandFragment(raw string) string {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "/") {
		return ""
	}
	raw = strings.TrimPrefix(raw, "/")
	return strings.TrimSpace(raw)
}

// View renders the current state.
func (m Model) View() string {
	if m.quitting {
		return renderGoodbye(m.width)
	}

	return m.renderChatWorkspace()
}

// refresh reloads config and available skills.
func (m *Model) refresh() {
	m.cfg = config.LoadConfig(m.paths)
	m.available = config.LoadAvailableSkills(m.paths, m.cfg)
	m.availableIDs = config.SkillIDs(m.available)
	if m.skillPickerOpen {
		m.recomputeSkillMatches()
		return
	}
	if m.importAgentPickerOpen {
		m.recomputeImportAgentMatches()
		return
	}
	if m.importSkillPickerOpen {
		m.recomputeImportSkillMatches()
		return
	}
	m.recomputeMatches()
}
