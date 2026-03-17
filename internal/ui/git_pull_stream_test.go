package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"akhilsingh.in/skillctl/internal/config"
	"akhilsingh.in/skillctl/internal/core"
)

func TestStartAutoGitPullCmd(t *testing.T) {
	cmd := startAutoGitPullCmd()
	require.NotNil(t, cmd)
	_, ok := cmd().(autoGitPullMsg)
	assert.True(t, ok)
}

func TestWaitForGitPullEventCmd(t *testing.T) {
	events := make(chan tea.Msg, 1)
	events <- gitPullChunkMsg{chunk: "hello"}

	msg := waitForGitPullEventCmd(events)()
	chunk, ok := msg.(gitPullChunkMsg)
	require.True(t, ok)
	assert.Equal(t, "hello", chunk.chunk)

	close(events)
	assert.Nil(t, waitForGitPullEventCmd(events)())
}

func TestStartGitPullStreamCmd(t *testing.T) {
	paths := config.ResolvePaths(t.TempDir())
	repositories := []config.Repository{{
		ID:   "local-source",
		Type: config.RepositoryTypeLocal,
		Path: t.TempDir(),
	}}

	msg := startGitPullStreamCmd(paths, repositories)()
	started, ok := msg.(gitPullStreamStartedMsg)
	require.True(t, ok)
	require.NotNil(t, started.events)

	var chunks []gitPullChunkMsg
	var done *gitPullDoneMsg
	for {
		next := waitForGitPullEventCmd(started.events)()
		if next == nil {
			break
		}
		switch m := next.(type) {
		case gitPullChunkMsg:
			chunks = append(chunks, m)
		case gitPullDoneMsg:
			d := m
			done = &d
		}
	}

	require.NotNil(t, done)
	assert.True(t, done.outcome.Success())
	require.Len(t, done.outcome.Results, 1)
	assert.Equal(t, core.RepoPullResult{RepoID: "local-source", RepoURL: "", Action: "skip", ReturnCode: 0, Stdout: "", Stderr: "", LocalRepoDir: repositories[0].Path}, done.outcome.Results[0])

	require.NotEmpty(t, chunks)
	assert.Contains(t, chunks[0].chunk, "SKIP (local source)")
	assert.False(t, chunks[0].isStderr)
}
