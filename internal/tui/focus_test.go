package tui

import (
	"testing"
)

func TestCycleFocusOnlyMovesThroughVisibleLogPanes(t *testing.T) {
	m := Model{
		rightMode:   modeLog,
		focusedArea: focusTasks,
		chat:        newChatModel(maxChatMessages),
	}

	m.cycleFocus(true)
	if m.focusedArea != focusLogs {
		t.Fatalf("focusedArea = %v, want focusLogs", m.focusedArea)
	}
	if m.rightMode != modeLog {
		t.Fatalf("rightMode = %v, want modeLog", m.rightMode)
	}

	m.cycleFocus(true)
	if m.focusedArea != focusTasks {
		t.Fatalf("focusedArea = %v, want focusTasks", m.focusedArea)
	}
	if m.rightMode != modeLog {
		t.Fatalf("rightMode = %v, want modeLog", m.rightMode)
	}
	if m.chat.Focused() {
		t.Fatal("chat input should not be focused by tab")
	}
}

func TestCycleFocusMovesThroughVisibleSplitPanes(t *testing.T) {
	m := Model{
		rightMode:   modeSplit,
		focusedArea: focusTasks,
		chat:        newChatModel(maxChatMessages),
	}

	m.cycleFocus(true)
	if m.focusedArea != focusLogs {
		t.Fatalf("focusedArea = %v, want focusLogs", m.focusedArea)
	}
	m.cycleFocus(true)
	if m.focusedArea != focusChatView {
		t.Fatalf("focusedArea = %v, want focusChatView", m.focusedArea)
	}
	m.cycleFocus(true)
	if m.focusedArea != focusTasks {
		t.Fatalf("focusedArea = %v, want focusTasks", m.focusedArea)
	}
}

func TestCycleFocusSkipsHiddenSidebar(t *testing.T) {
	m := Model{
		leftHidden:  true,
		rightMode:   modeChat,
		focusedArea: focusChatInput,
		chat:        newChatModel(maxChatMessages),
	}
	m.chat.Focus()

	m.cycleFocus(true)
	if m.focusedArea != focusChatView {
		t.Fatalf("focusedArea = %v, want focusChatView", m.focusedArea)
	}
	if m.rightMode != modeChat {
		t.Fatalf("rightMode = %v, want modeChat", m.rightMode)
	}

	m.cycleFocus(false)
	if m.focusedArea != focusChatView {
		t.Fatalf("focusedArea = %v, want focusChatView", m.focusedArea)
	}
}

func TestPaneActiveTracksSplitLogAndChatFocus(t *testing.T) {
	m := Model{rightMode: modeSplit, focusedArea: focusLogs}
	if !m.paneIsActive(paneRight) {
		t.Fatal("logs pane should be active")
	}
	if m.paneIsActive(paneChat) {
		t.Fatal("chat pane should not be active")
	}

	m.focusedArea = focusChatView
	if !m.paneIsActive(paneChat) {
		t.Fatal("chat pane should be active")
	}
	if m.paneIsActive(paneRight) {
		t.Fatal("logs pane should not be active")
	}
}
