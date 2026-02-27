package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/atotto/clipboard"
	"github.com/parth/watchy/internal/agent"
	"github.com/parth/watchy/internal/logcolor"
	"github.com/parth/watchy/internal/task"
)

func fetchTasks(mgr *task.Manager) tea.Cmd {
	return func() tea.Msg {
		tasks, err := mgr.ListTasks()
		if err != nil {
			return tasksUpdatedMsg(nil)
		}
		return tasksUpdatedMsg(tasks)
	}
}

func fetchLogs(mgr *task.Manager, taskID int) tea.Cmd {
	return func() tea.Msg {
		lines, err := mgr.TailLogs(taskID, 0)
		if err != nil {
			return logContentMsg{}
		}
		var raw, colored strings.Builder
		for i, line := range lines {
			if i > 0 {
				raw.WriteString("\n")
				colored.WriteString("\n")
			}
			raw.WriteString(line)
			colored.WriteString(logcolor.Colorize(line))
		}
		return logContentMsg{raw: raw.String(), colored: colored.String()}
	}
}

// sendToAgent runs the agent loop, sending tool call events back to the TUI
// via p.Send so they appear in real time.
func sendToAgent(conv *agent.Conversation, msg string, ctx context.Context, p *tea.Program) tea.Cmd {
	return func() tea.Msg {
		resp, err := conv.SendWithEvents(ctx, msg,
			func(evt agent.ToolStartEvent) {
				p.Send(agentToolStartMsg(evt))
			},
			func(evt agent.ToolResultEvent) {
				p.Send(agentToolResultMsg(evt))
			},
		)
		if err != nil {
			if ctx.Err() != nil {
				return agentErrorMsg{err: fmt.Errorf("cancelled")}
			}
			return agentErrorMsg{err: err}
		}
		return agentResponseMsg(resp)
	}
}

func stopTask(mgr *task.Manager, id int) tea.Cmd {
	return func() tea.Msg {
		mgr.StopTask(id)
		return taskStoppedMsg(id)
	}
}

func restartTaskCmd(mgr *task.Manager, id int) tea.Cmd {
	return func() tea.Msg {
		newTaskID, err := mgr.RestartTask(id)
		if err != nil {
			return taskRestartedMsg(0)
		}
		return taskRestartedMsg(newTaskID)
	}
}

func tickEvery(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func copyToClipboard(text string) tea.Cmd {
	return func() tea.Msg {
		clipboard.WriteAll(text)
		return clipboardCopiedMsg{}
	}
}
