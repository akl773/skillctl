# skillctl Architecture

This document explains how `skillctl` is organized so contributors can add
features and tests quickly.

## High-Level Design

`skillctl` is a Go terminal app that synchronizes selected skills from multiple
GitHub repositories into multiple local agent target directories.

Core responsibilities:

1. Manage repository sources (`/repos`, `/repo add`, `/repo remove`)
2. Clone/update repositories into a local workspace cache (`/pull`)
3. Discover skills generically by scanning for `SKILL.md`
4. Persist user config in `<workspace>/.local/skillctl.json`
5. Sync selected skills into configured target folders (`/sync`)
6. Present all interactions through a Bubble Tea TUI

## Workspace Layout

Default workspace: `~/.skillctl`

```text
~/.skillctl/
  .local/
    skillctl.json
    state.json
    repos/
      <repo-id>/
      <repo-id>/
```

Key behaviors:

- Repositories are cloned under `.local/repos/<repo-id>`
- Skills are discovered recursively from cloned repositories
- Selected skill IDs are namespaced as `<repo-id>/<skill-name>`
- Target folder names are collision-safe: `<repo-id>--<skill-name>`

## Package Layout

### `cmd/skillctl`

- `main.go`: process entrypoint
- Parses CLI flags (`--workspace`, `--version`)
- Resolves paths and validates setup
- Boots Bubble Tea with `ui.NewModel(...)`

### `internal/config`

Configuration and path layer.

- Data types:
  - `AppPaths`: workspace/config/cache paths
  - `Repository`: normalized repository metadata (`id`, `url`)
  - `Config`: selected skills, disabled skills, targets, repositories
  - `AvailableSkill`: discovered catalog entry with source path
  - `State`: runtime metadata (`last_sync_at`)
- Responsibilities:
  - Resolve and normalize paths (`ResolvePaths`, `ExpandPath`, `CompactPath`)
  - Normalize repository references (`NormalizeRepository`)
  - Read/write JSON config (`LoadConfig`, `SaveConfig`, `ReadJSON`, `WriteJSON`)
  - Bootstrap workspace files (`EnsureSetup`)
  - Discover skills recursively (`LoadAvailableSkills`)

This package is mostly pure logic and is the easiest place to add fast,
deterministic unit tests.

### `internal/core`

Business logic for skill operations.

- `RunGitPull` / `RunGitPullStream`: clone/pull all configured repositories
- `AddRequestedSkills`: resolve requested IDs and update selected list
- `RemoveSelectedSkills`: remove selected IDs and clean target folders
- `SyncSelectedSkills`: `rsync` selected catalog entries to every target
- Outcome structs (`GitPullOutcome`, `AddOutcome`, `RemoveOutcome`, `SyncOutcome`)
  encapsulate side-effect results for UI rendering

`core` owns domain behavior, while `ui` orchestrates it.

### `internal/ui`

Bubble Tea presentation layer.

- `menu.go`: model state, update loop, key handling, viewport behavior
- `commands.go`: slash command definitions, command matching/resolution
- `actions.go`: command handlers that call `config` and `core`
- `views.go`: style system and rendering of chat/workspace panels
- `git_pull_stream.go`: async message bridge for streamed repository updates

UI remains intentionally thin: parse input, call `core`, render outcomes.

## Data Flow (Example: `/sync`)

1. User enters `/sync`.
2. `ui.resolveCommand` routes to `actionSync`.
3. `actionSync` calls `core.SyncSelectedSkills(cfg, available)`.
4. `core` resolves selected IDs against available catalog entries.
5. `core` `rsync`s each selected skill into each target directory.
6. `core.SyncOutcome` is formatted and rendered in the chat viewport.

## External Boundaries

- `git` command for clone/pull repository operations
- `rsync` command for target synchronization
- Filesystem reads/writes via `os` and `filepath`

Because these boundaries are concrete, pure logic is prioritized for unit tests,
while command-execution paths are better covered by integration tests.

## Testing Strategy

Current tests follow mature Go CLI patterns:

- Table-driven tests with clear subtest names
- `testify/require` for setup preconditions
- `testify/assert` for behavior assertions
- `t.TempDir()` for filesystem-based tests
- Package-local fixtures for realistic scenarios

### Priority Testing Areas

1. `internal/config`: normalization, repository parsing, discovery behavior
2. `internal/core`: add/remove logic and sync outcome aggregation
3. `internal/ui` command parser: matching and resolution behavior

### Follow-Up Improvements

To improve coverage for command execution in `core`, introduce a command runner
interface so tests can stub external process execution (`git`, `rsync`).

## Contributor Playbook

When adding a command or feature:

1. Put domain logic in `internal/core` (or `internal/config` for path/config)
2. Keep `internal/ui/actions.go` focused on orchestration and formatting
3. Add/extend command metadata in `internal/ui/commands.go`
4. Add table-driven unit tests for new behavior
5. Run:

```bash
make test
make cover
```
