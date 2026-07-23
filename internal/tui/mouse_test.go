package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/parth/watchy/internal/task"
)

func testMouseModel() Model {
	m := Model{
		width:       100,
		height:      30,
		rightMode:   modeLog,
		focusedArea: focusTasks,
		chat:        newChatModel(maxChatMessages),
		tasks: []*task.Task{
			{ID: 1, Name: "serve", Status: "running"},
			{ID: 2, Name: "tests", Status: "stopped"},
			{ID: 3, Name: "dev", Status: "running"},
		},
	}
	m.recalcLayout()
	return m
}

func TestMouseClickTaskSelectsAndFocusesTaskList(t *testing.T) {
	m := testMouseModel()
	m.rightMode = modeChat

	updated, _ := m.handleMouse(tea.MouseMsg{
		X:      2,
		Y:      3,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})

	if updated.selectedIdx != 1 {
		t.Fatalf("selectedIdx = %d, want 1", updated.selectedIdx)
	}
	if updated.focusedArea != focusTasks {
		t.Fatalf("focusedArea = %v, want focusTasks", updated.focusedArea)
	}
	if updated.rightMode != modeLog {
		t.Fatalf("rightMode = %v, want modeLog", updated.rightMode)
	}
}

func TestMouseClickChatComposerFocusesInput(t *testing.T) {
	m := testMouseModel()
	m.rightMode = modeChat
	m.recalcLayout()

	updated, _ := m.handleMouse(tea.MouseMsg{
		X:      m.leftWidth + 2,
		Y:      m.boxHeight - 3,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})

	if updated.focusedArea != focusChatInput {
		t.Fatalf("focusedArea = %v, want focusChatInput", updated.focusedArea)
	}
	if !updated.chat.Focused() {
		t.Fatal("chat input should be focused")
	}
}

func TestMouseWheelOverLogsMovesCursor(t *testing.T) {
	m := testMouseModel()
	m.rawLogContent = "one\ntwo\nthree\nfour\nfive"
	m.originalLogContent = m.rawLogContent
	m.logLineNumbers = []int{1, 2, 3, 4, 5}
	m.logViewport.SetContent(m.addLineNumbers(m.originalLogContent))

	updated, _ := m.handleMouse(tea.MouseMsg{
		X:      m.leftWidth + 2,
		Y:      3,
		Button: tea.MouseButtonWheelDown,
		Action: tea.MouseActionPress,
	})

	if updated.focusedArea != focusLogs {
		t.Fatalf("focusedArea = %v, want focusLogs", updated.focusedArea)
	}
	if updated.cursorLine != mouseWheelStep {
		t.Fatalf("cursorLine = %d, want %d", updated.cursorLine, mouseWheelStep)
	}
}

func TestMousePaneRectsMatchRenderedBoundaries(t *testing.T) {
	m := testMouseModel()
	view := m.View()
	firstLine := strings.Split(view, "\n")[0]

	leftRect, ok := m.paneRect(paneLeft)
	if !ok {
		t.Fatal("expected left pane rect")
	}
	rightRect, ok := m.paneRect(paneRight)
	if !ok {
		t.Fatal("expected right pane rect")
	}

	leftPane := m.applyBorder(paneLeft, m.leftWidth, m.boxHeight, "Tasks", "")
	if got := lipgloss.Width(leftPane); got != leftRect.width {
		t.Fatalf("left rendered width = %d, rect width = %d", got, leftRect.width)
	}
	if got := lipgloss.Width(firstLine); got != leftRect.width+rightRect.width {
		t.Fatalf("first rendered line width = %d, rect total = %d", got, leftRect.width+rightRect.width)
	}
	if m.mouseRegionAt(rightRect.x-1, 1) != mouseRegionTasks {
		t.Fatal("cell before right pane should be task pane")
	}
	if m.mouseRegionAt(rightRect.x, 1) != mouseRegionLogs {
		t.Fatal("right pane start should be logs")
	}
}

func TestMousePaneRectsMatchSplitBoundaries(t *testing.T) {
	m := testMouseModel()
	m.rightMode = modeSplit
	m.recalcLayout()

	logRect, ok := m.paneRect(paneRight)
	if !ok {
		t.Fatal("expected log pane rect")
	}
	chatRect, ok := m.paneRect(paneChat)
	if !ok {
		t.Fatal("expected chat pane rect")
	}

	if m.mouseRegionAt(logRect.x+logRect.width-1, 1) != mouseRegionLogs {
		t.Fatal("last log cell should be logs")
	}
	if m.mouseRegionAt(chatRect.x, 1) != mouseRegionChat {
		t.Fatal("chat pane start should be chat")
	}
}
