package tui

import (
	"time"

	"github.com/parth/watchy/internal/agent"
	"github.com/parth/watchy/internal/task"
)

type tasksUpdatedMsg struct {
	tasks []*task.Task
	err   error
}
type logContentMsg struct {
	taskID      int
	visibleRaw  string // raw content for visible display lines only
	colored     string // colorized display content
	lineNumbers []int  // original line number for each line in content
	hiddenNoise int    // number of routine noise lines hidden from display
	err         error
}
type clipboardCopiedMsg struct{}
type clearCopiedMsg struct{}
type clearStatusMsg int
type agentResponseMsg string
type agentErrorMsg struct{ err error }
type agentToolStartMsg agent.ToolStartEvent
type agentToolResultMsg agent.ToolResultEvent
type taskStoppedMsg struct {
	id  int
	err error
}
type taskRestartedMsg struct {
	id  int64
	err error
}
type modelsLoadedMsg struct {
	models []string
	err    error
}
type tickMsg time.Time
