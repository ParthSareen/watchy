package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/parth/watchy/internal/task"
)

func TestRunningFilterHidesNonRunningTasks(t *testing.T) {
	tasks := []*task.Task{
		{ID: 1, Name: "alpha", Status: "running"},
		{ID: 2, Name: "beta", Status: "stopped"},
		{ID: 3, Name: "gamma", Status: "running"},
		{ID: 4, Name: "delta", Status: "crashed"},
	}
	m := Model{
		width:         80,
		height:        20,
		rightMode:     modeLog,
		focusedArea:   focusTasks,
		chat:          newChatModel(maxChatMessages),
		tasks:         tasks,
		filterRunning: true,
	}
	m.recalcLayout()

	vis := m.visibleTasks()
	if len(vis) != 2 {
		t.Fatalf("visibleTasks() = %d, want 2", len(vis))
	}
	if vis[0].ID != 1 || vis[1].ID != 3 {
		t.Fatalf("visible tasks = %d,%d, want 1,3", vis[0].ID, vis[1].ID)
	}

	view := m.View()
	if strings.Contains(view, "beta") || strings.Contains(view, "delta") {
		t.Fatalf("filtered-out tasks should not appear in view:\n%s", view)
	}
	if !strings.Contains(view, "alpha") || !strings.Contains(view, "gamma") {
		t.Fatalf("running tasks should appear in view:\n%s", view)
	}
	if !strings.Contains(view, "Tasks (running)") {
		t.Fatalf("filter state should be reflected in the pane title")
	}
}

func TestRunningFilterClampsSelection(t *testing.T) {
	tasks := []*task.Task{
		{ID: 1, Name: "alpha", Status: "stopped"},
		{ID: 2, Name: "beta", Status: "stopped"},
		{ID: 3, Name: "gamma", Status: "running"},
	}
	m := Model{
		width:       60,
		height:      20,
		rightMode:   modeLog,
		focusedArea: focusTasks,
		chat:        newChatModel(maxChatMessages),
		tasks:       tasks,
		selectedIdx: 1, // points at a stopped task
	}
	m.recalcLayout()

	m.filterRunning = true
	m.clampSelection()
	vis := m.visibleTasks()
	if m.selectedIdx != 0 {
		t.Fatalf("selectedIdx = %d, want 0 after clamp", m.selectedIdx)
	}
	if vis[m.selectedIdx].ID != 3 {
		t.Fatalf("selected task ID = %d, want 3 (the only running task)", vis[m.selectedIdx].ID)
	}
}

func TestRunningFilterEmptyMessage(t *testing.T) {
	tasks := []*task.Task{
		{ID: 1, Name: "alpha", Status: "stopped"},
	}
	m := Model{
		width:         60,
		height:        20,
		rightMode:     modeLog,
		focusedArea:   focusTasks,
		chat:          newChatModel(maxChatMessages),
		tasks:         tasks,
		filterRunning: true,
	}
	m.recalcLayout()
	view := m.View()
	if !strings.Contains(view, "No running tasks") {
		t.Fatalf("expected empty filter message in view:\n%s", view)
	}
}

func TestViewFitsTerminalWithLongTaskList(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		height int
		mode   mode
	}{
		{name: "logs narrow", width: 72, height: 18, mode: modeLog},
		{name: "logs screenshot size", width: 128, height: 40, mode: modeLog},
		{name: "logs short resize", width: 128, height: 16, mode: modeLog},
		{name: "chat narrow and short", width: 72, height: 10, mode: modeChat},
		{name: "chat wide", width: 160, height: 40, mode: modeChat},
		{name: "split odd width", width: 127, height: 40, mode: modeSplit},
		{name: "split short resize", width: 127, height: 16, mode: modeSplit},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := layoutTestModel(tt.width, tt.height, tt.mode, 35)
			assertViewFitsTerminal(t, m)
		})
	}
}

func TestResizeKeepsSelectedTaskVisible(t *testing.T) {
	m := layoutTestModel(128, 40, modeSplit, 35)
	m.selectedIdx = 30

	for _, size := range []tea.WindowSizeMsg{
		{Width: 128, Height: 16},
		{Width: 72, Height: 10},
		{Width: 160, Height: 40},
		{Width: 151, Height: 31},
	} {
		updated, cmd := m.Update(size)
		if cmd == nil {
			t.Fatalf("resize to %dx%d did not request a full repaint", size.Width, size.Height)
		}
		m = updated.(Model)
		assertViewFitsTerminal(t, m)
		taskRows := ansi.Strip(m.renderTaskList(m.leftWidth-2, m.innerHeight))
		if !strings.Contains(taskRows, " 31 ") {
			t.Fatalf("selected task is not visible after resize to %dx%d", size.Width, size.Height)
		}
	}
}

