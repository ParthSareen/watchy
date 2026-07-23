package tui

import (
	"context"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/parth/watchy/internal/agent"
	"github.com/parth/watchy/internal/logcolor"
	"github.com/parth/watchy/internal/task"
)

const maxTUILogLines = 5000

func fetchTasks(mgr *task.Manager) tea.Cmd {
	return func() tea.Msg {
		tasks, err := mgr.ListTasks()
		if err != nil {
			return tasksUpdatedMsg{err: err}
		}
		return tasksUpdatedMsg{tasks: tasks}
	}
}

func fetchLogs(mgr *task.Manager, taskID int, showNoise bool) tea.Cmd {
	return func() tea.Msg {
		result, err := mgr.FollowLogs(taskID, maxTUILogLines)
		if err != nil {
			return logContentMsg{taskID: taskID, err: err}
		}
		var visibleRaw, colored strings.Builder
		lineNumbers := make([]int, 0, len(result.Lines))
		hiddenNoise := 0
		for _, line := range result.Lines {
			rendered, hidden := logcolor.RenderLine(line.Content, logcolor.RenderOptions{ShowNoise: showNoise})
			if hidden {
				hiddenNoise++
				continue
			}
			if colored.Len() > 0 {
				colored.WriteString("\n")
				visibleRaw.WriteString("\n")
			}
			colored.WriteString(rendered)
			visibleRaw.WriteString(line.Content)
			lineNumbers = append(lineNumbers, line.LineNum)
		}
		return logContentMsg{
			taskID:      taskID,
			visibleRaw:  visibleRaw.String(),
			colored:     colored.String(),
			lineNumbers: lineNumbers,
			hiddenNoise: hiddenNoise,
		}
	}
}

func listModels(a *agent.Agent) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		models, err := a.ListModels(ctx)
		return modelsLoadedMsg{models: models, err: err}
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
				return agentErrorMsg{err: ctx.Err()}
			}
			return agentErrorMsg{err: err}
		}
		return agentResponseMsg(resp)
	}
}

func stopTask(mgr *task.Manager, id int) tea.Cmd {
	return func() tea.Msg {
		return taskStoppedMsg{id: id, err: mgr.StopTask(id)}
	}
}

func restartTaskCmd(mgr *task.Manager, id int) tea.Cmd {
	return func() tea.Msg {
		newTaskID, err := mgr.RestartTask(id)
		return taskRestartedMsg{id: newTaskID, err: err}
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
