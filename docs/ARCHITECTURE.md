# skillctl Architecture

This document explains how `skillctl` is organized so new contributors can
quickly add features and tests.

## High-Level Design

`skillctl` is a Go terminal app that synchronizes selected skills from a source
repository into multiple target directories used by AI tools.

Core responsibilities:

1. Discover available skills from `<source-repo>/skills`
2. Persist user config in `<source-repo>/.local/skillctl.json`
3. Execute operational commands (`git pull`, `rsync`, add/remove/toggle)
4. Present all interactions through a Bubble Tea TUI

## Package Layout

### `cmd/skillctl`

- `main.go`: process entrypoint
- Parses CLI flags (`--source-repo`, `--version`)
- Resolves paths and validates setup
- Boots the Bubble Tea program with `ui.NewModel(...)`

### `internal/config`

Configuration and path layer.

- Data types:
  - `AppPaths`: all filesystem locations used by the app
  - `Config`: selected skills, disabled skills, sync targets
  - `State`: runtime metadata (`last_sync_at`)
- Responsibilities:
  - Resolve and normalize paths (`ResolvePaths`, `ExpandPath`, `CompactPath`)
  - Read/write JSON config (`LoadConfig`, `SaveConfig`, `ReadJSON`, `WriteJSON`)
  - Bootstrap local metadata (`EnsureSetup`)
  - Utility parsing helpers used across packages (`InputCSV`, `SplitByReference`)

This package has a high amount of pure logic and is the easiest place to add
fast, deterministic unit tests.

### `internal/core`

Business logic for skill operations.

- `RunGitPull` / `RunGitPullStream`: execute and stream git pull output
- `AddRequestedSkills`: resolve requested names and update selected list
- `RemoveSelectedSkills`: remove selected skills and clean target folders
- `SyncSelectedSkills`: `rsync` selected skills to every configured target
- Outcome structs (`AddOutcome`, `RemoveOutcome`, `SyncOutcome`) encapsulate
  side-effect results for UI rendering

`core` owns domain behavior, while `ui` is a consumer of these outcomes.

### `internal/ui`

Bubble Tea presentation layer.

- `menu.go`: model state, update loop, key handling, viewport behavior
- `commands.go`: slash command definitions, command matching/resolution
- `actions.go`: command handlers that call `config` and `core`
- `views.go`: style system and rendering of chat/workspace panels
- `git_pull_stream.go`: async message bridge for streamed pull output

The UI should remain thin where possible: parse input, call `core`, render
outcomes.

## Data Flow (Example: `/sync`)

1. User enters `/sync` in the TUI.
2. `ui.resolveCommand` maps input to the `sync` command handler.
3. `ui.actionSync` calls `core.SyncSelectedSkills(paths, cfg, available)`.
4. `core` computes missing skills and runs `rsync` for each target.
5. `core.SyncOutcome` is returned to `ui`.
6. `ui` formats the result and appends it to the chat viewport.

## External Boundaries

- `git` command is invoked for repository updates
- `rsync` command is invoked for target synchronization
- Filesystem is read/written directly through `os` and `filepath`

Because these boundaries are concrete today, pure logic is prioritized for unit
testing and command execution paths are better covered by integration tests.

## Testing Strategy

Current tests follow patterns common in mature Go CLI projects such as
`cli/cli` (GitHub CLI):

- Table-driven tests with clear subtest names
- `testify/require` for setup preconditions
- `testify/assert` for behavior assertions
- `t.TempDir()` for filesystem-based tests
- Package-local helper builders for realistic test fixtures

### Priority Testing Areas

1. `internal/config`: normalization, path helpers, config load/save behavior
2. `internal/core`: add/remove logic and outcome aggregation
3. `internal/ui` command parser: matching and resolution behavior

### Follow-Up Improvements

To improve coverage for `RunGitPull*` and `SyncSelectedSkills`, introduce a
command runner interface in `core` so tests can stub external process
execution, similar to `cli/cli`'s command stubbing approach.

## Contributor Playbook

When adding a new command or feature:

1. Put domain logic in `internal/core` (or `internal/config` if config/path)
2. Keep `internal/ui/actions.go` focused on orchestration and formatting
3. Add/extend command metadata in `internal/ui/commands.go`
4. Add table-driven unit tests for the new core behavior
5. Run:

```bash
make test
make cover
```
