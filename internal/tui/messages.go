package tui

import (
	"time"

	"github.com/parth/watchy/internal/agent"
	"github.com/parth/watchy/internal/task"
)

type tasksUpdatedMsg []*task.Task
type logContentMsg string
type agentResponseMsg string
type agentErrorMsg struct{ err error }
type agentToolStartMsg agent.ToolStartEvent
type agentToolResultMsg agent.ToolResultEvent
type taskStoppedMsg int
type taskRestartedMsg int64
type selectTaskMsg int
type tickMsg time.Time
