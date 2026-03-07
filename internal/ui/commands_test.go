package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScoreCommand(t *testing.T) {
	commands := testCommandDefs()

	tests := []struct {
		name      string
		query     string
		cmd       commandDef
		wantScore int
		wantOK    bool
	}{
		{name: "empty query matches all", query: "", cmd: commands[0], wantScore: 100, wantOK: true},
		{name: "exact name", query: "list", cmd: commands[1], wantScore: 0, wantOK: true},
		{name: "name with args", query: "list 2", cmd: commands[1], wantScore: 1, wantOK: true},
		{name: "name prefix", query: "li", cmd: commands[1], wantScore: 2, wantOK: true},
		{name: "exact alias", query: "ls", cmd: commands[1], wantScore: 3, wantOK: true},
		{name: "alias with args", query: "ls all", cmd: commands[1], wantScore: 4, wantOK: true},
		{name: "alias prefix", query: "up", cmd: commands[2], wantScore: 5, wantOK: true},
		{name: "alias prefix wins before name contains", query: "tog", cmd: commands[3], wantScore: 5, wantOK: true},
		{name: "alias contains", query: "dat", cmd: commands[2], wantScore: 7, wantOK: true},
		{name: "description contains", query: "latest", cmd: commands[2], wantScore: 8, wantOK: true},
		{name: "no match", query: "zzz", cmd: commands[0], wantScore: 0, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score, ok := scoreCommand(tt.query, tt.cmd)
			assert.Equal(t, tt.wantScore, score)
			assert.Equal(t, tt.wantOK, ok)
		})
	}
}

func TestMatchCommands(t *testing.T) {
	commands := testCommandDefs()

	t.Run("empty query returns all sorted by name", func(t *testing.T) {
		matches := matchCommands(commands, "")
		require.Len(t, matches, len(commands))

		names := []string{matches[0].Command.Name, matches[1].Command.Name, matches[2].Command.Name, matches[3].Command.Name}
		assert.Equal(t, []string{"help", "list", "list toggle", "pull"}, names)
	})

	t.Run("sorts by score then name", func(t *testing.T) {
		matches := matchCommands(commands, "li")
		require.NotEmpty(t, matches)
		assert.Equal(t, "list", matches[0].Command.Name)
		assert.Equal(t, 2, matches[0].Score)
	})

	t.Run("returns empty for no matches", func(t *testing.T) {
		matches := matchCommands(commands, "does-not-exist")
		assert.Empty(t, matches)
	})
}

func TestResolveCommand(t *testing.T) {
	commands := testCommandDefs()

	tests := []struct {
		name     string
		raw      string
		wantName string
		wantArgs string
		wantOK   bool
	}{
		{name: "exact name with slash", raw: "/help", wantName: "help", wantArgs: "", wantOK: true},
		{name: "exact name without slash", raw: "help", wantName: "help", wantArgs: "", wantOK: true},
		{name: "name with args", raw: "/help add", wantName: "help", wantArgs: "add", wantOK: true},
		{name: "multi word command uses longest match", raw: "/list toggle 2", wantName: "list toggle", wantArgs: "2", wantOK: true},
		{name: "multi word alias", raw: "/toggle alpha", wantName: "list toggle", wantArgs: "alpha", wantOK: true},
		{name: "alias with args", raw: "/up now", wantName: "pull", wantArgs: "now", wantOK: true},
		{name: "unknown command", raw: "/unknown", wantName: "", wantArgs: "", wantOK: false},
		{name: "empty command", raw: "/", wantName: "", wantArgs: "", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, args, ok := resolveCommand(commands, tt.raw)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantArgs, args)
			if tt.wantOK {
				assert.Equal(t, tt.wantName, cmd.Name)
			}
		})
	}
}

func TestFindCommandByAlias(t *testing.T) {
	commands := testCommandDefs()

	cmd, ok := findCommandByAlias(commands, "LS")
	require.True(t, ok)
	assert.Equal(t, "list", cmd.Name)

	cmd, ok = findCommandByAlias(commands, "toggle")
	require.True(t, ok)
	assert.Equal(t, "list toggle", cmd.Name)

	_, ok = findCommandByAlias(commands, "nope")
	assert.False(t, ok)
}

func TestCommandHelpers(t *testing.T) {
	t.Run("command fragment", func(t *testing.T) {
		assert.Equal(t, "help", commandFragment("/help"))
		assert.Equal(t, "sync", commandFragment(" /sync "))
		assert.Equal(t, "", commandFragment("plain"))
		assert.Equal(t, "", commandFragment("/"))
		assert.Equal(t, "target add ~/skills", commandFragment("/target add ~/skills"))
	})

	t.Run("command prefix", func(t *testing.T) {
		assert.True(t, hasCommandPrefix("/help"))
		assert.True(t, hasCommandPrefix("  /help"))
		assert.True(t, hasCommandPrefix(" /"))
		assert.False(t, hasCommandPrefix("help"))
		assert.False(t, hasCommandPrefix(""))
	})
}

func TestBuiltInCommands(t *testing.T) {
	commands := builtInCommands()
	require.GreaterOrEqual(t, len(commands), 10)

	seenNames := make(map[string]bool)
	seenAliases := make(map[string]bool)

	for _, cmd := range commands {
		require.NotEmpty(t, cmd.Name)
		require.NotEmpty(t, cmd.Description)
		require.NotEmpty(t, cmd.Usage)
		assert.True(t, cmd.Run != nil)
		assert.False(t, seenNames[cmd.Name], "duplicate command name: %s", cmd.Name)
		seenNames[cmd.Name] = true

		for _, alias := range cmd.Aliases {
			require.NotEmpty(t, alias)
			assert.False(t, seenAliases[alias], "duplicate alias: %s", alias)
			seenAliases[alias] = true
		}
	}

	clearCmd, ok := findCommandByAlias(commands, "clear")
	require.True(t, ok)
	assert.Equal(t, "clear", clearCmd.Name)
}

func TestResolveBuiltInRepoCommands(t *testing.T) {
	commands := builtInCommands()

	cmd, args, ok := resolveCommand(commands, "/repo add https://github.com/foo/bar")
	require.True(t, ok)
	assert.Equal(t, "repo add", cmd.Name)
	assert.Equal(t, "https://github.com/foo/bar", args)

	cmd, args, ok = resolveCommand(commands, "/repo remove vercel-labs-agent-skills")
	require.True(t, ok)
	assert.Equal(t, "repo remove", cmd.Name)
	assert.Equal(t, "vercel-labs-agent-skills", args)

	cmd, args, ok = resolveCommand(commands, "/repos")
	require.True(t, ok)
	assert.Equal(t, "repos", cmd.Name)
	assert.Equal(t, "", args)
}

func testCommandDefs() []commandDef {
	return []commandDef{
		{
			Name:        "help",
			Aliases:     []string{"h", "?"},
			Description: "Show command help",
			Usage:       "/help [command]",
		},
		{
			Name:        "list",
			Aliases:     []string{"ls"},
			Description: "List selected skills",
			Usage:       "/list",
		},
		{
			Name:        "pull",
			Aliases:     []string{"update", "up"},
			Description: "Pull latest changes",
			Usage:       "/pull",
		},
		{
			Name:        "list toggle",
			Aliases:     []string{"toggle", "lt"},
			Description: "Toggle selected skill",
			Usage:       "/list toggle <skill>",
		},
	}
}
