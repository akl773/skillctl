# skillctl

An interactive TUI for managing and syncing AI skills from multiple GitHub repositories into local agent folders.

`skillctl` keeps your Claude, Gemini, Cursor, Codex, Kiro, OpenCode, and other agent skill directories up to date from one place.

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

- [`git`](https://git-scm.com/) — for cloning and updating source repositories
- [`rsync`](https://rsync.samba.org/) — for syncing skills to target directories (pre-installed on macOS and most Linux distros)

## Usage

```bash
# Launch the interactive menu
skillctl

# Use a custom workspace
skillctl --workspace /path/to/skillctl-workspace

# Print version
skillctl --version
```

## Default Source Repositories

These repositories are included out of the box:

- `https://github.com/vercel-labs/agent-skills.git`
- `https://github.com/callstackincubator/agent-skills.git`
- `https://github.com/tech-leads-club/agent-skills.git`
- `https://github.com/ComposioHQ/awesome-claude-skills.git`
- `https://github.com/sickn33/antigravity-awesome-skills.git`

You can add and remove repositories at runtime via `/repo add` and `/repo remove`.

## How It Works

1. `skillctl` stores its workspace at `~/.skillctl` by default (override via `--workspace` or `$SKILLCTL_WORKSPACE`).
2. Repository definitions and selections are stored in `~/.skillctl/.local/skillctl.json`.
3. On launch, `skillctl` automatically clones or updates each configured repository in the background into `~/.skillctl/.local/repos/<repo-id>`.
4. `/pull` is still available if you want to force a manual refresh immediately.
5. `skillctl` recursively discovers skills by finding `SKILL.md` files in those local clones.
6. Skills are selected using namespaced IDs: `<repo-id>/<skill-name>`.
7. `/sync` rsyncs selected skills to each configured target.

To prevent cross-repo collisions, target directory names use a namespaced layout:

- skill ID: `vercel-labs-agent-skills/react-best-practices`
- installed folder: `vercel-labs-agent-skills--react-best-practices`

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

## Common Workflow

1. `/repos` to inspect configured repositories
2. `/pull` (optional) to force an immediate repository refresh
3. `/search <query>` to find skills
4. `/add <skill-id>` (or `/add <catalog-number>`) to select skills
5. `/sync` to deploy selected skills to all targets

## Features

- **Interactive TUI** — keyboard-driven terminal UI with search and navigation
- **Multi-repo catalog** — aggregate skills from many repositories
- **Namespaced skill IDs** — avoid collisions across repositories
- **Multi-target sync** — rsync selected skills to all configured agent folders at once
- **Repository management** — add/remove/list repositories on the fly
- **Auto background updates** — repositories sync automatically on app launch
- **Git integration** — run `/pull` anytime for an immediate refresh

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
- Unit tests follow table-driven Go style with `testify/assert` and `testify/require`.
- Priority coverage targets are `internal/config`, `internal/core`, and command parsing in `internal/ui`.

## Releasing

Releases are automated via [GoReleaser](https://goreleaser.com/) and GitHub Actions.
Push a version tag to trigger a release and update the Homebrew formula automatically:

```bash
git tag v1.0.0
git push origin v1.0.0
```

## License

MIT
