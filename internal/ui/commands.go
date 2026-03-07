package ui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type commandResult struct {
	Output string
	Quit   bool
	Clear  bool
	Cmd    tea.Cmd
}

type commandDef struct {
	Name        string
	Aliases     []string
	Description string
	Usage       string
	Examples    []string
	Run         func(m *Model, args string) commandResult
}

type commandMatch struct {
	Command commandDef
	Score   int
}

func builtInCommands() []commandDef {
	return []commandDef{
		{
			Name:        "help",
			Aliases:     []string{"h", "?"},
			Description: "Show all commands or command details",
			Usage:       "/help [command]",
			Examples:    []string{"/help", "/help add", "/help target add"},
			Run: func(m *Model, args string) commandResult {
				return commandResult{Output: m.renderHelp(args)}
			},
		},
		{
			Name:        "pull",
			Aliases:     []string{"update", "git pull"},
			Description: "Pull latest changes in source repo",
			Usage:       "/pull",
			Examples:    []string{"/pull"},
			Run: func(m *Model, args string) commandResult {
				return m.actionGitPull()
			},
		},
		{
			Name:        "list",
			Aliases:     []string{"ls"},
			Description: "List selected skills with status",
			Usage:       "/list",
			Examples:    []string{"/list"},
			Run: func(m *Model, args string) commandResult {
				return commandResult{Output: m.actionListSelected()}
			},
		},
		{
			Name:        "list toggle",
			Aliases:     []string{"toggle", "lt"},
			Description: "Enable or disable a selected skill",
			Usage:       "/list toggle <name|index>",
			Examples:    []string{"/list toggle 2", "/list toggle security-auditor", "/toggle 2"},
			Run: func(m *Model, args string) commandResult {
				args = strings.TrimSpace(args)
				if args == "" {
					return commandResult{Output: errorStyle.Render("Usage: /list toggle <name|index>")}
				}
				return commandResult{Output: m.actionToggleSkill(args)}
			},
		},
		{
			Name:        "search",
			Aliases:     []string{"find"},
			Description: "Search the skill catalog",
			Usage:       "/search <query>",
			Examples:    []string{"/search review", "/search ui"},
			Run: func(m *Model, args string) commandResult {
				args = strings.TrimSpace(args)
				if args == "" {
					return commandResult{Output: errorStyle.Render("Usage: /search <query>")}
				}
				return commandResult{Output: m.actionSearch(args)}
			},
		},
		{
			Name:        "add",
			Aliases:     []string{"install"},
			Description: "Add selected skills by name or number",
			Usage:       "/add <name|index[,name|index...]>",
			Examples:    []string{"/add code-reviewer", "/add 1,2,3"},
			Run: func(m *Model, args string) commandResult {
				args = strings.TrimSpace(args)
				if args == "" {
					return commandResult{Output: errorStyle.Render("Usage: /add <name or index list>")}
				}
				return commandResult{Output: m.actionAddSkill(args)}
			},
		},
		{
			Name:        "remove",
			Aliases:     []string{"rm", "delete"},
			Description: "Remove selected skills by name or number",
			Usage:       "/remove <name|index[,name|index...]>",
			Examples:    []string{"/remove seo-audit", "/remove 2,4"},
			Run: func(m *Model, args string) commandResult {
				args = strings.TrimSpace(args)
				if args == "" {
					return commandResult{Output: errorStyle.Render("Usage: /remove <name or index list>")}
				}
				return commandResult{Output: m.actionRemoveSkill(args)}
			},
		},
		{
			Name:        "sync",
			Aliases:     []string{"push"},
			Description: "Sync selected skills to all targets",
			Usage:       "/sync",
			Examples:    []string{"/sync"},
			Run: func(m *Model, args string) commandResult {
				return commandResult{Output: m.actionSync()}
			},
		},
		{
			Name:        "targets",
			Aliases:     []string{"target list"},
			Description: "List target directories",
			Usage:       "/targets",
			Examples:    []string{"/targets"},
			Run: func(m *Model, args string) commandResult {
				return commandResult{Output: m.actionListTargets()}
			},
		},
		{
			Name:        "target add",
			Aliases:     []string{"ta"},
			Description: "Add one target directory",
			Usage:       "/target add <path>",
			Examples:    []string{"/target add ~/.codex/skills"},
			Run: func(m *Model, args string) commandResult {
				args = strings.TrimSpace(args)
				if args == "" {
					return commandResult{Output: errorStyle.Render("Usage: /target add <path>")}
				}
				return commandResult{Output: m.actionAddTarget(args)}
			},
		},
		{
			Name:        "target remove",
			Aliases:     []string{"tr"},
			Description: "Remove target by index or path",
			Usage:       "/target remove <index|path>",
			Examples:    []string{"/target remove 2", "/target remove ~/.kiro/skills"},
			Run: func(m *Model, args string) commandResult {
				args = strings.TrimSpace(args)
				if args == "" {
					return commandResult{Output: errorStyle.Render("Usage: /target remove <index or path>")}
				}
				return commandResult{Output: m.actionRemoveTarget(args)}
			},
		},
		{
			Name:        "status",
			Aliases:     []string{"info"},
			Description: "Show full status overview",
			Usage:       "/status",
			Examples:    []string{"/status"},
			Run: func(m *Model, args string) commandResult {
				return commandResult{Output: m.actionStatus()}
			},
		},
		{
			Name:        "clear",
			Aliases:     []string{"cls"},
			Description: "Clear chat history",
			Usage:       "/clear",
			Examples:    []string{"/clear"},
			Run: func(m *Model, args string) commandResult {
				return commandResult{Output: infoStyle.Render("Conversation cleared."), Clear: true}
			},
		},
		{
			Name:        "quit",
			Aliases:     []string{"exit", "q"},
			Description: "Exit skillctl",
			Usage:       "/quit",
			Examples:    []string{"/quit"},
			Run: func(m *Model, args string) commandResult {
				return commandResult{Output: infoStyle.Render("Goodbye."), Quit: true}
			},
		},
	}
}

