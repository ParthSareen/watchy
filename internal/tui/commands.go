package tui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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
		lines, err := mgr.TailLogs(taskID, 200)
		if err != nil {
			return logContentMsg("")
		}
		content := ""
		for i, line := range lines {
			if i > 0 {
				content += "\n"
			}
			content += logcolor.Colorize(line)
		}
		return logContentMsg(content)
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