func TestBusyChatFrameHasSingleComposerAndStableFooter(t *testing.T) {
	for _, tt := range []struct {
		name   string
		width  int
		height int
		mode   mode
	}{
		{name: "narrow chat", width: 72, height: 12, mode: modeChat},
		{name: "wide chat", width: 160, height: 40, mode: modeChat},
		{name: "narrow split", width: 96, height: 12, mode: modeSplit},
		{name: "wide split", width: 160, height: 40, mode: modeSplit},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := layoutTestModel(tt.width, tt.height, tt.mode, 6)
			m.focusedArea = focusChatInput
			m.chat.Focus()
			m.chat.AppendMessage("user", "inspect the active task")
			m.chat.SetBusy(true)
			m.agentBusy = true

			view := ansi.Strip(m.View())
			if got := strings.Count(view, "Ask the agent..."); got != 1 {
				t.Fatalf("composer count = %d, want 1:\n%s", got, view)
			}
			if got := strings.Count(view, "agent working"); got != 2 {
				t.Fatalf("busy indicator count = %d, want chat and footer indicators:\n%s", got, view)
			}

			lines := strings.Split(view, "\n")
			footer := ansi.Strip(m.renderStatusBar())
			if got := strings.Count(view, footer); got != 1 {
				t.Fatalf("footer count = %d, want 1:\n%s", got, view)
			}
			if lines[len(lines)-1] != footer {
				t.Fatalf("last rendered row = %q, want footer %q", lines[len(lines)-1], footer)
			}
			assertViewFitsTerminal(t, m)
		})
	}
}

func TestTaskRowsRenderOnceWithMixedStates(t *testing.T) {
	tasks := []*task.Task{
		{ID: 41, Name: "active", Status: "running"},
		{ID: 40, Name: "finished", Status: "stopped"},
		{ID: 39, Name: "failed", Status: "crashed"},
		{ID: 38, Name: "a-task-name-that-is-long-enough-to-truncate", Status: "stopped"},
	}
	m := Model{
		width:       96,
		height:      16,
		rightMode:   modeChat,
		focusedArea: focusTasks,
		chat:        newChatModel(maxChatMessages),
		tasks:       tasks,
	}
	m.recalcLayout()

	rows := strings.Split(ansi.Strip(m.renderTaskList(m.leftWidth-2, m.innerHeight)), "\n")
	if len(rows) != len(tasks) {
		t.Fatalf("task row count = %d, want %d:\n%s", len(rows), len(tasks), strings.Join(rows, "\n"))
	}
	for _, id := range []string{"41", "40", "39", "38"} {
		if got := strings.Count(strings.Join(rows, "\n"), id); got != 1 {
			t.Fatalf("task %s count = %d, want 1:\n%s", id, got, strings.Join(rows, "\n"))
		}
	}
}

func TestTaskActionFeedback(t *testing.T) {
	tasks := []*task.Task{
		{ID: 1, Name: "alpha", Status: "stopped"},
		{ID: 2, Name: "beta", Status: "running"},
	}
	newModel := func(focus focusTarget, idx int) Model {
		m := Model{
			width:       60,
			height:      20,
			rightMode:   modeLog,
			focusedArea: focus,
			chat:        newChatModel(maxChatMessages),
			tasks:       tasks,
			selectedIdx: idx,
		}
		m.recalcLayout()
		return m
	}

	// x on a non-running task reports it isn't running.
	m := newModel(focusTasks, 0)
	updated, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	mod := updated.(Model)
	if !mod.statusError || !strings.Contains(mod.statusMessage, "isn't running") {
		t.Fatalf("x on stopped task: status=%q err=%v", mod.statusMessage, mod.statusError)
	}

	// x outside the task list tells the user to focus it.
	m = newModel(focusLogs, 0)
	updated, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	mod = updated.(Model)
	if !mod.statusError || !strings.Contains(mod.statusMessage, "focus the task list") {
		t.Fatalf("x off task list: status=%q err=%v", mod.statusMessage, mod.statusError)
	}

	// r on a running task reports it is running.
	m = newModel(focusTasks, 1)
	updated, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	mod = updated.(Model)
	if !mod.statusError || !strings.Contains(mod.statusMessage, "is running") {
		t.Fatalf("r on running task: status=%q err=%v", mod.statusMessage, mod.statusError)
	}
}

func layoutTestModel(width, height int, rightMode mode, taskCount int) Model {
	tasks := make([]*task.Task, taskCount)
	for i := range tasks {
		tasks[i] = &task.Task{ID: i + 1, Name: fmt.Sprintf("task-%02d", i), Status: "stopped"}
	}
	if len(tasks) > 0 {
		tasks[0].Name = "selected-log-task-with-long-name"
	}

	m := Model{
		width:       width,
		height:      height,
		rightMode:   rightMode,
		focusedArea: focusLogs,
		chat:        newChatModel(maxChatMessages),
		tasks:       tasks,
	}
	m.recalcLayout()
	logLines := make([]string, 11)
	m.logs.logLineNumbers = make([]int, len(logLines))
	for i := range logLines {
		logLines[i] = fmt.Sprintf("log line %d", i+1)
		m.logs.logLineNumbers[i] = i + 1
	}
	m.logs.originalLogContent = strings.Join(logLines, "\n")
	m.logs.rawLogContent = m.logs.originalLogContent
	m.logs.logViewport.SetContent(m.logs.addLineNumbers(m.logs.originalLogContent))
	return m
}

func assertViewFitsTerminal(t *testing.T, m Model) {
	t.Helper()
	view := m.View()
	wantHeight := maxInt(1, m.height-1)
	if got := lipgloss.Height(view); got != wantHeight {
		t.Fatalf("view height = %d, want %d with bottom guard row:\n%s", got, wantHeight, ansi.Strip(view))
	}

	lines := strings.Split(view, "\n")
	for row := 0; row < m.boxHeight; row++ {
		if got := lipgloss.Width(lines[row]); got != m.width {
			t.Fatalf("row %d width = %d, want terminal width %d", row, got, m.width)
		}
	}
}