func (m Model) renderHelp(raw string) string {
	query := strings.TrimSpace(strings.ToLower(raw))
	if query == "" {
		var sb strings.Builder
		sb.WriteString(infoStyle.Render("Slash Commands") + "\n")
		sb.WriteString(mutedStyle.Render(strings.Repeat("-", 78)) + "\n")
		for _, cmd := range m.commands {
			sb.WriteString(fmt.Sprintf("/%-14s %s\n", cmd.Name, cmd.Description))
		}
		sb.WriteString(mutedStyle.Render(strings.Repeat("-", 78)) + "\n")
		sb.WriteString(mutedStyle.Render("Tips: Up/Down to pick a command, Tab to autocomplete, Enter to run."))
		sb.WriteString("\n")
		sb.WriteString(mutedStyle.Render("Use /help <command> for detailed usage."))
		return sb.String()
	}

	cmd, ok := findCommandByAlias(m.commands, query)
	if !ok {
		return errorStyle.Render("Unknown command: "+raw) + "\n" + mutedStyle.Render("Try /help.")
	}

	var sb strings.Builder
	sb.WriteString(infoStyle.Render("Command Details") + "\n")
	sb.WriteString(fmt.Sprintf("Name        : /%s\n", cmd.Name))
	sb.WriteString(fmt.Sprintf("Description : %s\n", cmd.Description))
	sb.WriteString(fmt.Sprintf("Usage       : %s\n", cmd.Usage))
	if len(cmd.Aliases) > 0 {
		sb.WriteString(fmt.Sprintf("Aliases     : %s\n", strings.Join(cmd.Aliases, ", ")))
	}
	if len(cmd.Examples) > 0 {
		sb.WriteString("Examples    :\n")
		for _, ex := range cmd.Examples {
			sb.WriteString("  " + ex + "\n")
		}
	}
	return sb.String()
}

func matchCommands(commands []commandDef, fragment string) []commandMatch {
	query := strings.TrimSpace(strings.ToLower(fragment))
	matches := make([]commandMatch, 0, len(commands))
	for _, cmd := range commands {
		score, ok := scoreCommand(query, cmd)
		if ok {
			matches = append(matches, commandMatch{Command: cmd, Score: score})
		}
	}

	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score < matches[j].Score
		}
		return matches[i].Command.Name < matches[j].Command.Name
	})

	return matches
}

func scoreCommand(query string, cmd commandDef) (int, bool) {
	if query == "" {
		return 100, true
	}

	name := strings.ToLower(cmd.Name)
	if query == name {
		return 0, true
	}
	if strings.HasPrefix(query, name+" ") {
		return 1, true
	}
	if strings.HasPrefix(name, query) {
		return 2, true
	}

	for _, alias := range cmd.Aliases {
		alias = strings.ToLower(alias)
		if query == alias {
			return 3, true
		}
		if strings.HasPrefix(query, alias+" ") {
			return 4, true
		}
		if strings.HasPrefix(alias, query) {
			return 5, true
		}
	}

	if strings.Contains(name, query) {
		return 6, true
	}
	for _, alias := range cmd.Aliases {
		if strings.Contains(strings.ToLower(alias), query) {
			return 7, true
		}
	}
	if strings.Contains(strings.ToLower(cmd.Description), query) {
		return 8, true
	}

	return 0, false
}

func resolveCommand(commands []commandDef, raw string) (commandDef, string, bool) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "/")
	if raw == "" {
		return commandDef{}, "", false
	}

	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return commandDef{}, "", false
	}

	lowerParts := make([]string, 0, len(parts))
	for _, p := range parts {
		lowerParts = append(lowerParts, strings.ToLower(p))
	}

	bestLen := -1
	bestIdx := -1

	for i, cmd := range commands {
		candidates := append([]string{cmd.Name}, cmd.Aliases...)
		for _, cand := range candidates {
			candParts := strings.Fields(strings.ToLower(cand))
			if len(candParts) > len(lowerParts) {
				continue
			}

			ok := true
			for j := range candParts {
				if lowerParts[j] != candParts[j] {
					ok = false
					break
				}
			}
			if !ok {
				continue
			}

			if len(candParts) > bestLen {
				bestLen = len(candParts)
				bestIdx = i
			}
		}
	}

	if bestIdx == -1 {
		return commandDef{}, "", false
	}

	args := ""
	if bestLen < len(parts) {
		args = strings.Join(parts[bestLen:], " ")
	}

	return commands[bestIdx], args, true
}

func findCommandByAlias(commands []commandDef, query string) (commandDef, bool) {
	query = strings.TrimSpace(strings.ToLower(query))
	for _, cmd := range commands {
		if strings.ToLower(cmd.Name) == query {
			return cmd, true
		}
		for _, alias := range cmd.Aliases {
			if strings.ToLower(alias) == query {
				return cmd, true
			}
		}
	}
	return commandDef{}, false
}
