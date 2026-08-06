package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/parth/watchy/internal/task"
)

func TestViewFitsTerminalWithLongTaskList(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		height int
		mode   mode
	}{
		{name: "logs screenshot size", width: 128, height: 40, mode: modeLog},
		{name: "logs short resize", width: 128, height: 16, mode: modeLog},
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
		{Width: 151, Height: 31},
	} {
		updated, _ := m.Update(size)
		m = updated.(Model)
		assertViewFitsTerminal(t, m)
		if !strings.Contains(m.View(), "task-30") {
			t.Fatalf("selected task is not visible after resize to %dx%d", size.Width, size.Height)
		}
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
	m.logLineNumbers = make([]int, len(logLines))
	for i := range logLines {
		logLines[i] = fmt.Sprintf("log line %d", i+1)
		m.logLineNumbers[i] = i + 1
	}
	m.originalLogContent = strings.Join(logLines, "\n")
	m.rawLogContent = m.originalLogContent
	m.logViewport.SetContent(m.addLineNumbers(m.originalLogContent))
	return m
}

func assertViewFitsTerminal(t *testing.T, m Model) {
	t.Helper()
	view := m.View()
	if got := lipgloss.Height(view); got != m.height {
		t.Fatalf("view height = %d, want terminal height %d", got, m.height)
	}

	lines := strings.Split(view, "\n")
	for row := 0; row < m.boxHeight; row++ {
		if got := lipgloss.Width(lines[row]); got != m.width {
			t.Fatalf("row %d width = %d, want terminal width %d", row, got, m.width)
		}
	}
}
