# skillctl

An interactive CLI for managing a curated set of AI skills across multiple agent folders.

`skillctl` syncs skill directories from a central source repository to various AI agent target
folders (Claude, Gemini, Cursor, Codex, Kiro, OpenCode, etc.) — keeping them up-to-date with a
single command.

## Features

- **Interactive TUI** — beautiful terminal UI with keyboard navigation
- **Skill catalog** — browse, search, and add skills from the source repo
- **Multi-target sync** — rsync selected skills to all configured agent folders
- **Git integration** — pull latest changes from the source repo
- **Target management** — add/remove target agent folders on the fly

## Install

### From source

```bash
git clone https://github.com/akl773/skillctl.git
cd skillctl
make build
```

### Homebrew (coming soon)

```bash
brew tap akl773/skillctl
brew install skillctl
```

## Usage

```bash
# Launch the interactive menu
skillctl

# Use a custom source repo
skillctl --source-repo /path/to/skills-repo

# Print version
skillctl --version
```

## Development

```bash
make build    # Build binary
make run      # Build and run
make clean    # Remove binary
make install  # Install to $GOPATH/bin
```

## How It Works

1. A curated skills repository lives at `~/.skills-curated` (configurable)
2. You select which skills you want from the catalog
3. `skillctl` rsyncs those skills into each of your configured target directories
4. Targets are agent-specific skill folders (e.g. `~/.claude/skills`, `~/.gemini/antigravity/skills`)

Configuration is stored at `<source-repo>/.local/skillctl.json`.
