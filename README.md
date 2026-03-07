# skillctl

An interactive TUI for managing curated AI skills across multiple agent folders.

`skillctl` syncs skill directories from a central source repository to your AI agent target
folders — keeping Claude, Gemini, Cursor, Codex, Kiro, OpenCode, and more up-to-date with a
single command.

## Install

### Homebrew (recommended)

```bash
brew tap akl773/skillctl
brew install skillctl
```

### From source

```bash
git clone https://github.com/akl773/skillctl.git
cd skillctl
make install
```

> Requires Go 1.24+

## Prerequisites

- [`git`](https://git-scm.com/) — for pulling updates from the source repo
- [`rsync`](https://rsync.samba.org/) — for syncing skills to target directories (pre-installed on macOS and most Linux distros)
- A skills source repository cloned at `~/.skills-curated` (configurable)

## Usage

```bash
# Launch the interactive menu
skillctl

# Use a custom source repo
skillctl --source-repo /path/to/skills-repo

# Print version
skillctl --version
```

## How It Works

1. A curated skills repository lives at `~/.skills-curated` (or set via `--source-repo` / `$SKILLCTL_SOURCE_REPO`)
2. You browse the skill catalog and select which skills you want
3. `skillctl` rsyncs those skills into each of your configured target directories
4. Targets are agent-specific skill folders (e.g. `~/.claude/skills`, `~/.config/opencode/skills`)

Configuration is stored at `<source-repo>/.local/skillctl.json`.

## Default Targets

| Agent | Target path |
|-------|------------|
| Claude | `~/.claude/skills` |
| OpenCode | `~/.config/opencode/skills` |
| Gemini | `~/.gemini/antigravity/skills` |
| Cursor | `~/.cursor/skills/antigravity-awesome-skills/skills` |
| Codex | `~/.codex/skills` |
| Kiro | `~/.kiro/skills` |

Targets can be added or removed from within the TUI at any time.

## Features

- **Interactive TUI** — keyboard-driven terminal UI with search and navigation
- **Skill catalog** — browse and add skills from the source repo
- **Multi-target sync** — rsync selected skills to all configured agent folders at once
- **Git integration** — pull latest changes from the source repo
- **Target management** — add and remove target directories on the fly

## Development

```bash
make build    # Build binary (injects version from git tag)
make run      # Build and run
make clean    # Remove binary
make install  # Install to $GOPATH/bin
make test     # Run unit tests
make cover    # Run tests with coverage summary
```

## Architecture and Testing

- Architecture guide: [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md)
- Unit tests follow table-driven Go style with `testify/assert` and
  `testify/require`, inspired by mature open-source Go CLI projects.
- Priority coverage targets are `internal/config`, `internal/core`, and command
  parsing helpers in `internal/ui`.

## Releasing

Releases are automated via [GoReleaser](https://goreleaser.com/) and GitHub Actions.
Push a version tag to trigger a release and update the Homebrew formula automatically:

```bash
git tag v1.0.0
git push origin v1.0.0
```

## License

MIT
