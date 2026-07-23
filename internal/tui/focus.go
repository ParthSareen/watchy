package tui

import tea "github.com/charmbracelet/bubbletea"

func (m *Model) setFocus(target focusTarget) tea.Cmd {
	if target == focusTasks && m.leftHidden {
		target = m.rightFocusTarget()
	}
	if target == focusLogs && m.rightMode == modeChat {
		target = focusChatView
	}
	if (target == focusChatView || target == focusChatInput) && m.rightMode == modeLog {
		target = focusLogs
	}

	m.focusedArea = target
	switch target {
	case focusTasks:
		m.chat.Blur()
	case focusLogs:
		m.chat.Blur()
	case focusChatView:
		m.chat.Blur()
	case focusChatInput:
		return m.chat.Focus()
	}
	return nil
}

func (m Model) rightFocusTarget() focusTarget {
	if m.rightMode == modeChat {
		return focusChatView
	}
	return focusLogs
}

func (m Model) paneIsActive(p pane) bool {
	switch p {
	case paneLeft:
		return !m.leftHidden && m.focusedArea == focusTasks
	case paneRight:
		return m.focusedArea == focusLogs
	case paneChat:
		return m.focusedArea == focusChatView || m.focusedArea == focusChatInput
	default:
		return false
	}
}

func (m Model) logsFocused() bool {
	return m.focusedArea == focusLogs && (m.rightMode == modeLog || m.rightMode == modeSplit)
}

func (m Model) chatViewFocused() bool {
	return m.focusedArea == focusChatView && (m.rightMode == modeChat || m.rightMode == modeSplit)
}

func (m Model) focusCycleTargets() []focusTarget {
	targets := make([]focusTarget, 0, 3)
	if !m.leftHidden {
		targets = append(targets, focusTasks)
	}
	switch m.rightMode {
	case modeLog:
		targets = append(targets, focusLogs)
	case modeChat:
		targets = append(targets, focusChatView)
	case modeSplit:
		targets = append(targets, focusLogs, focusChatView)
	}
	return targets
}

func (m *Model) cycleFocus(forward bool) tea.Cmd {
	targets := m.focusCycleTargets()
	if len(targets) == 0 {
		return nil
	}

	current := m.focusedArea
	if current == focusChatInput {
		current = focusChatView
	}

	idx := -1
	for i, target := range targets {
		if target == current {
			idx = i
			break
		}
	}
	if idx < 0 {
		idx = 0
	} else if forward {
		idx = (idx + 1) % len(targets)
	} else {
		idx--
		if idx < 0 {
			idx = len(targets) - 1
		}
	}

	return m.setFocus(targets[idx])
}

func (m *Model) showRightMode(next mode) tea.Cmd {
	m.rightMode = next
	switch next {
	case modeLog:
		m.setFocus(focusLogs)
	case modeChat:
		m.setFocus(focusChatView)
	case modeSplit:
		if m.focusedArea == focusTasks {
			m.setFocus(focusLogs)
		}
	}
	m.recalcLayout()
	if (next == modeLog || next == modeSplit) && len(m.tasks) > 0 && m.selectedIdx < len(m.tasks) {
		m.logsLoading = m.originalLogContent == ""
		return fetchLogs(m.mgr, m.tasks[m.selectedIdx].ID, m.showLogNoise)
	}
	return nil
}

func (m *Model) cycleRightMode(forward bool) tea.Cmd {
	next := int(m.rightMode)
	if forward {
		next = (next + 1) % 3
	} else {
		next--
		if next < 0 {
			next = 2
		}
	}
	return m.showRightMode(mode(next))
}
