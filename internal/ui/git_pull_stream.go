package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"akhilsingh.in/skillctl/internal/config"
	"akhilsingh.in/skillctl/internal/core"
)

type gitPullStreamStartedMsg struct {
	events <-chan tea.Msg
}

type gitPullChunkMsg struct {
	chunk    string
	isStderr bool
}

type gitPullDoneMsg struct {
	outcome core.GitPullOutcome
}

func startGitPullStreamCmd(paths config.AppPaths) tea.Cmd {
	return func() tea.Msg {
		events := make(chan tea.Msg, 128)
		go func() {
			defer close(events)
			outcome := core.RunGitPullStream(paths,
				func(chunk string) {
					events <- gitPullChunkMsg{chunk: chunk}
				},
				func(chunk string) {
					events <- gitPullChunkMsg{chunk: chunk, isStderr: true}
				},
			)
			events <- gitPullDoneMsg{outcome: outcome}
		}()

		return gitPullStreamStartedMsg{events: events}
	}
}

func waitForGitPullEventCmd(events <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-events
		if !ok {
			return nil
		}
		return msg
	}
}
