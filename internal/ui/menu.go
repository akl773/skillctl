package ui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"akhilsingh.in/skillctl/internal/config"
)

const maxPaletteItems = 24

const (
	defaultContentWidth = 90
	minContentWidth     = 10
)

type chatMessageType string

const (
	chatMessageCommand chatMessageType = "command"
	chatMessageOutput  chatMessageType = "output"
	chatMessageInfo    chatMessageType = "info"
	chatMessageError   chatMessageType = "error"
)

type chatMessage struct {
	ID        int
	Type      chatMessageType
	Content   string
	Timestamp time.Time
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
	chatMessages  []chatMessage
	nextMessageID int

	gitPullRunning   bool
	gitPullEvents    <-chan tea.Msg
	gitPullOutput    *strings.Builder
	gitPullMessageID int

	commandInput  textinput.Model
	commands      []commandDef
	matches       []commandMatch
	paletteCursor int

	history      []string
	historyIndex int
}

// NewModel creates a new UI model.
func NewModel(paths config.AppPaths) Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 512
	ti.Width = 72
	ti.Placeholder = "Type / for commands..."
	ti.SetValue("")
	ti.CursorEnd()
	_ = ti.Focus()

	vp := viewport.New(defaultContentWidth, 12)

	cfg := config.LoadConfig(paths)
	available := config.LoadAvailableSkills(paths, cfg)
	availableIDs := config.SkillIDs(available)

	m := Model{
		paths:            paths,
		cfg:              cfg,
		available:        available,
		availableIDs:     availableIDs,
		contentWidth:     defaultContentWidth,
		chatViewport:     vp,
		gitPullOutput:    new(strings.Builder),
		gitPullMessageID: -1,
		commandInput:     ti,
		commands:         builtInCommands(),
		historyIndex:     0,
	}
	m.recomputeMatches()
	m.refreshChatViewport(false)
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.WindowSize()
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

	case gitPullStreamStartedMsg:
		m.gitPullEvents = msg.events
		if m.gitPullEvents == nil {
			m.gitPullRunning = false
			return m, nil
		}
		return m, waitForGitPullEventCmd(m.gitPullEvents)

	case gitPullChunkMsg:
		if msg.isStderr {
			m.gitPullOutput.WriteString(mutedStyle.Render(msg.chunk))
		} else {
			m.gitPullOutput.WriteString(msg.chunk)
		}
		m.upsertGitPullMessage(m.gitPullOutput.String())
		if m.gitPullEvents != nil {
			return m, waitForGitPullEventCmd(m.gitPullEvents)
		}
		return m, nil

	case gitPullDoneMsg:
		if msg.outcome.Success() {
			m.gitPullOutput.WriteString("\n" + successStyle.Render("OK: repositories are up to date."))
			m.refresh()
		} else {
			m.gitPullOutput.WriteString("\n" + errorStyle.Render("ERROR: one or more repository updates failed. Resolve git issues before syncing."))
		}
		m.upsertGitPullMessage(m.gitPullOutput.String())
		m.gitPullRunning = false
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
		m.commandInput.SetValue("")
		m.commandInput.CursorEnd()
		m.historyIndex = len(m.history)
		m.recomputeMatches()
		m.applyLayout(false)
		return m, nil
	}

	var cmd tea.Cmd
	m.commandInput, cmd = m.commandInput.Update(msg)
	m.historyIndex = len(m.history)
	m.recomputeMatches()
	m.applyLayout(false)
	return m, cmd
}

func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress {
		return m, nil
	}

	switch msg.Button {
	case tea.MouseButtonWheelUp:
		if m.paletteOpen() {
			m.movePalette(-1)
			return m, nil
		}
		var cmd tea.Cmd
		m.chatViewport, cmd = m.chatViewport.Update(msg)
		return m, cmd
	case tea.MouseButtonWheelDown:
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
		m.addChatMessage(chatMessageInfo, "Type / to open commands.")
		m.applyLayout(true)
		return m, nil
	}

	if !hasCommandPrefix(raw) {
		m.addChatMessage(chatMessageInfo, "Commands start with '/'. Try /help.")
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
			m.addChatMessage(chatMessageError, "Unknown command.\nTry /help.")
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

	if result.Clear {
		m.clearChatMessages()
	} else {
		m.addChatMessage(chatMessageCommand, raw)
	}

	if strings.TrimSpace(result.Output) != "" {
		msgType := chatMessageOutput
		if cmd.Name == "help" {
			msgType = chatMessageInfo
		}
		messageID := m.addChatMessage(msgType, result.Output)
		if m.gitPullRunning {
			m.gitPullMessageID = messageID
		} else {
			m.gitPullMessageID = -1
		}
	} else if !m.gitPullRunning {
		m.gitPullMessageID = -1
	}

	m.commandInput.SetValue("")
	m.commandInput.CursorEnd()
	m.recomputeMatches()
	m.historyIndex = len(m.history)
	m.applyLayout(true)

	if result.Quit {
		m.quitting = true
		return m, tea.Quit
	}

	return m, result.Cmd
}

func (m *Model) recomputeMatches() {
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

func (m Model) paletteOpen() bool {
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

func (m *Model) addChatMessage(messageType chatMessageType, content string) int {
	content = strings.TrimRight(content, "\n")
	if strings.TrimSpace(content) == "" {
		return -1
	}

	id := m.nextMessageID
	m.nextMessageID++
	m.chatMessages = append(m.chatMessages, chatMessage{
		ID:        id,
		Type:      messageType,
		Content:   content,
		Timestamp: time.Now(),
	})
	m.refreshChatViewport(true)
	return id
}

func (m *Model) updateChatMessage(id int, content string) bool {
	content = strings.TrimRight(content, "\n")
	for i := range m.chatMessages {
		if m.chatMessages[i].ID == id {
			m.chatMessages[i].Content = content
			m.chatMessages[i].Timestamp = time.Now()
			m.refreshChatViewport(true)
			return true
		}
	}
	return false
}

func (m *Model) clearChatMessages() {
	m.chatMessages = nil
	m.nextMessageID = 0
	m.gitPullMessageID = -1
	m.gitPullOutput = new(strings.Builder)
	m.refreshChatViewport(true)
}

func (m *Model) upsertGitPullMessage(content string) {
	if m.gitPullMessageID >= 0 && m.updateChatMessage(m.gitPullMessageID, content) {
		return
	}
	m.gitPullMessageID = m.addChatMessage(chatMessageOutput, content)
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
	if m.paletteOpen() {
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
}
