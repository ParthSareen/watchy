package tui

func (m *Model) recalcLayout() {
	m.boxHeight = m.height - 1
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
		m.logViewport.Width = logWidth
		m.logViewport.Height = m.innerHeight
		m.chat.SetSize(chatWidth, m.innerHeight)
		return
	}
	m.logViewport.Width = m.rightWidth
	m.logViewport.Height = m.innerHeight
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

func (m Model) taskWindow(height int) (int, int) {
	if height <= 0 || len(m.tasks) == 0 {
		return 0, 0
	}

	selected := m.selectedIdx
	if selected < 0 {
		selected = 0
	}
	if selected >= len(m.tasks) {
		selected = len(m.tasks) - 1
	}

	start := 0
	if selected >= height {
		start = selected - height + 1
	}
	end := start + height
	if end > len(m.tasks) {
		end = len(m.tasks)
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
