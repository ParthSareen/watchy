package tui

import "github.com/parth/watchy/internal/task"

func (m *Model) recalcLayout() {
	// Leave the terminal's last row unused. Some terminal emulators scroll when
	// a full-width footer reaches the bottom-right cell, which leaves fragments
	// of earlier frames behind during spinner updates and resizes.
	m.boxHeight = m.height - 2
	if m.boxHeight < 0 {
		m.boxHeight = 0
	}
	m.innerHeight = m.boxHeight - 3
	if m.innerHeight < 0 {
		m.innerHeight = 0
	}

	if m.leftHidden {
		m.leftWidth = 0
		m.rightWidth = m.width - 2
	} else {
		m.leftWidth = m.width * 30 / 100
		m.rightWidth = m.width - m.leftWidth - 4
		if m.rightWidth < 1 {
			m.rightWidth = 1
		}
	}

	if m.rightMode == modeSplit {
		logWidth, chatWidth := m.splitPaneWidths()
		m.logs.logViewport.Width = logWidth
		m.logs.logViewport.Height = m.innerHeight
		m.chat.SetSize(chatWidth, m.innerHeight)
		return
	}
	m.logs.logViewport.Width = m.rightWidth
	m.logs.logViewport.Height = m.innerHeight
	m.chat.SetSize(m.rightWidth, m.innerHeight)
}

func (m Model) splitPaneWidths() (int, int) {
	available := m.rightWidth - 2 // each pane adds its own two border cells
	if available < 0 {
		available = 0
	}
	logWidth := available / 2
	return logWidth, available - logWidth
}

// visibleTasks returns the tasks currently shown in the sidebar. When the
// running-only filter is active, non-running tasks are excluded.
func (m Model) visibleTasks() []*task.Task {
	if !m.filterRunning {
		return m.tasks
	}
	out := make([]*task.Task, 0, len(m.tasks))
	for _, t := range m.tasks {
		if t.Status == "running" {
			out = append(out, t)
		}
	}
	return out
}

// clampSelection keeps selectedIdx within the bounds of the visible task list.
func (m *Model) clampSelection() {
	vis := m.visibleTasks()
	if len(vis) == 0 {
		m.selectedIdx = 0
		return
	}
	if m.selectedIdx < 0 {
		m.selectedIdx = 0
	}
	if m.selectedIdx >= len(vis) {
		m.selectedIdx = len(vis) - 1
	}
}

func (m Model) taskWindow(height int) (int, int) {
	vis := m.visibleTasks()
	if height <= 0 || len(vis) == 0 {
		return 0, 0
	}

	selected := m.selectedIdx
	if selected < 0 {
		selected = 0
	}
	if selected >= len(vis) {
		selected = len(vis) - 1
	}

	start := 0
	if selected >= height {
		start = selected - height + 1
	}
	end := start + height
	if end > len(vis) {
		end = len(vis)
		start = end - height
		if start < 0 {
			start = 0
		}
	}
	return start, end
}

func (m *Model) toggleTaskSidebar() {
	m.leftHidden = !m.leftHidden
	if m.leftHidden && m.focusedArea == focusTasks {
		m.setFocus(m.rightFocusTarget())
	}
	m.recalcLayout()
}
