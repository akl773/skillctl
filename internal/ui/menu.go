package ui

import (
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
	repoURLPromptPlaceholder = "Type repository URL and press Enter..."
)

type skillMatch struct {
	Skill        config.AvailableSkill
	CatalogIndex int
	Selected     bool
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
			m.setOutput("", errorStyle.Render("Background repository sync failed. Run /pull to inspect details and retry."))
		} else {
			m.gitPullOutput.WriteString("\n" + errorStyle.Render("ERROR: one or more repository updates failed. Resolve git issues before syncing."))
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

	output := m.actionAddRepo(raw)
	m.setOutput("/add", output)

	m.exitRepoURLPrompt(true)
	m.historyIndex = len(m.history)
	m.applyLayout(true)
	return m, nil
}

func (m *Model) recomputeMatches() {
	if m.skillPickerOpen {
		m.matches = nil
		m.paletteCursor = 0
		m.recomputeSkillMatches()
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
	if m.skillPickerOpen {
		return false
	}
	return hasCommandPrefix(m.commandInput.Value()) && len(m.visibleMatches()) > 0
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

func (m *Model) enterSkillPicker() {
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

func (m *Model) applySkillPickerSelections() {
	if len(m.available) == 0 {
		m.setOutput("", warnStyle.Render("No skills available yet. Repositories sync automatically on launch; use /pull to retry now."))
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
			m.setOutput("", warnStyle.Render("No skills available yet. Repositories sync automatically on launch; use /pull to retry now."))
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
	m.recomputeMatches()
}
