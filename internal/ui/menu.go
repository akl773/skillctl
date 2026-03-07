package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"akhilsingh.in/skillctl/internal/config"
)

const maxPaletteItems = 24

const (
	modeCommand = iota
	modeOutput
)

// Model is the root bubbletea model.
type Model struct {
	paths     config.AppPaths
	cfg       config.Config
	available []string
	width     int
	height    int

	quitting       bool
	output         string
	mode           int
	outputViewport viewport.Model
	lastCommand    string

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
	ti.Placeholder = "/help"
	ti.SetValue("/")
	ti.CursorEnd()
	_ = ti.Focus()

	vp := viewport.New(80, 20)

	m := Model{
		paths:          paths,
		cfg:            config.LoadConfig(paths),
		available:      config.LoadAvailableSkills(paths),
		output:         infoStyle.Render("Type / to browse commands. Try /help to get started."),
		mode:           modeCommand,
		outputViewport: vp,
		commandInput:   ti,
		commands:       builtInCommands(),
		historyIndex:   0,
	}
	m.recomputeMatches()
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
		m.commandInput.Width = max(24, msg.Width-14)
		m.outputViewport.Width = msg.Width
		m.outputViewport.Height = msg.Height - 4
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseMsg:
		return m.handleMouse(msg)
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		m.quitting = true
		return m, tea.Quit
	}

	switch m.mode {
	case modeOutput:
		return m.handleOutputKey(msg)
	default:
		return m.handleCommandKey(msg)
	}
}

func (m Model) handleOutputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "/":
		m.mode = modeCommand
		m.commandInput.SetValue("/")
		m.commandInput.CursorEnd()
		m.recomputeMatches()
		return m, nil
	}
	var cmd tea.Cmd
	m.outputViewport, cmd = m.outputViewport.Update(msg)
	return m, cmd
}

func (m Model) handleCommandKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "enter":
		return m.submitCommand()
	case "tab":
		m.autocompleteSelected()
		return m, nil
	case "up":
		if m.paletteOpen() {
			m.movePalette(-1)
			return m, nil
		}
		m.historyPrev()
		return m, nil
	case "down":
		if m.paletteOpen() {
			m.movePalette(1)
			return m, nil
		}
		m.historyNext()
		return m, nil
	case "ctrl+p":
		m.historyPrev()
		return m, nil
	case "ctrl+n":
		m.historyNext()
		return m, nil
	case "esc":
		m.commandInput.SetValue("/")
		m.commandInput.CursorEnd()
		m.historyIndex = len(m.history)
		m.recomputeMatches()
		return m, nil
	}

	var cmd tea.Cmd
	m.commandInput, cmd = m.commandInput.Update(msg)
	m.normalizeInput()
	m.historyIndex = len(m.history)
	m.recomputeMatches()
	return m, cmd
}

func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if !m.paletteOpen() {
		return m, nil
	}

	if msg.Action != tea.MouseActionPress {
		return m, nil
	}

	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.movePalette(-1)
		return m, nil
	case tea.MouseButtonWheelDown:
		m.movePalette(1)
		return m, nil
	case tea.MouseButtonLeft:
		start := m.paletteStartRow()
		idx := msg.Y - start
		visible := m.visibleMatches()
		if idx >= 0 && idx < len(visible) {
			m.paletteCursor = idx
			m.autocompleteSelected()
		}
		return m, nil
	}

	return m, nil
}

func (m Model) submitCommand() (tea.Model, tea.Cmd) {
	raw := strings.TrimSpace(m.commandInput.Value())
	if raw == "" || raw == "/" {
		if m.paletteOpen() && len(m.visibleMatches()) > 0 {
			m.autocompleteSelected()
			return m, nil
		}
		m.output = m.renderHelp("")
		m.outputViewport.SetContent(m.output)
		m.outputViewport.GotoTop()
		m.lastCommand = "/help"
		m.mode = modeOutput
		m.commandInput.SetValue("/")
		m.commandInput.CursorEnd()
		m.recomputeMatches()
		return m, nil
	}
	if !strings.HasPrefix(raw, "/") {
		raw = "/" + raw
	}

	cmd, args, ok := resolveCommand(m.commands, raw)
	if !ok {
		if m.paletteOpen() {
			m.autocompleteSelected()
			return m, nil
		}
		m.output = errorStyle.Render("Unknown command.") + "\n" + mutedStyle.Render("Try /help.")
		return m, nil
	}

	m.appendHistory(raw)
	result := cmd.Run(&m, args)
	if strings.TrimSpace(result.Output) != "" {
		m.output = result.Output
		m.lastCommand = raw
		m.outputViewport.SetContent(result.Output)
		m.outputViewport.GotoTop()
		m.mode = modeOutput
	}

	m.commandInput.SetValue("/")
	m.commandInput.CursorEnd()
	m.recomputeMatches()
	m.historyIndex = len(m.history)

	if result.Quit {
		m.quitting = true
		return m, tea.Quit
	}

	return m, nil
}

func (m *Model) normalizeInput() {
	value := m.commandInput.Value()
	if strings.TrimSpace(value) == "" {
		m.commandInput.SetValue("/")
		m.commandInput.CursorEnd()
		return
	}

	trimmedLeft := strings.TrimLeft(value, " ")
	if !strings.HasPrefix(trimmedLeft, "/") {
		trimmedLeft = "/" + trimmedLeft
	}

	if trimmedLeft != value {
		m.commandInput.SetValue(trimmedLeft)
		m.commandInput.CursorEnd()
	}
}

func (m *Model) recomputeMatches() {
	m.matches = matchCommands(m.commands, commandFragment(m.commandInput.Value()))
	visible := m.visibleMatches()
	if len(visible) == 0 {
		m.paletteCursor = 0
		return
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
	return len(m.visibleMatches()) > 0
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
		return
	}
	if m.historyIndex >= len(m.history)-1 {
		m.historyIndex = len(m.history)
		m.commandInput.SetValue("/")
		m.commandInput.CursorEnd()
		m.recomputeMatches()
		return
	}
	m.historyIndex++
	m.commandInput.SetValue(m.history[m.historyIndex])
	m.commandInput.CursorEnd()
	m.recomputeMatches()
}

func (m Model) paletteStartRow() int {
	return 6
}

func commandFragment(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "/") {
		raw = strings.TrimSpace(raw[1:])
	}
	return raw
}

// View renders the current state.
func (m Model) View() string {
	if m.quitting {
		return renderGoodbye(m.width)
	}

	switch m.mode {
	case modeOutput:
		return m.renderOutputView()
	default:
		return m.renderCommandWorkspace()
	}
}

// refresh reloads config and available skills.
func (m *Model) refresh() {
	m.cfg = config.LoadConfig(m.paths)
	m.available = config.LoadAvailableSkills(m.paths)
}
