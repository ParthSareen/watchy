package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

const mouseWheelStep = 3

type mouseRegion int

const (
	mouseRegionNone mouseRegion = iota
	mouseRegionTasks
	mouseRegionLogs
	mouseRegionChat
)

type paneRect struct {
	x      int
	y      int
	width  int
	height int
}

func (m Model) handleMouse(msg tea.MouseMsg) (Model, tea.Cmd) {
	if m.width == 0 || m.height == 0 {
		return m, nil
	}
	// Ignore mouse events while a full-screen overlay is open so clicks don't
	// select tasks or switch panes behind it.
	if m.showHelp || m.modelPicker || m.showTaskDetails {
		return m, nil
	}

	switch msg.Button {
	case tea.MouseButtonWheelUp:
		return m.handleMouseWheel(msg, -mouseWheelStep)
	case tea.MouseButtonWheelDown:
		return m.handleMouseWheel(msg, mouseWheelStep)
	case tea.MouseButtonWheelLeft:
		return m.handleMouseHorizontalWheel(msg, -mouseWheelStep*4)
	case tea.MouseButtonWheelRight:
		return m.handleMouseHorizontalWheel(msg, mouseWheelStep*4)
	}

	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress {
		return m, nil
	}

	switch m.mouseRegionAt(msg.X, msg.Y) {
	case mouseRegionTasks:
		return m.handleTaskClick(msg.Y)
	case mouseRegionLogs:
		cmd := m.setFocus(focusLogs)
		m.moveLogCursorToMouse(msg.Y)
		return m, cmd
	case mouseRegionChat:
		if m.mouseInChatInput(msg.Y) {
			return m, m.setFocus(focusChatInput)
		}
		return m, m.setFocus(focusChatView)
	default:
		return m, nil
	}
}

func (m Model) handleMouseWheel(msg tea.MouseMsg, delta int) (Model, tea.Cmd) {
	switch m.mouseRegionAt(msg.X, msg.Y) {
	case mouseRegionTasks:
		return m.scrollTasks(delta)
	case mouseRegionLogs:
		m.scrollLogs(delta)
	case mouseRegionChat:
		if delta > 0 {
			m.chat.ScrollDown(delta)
		} else {
			m.chat.ScrollUp(-delta)
		}
	}
	return m, nil
}

func (m Model) handleMouseHorizontalWheel(msg tea.MouseMsg, delta int) (Model, tea.Cmd) {
	if m.mouseRegionAt(msg.X, msg.Y) != mouseRegionLogs || m.logs.rawLogContent == "" {
		return m, nil
	}
	m.setFocus(focusLogs)
	m.logs.logXOffset += delta
	if m.logs.logXOffset < 0 {
		m.logs.logXOffset = 0
	}
	maxOffset := m.maxLogXOffset()
	if m.logs.logXOffset > maxOffset {
		m.logs.logXOffset = maxOffset
	}
	m.logs.refreshLogContent()
	return m, nil
}

func (m Model) handleTaskClick(y int) (Model, tea.Cmd) {
	cmd := m.setFocus(focusTasks)
	rect, ok := m.paneRect(paneLeft)
	if !ok {
		return m, cmd
	}
	start, end := m.taskWindow(m.innerHeight)
	idx := start + y - rect.contentY()
	if idx < start || idx >= end {
		return m, cmd
	}

	m.selectedIdx = idx
	m.logs.cursorLine = 0
	m.logs.visualMode = false
	if m.rightMode == modeChat {
		m.rightMode = modeLog
		m.recalcLayout()
		m.setFocus(focusTasks)
	}
	if m.rightMode == modeLog || m.rightMode == modeSplit {
		vis := m.visibleTasks()
		if len(vis) > 0 && m.selectedIdx < len(vis) {
			return m, tea.Batch(cmd, fetchLogs(m.mgr, vis[m.selectedIdx].ID, m.logs.showLogNoise))
		}
	}
	return m, cmd
}

func (m Model) scrollTasks(delta int) (Model, tea.Cmd) {
	vis := m.visibleTasks()
	if len(vis) == 0 {
		return m, nil
	}
	m.setFocus(focusTasks)

	m.selectedIdx += delta
	if m.selectedIdx < 0 {
		m.selectedIdx = 0
	}
	if m.selectedIdx >= len(vis) {
		m.selectedIdx = len(vis) - 1
	}

	if m.rightMode == modeLog || m.rightMode == modeSplit {
		return m, fetchLogs(m.mgr, vis[m.selectedIdx].ID, m.logs.showLogNoise)
	}
	return m, nil
}

func (m *Model) scrollLogs(delta int) {
	if m.logs.rawLogContent == "" {
		return
	}
	m.setFocus(focusLogs)
	totalLines := m.logs.totalLogLines()
	if totalLines <= 0 {
		return
	}

	m.logs.cursorLine += delta
	if m.logs.cursorLine < 0 {
		m.logs.cursorLine = 0
	}
	if m.logs.cursorLine >= totalLines {
		m.logs.cursorLine = totalLines - 1
	}
	m.logs.refreshLogContent()

	if m.logs.cursorLine < m.logs.logViewport.YOffset {
		m.logs.logViewport.SetYOffset(m.logs.cursorLine)
	} else if m.logs.cursorLine >= m.logs.logViewport.YOffset+m.logs.logViewport.Height {
		m.logs.logViewport.SetYOffset(m.logs.cursorLine - m.logs.logViewport.Height + 1)
	}
}

func (m *Model) moveLogCursorToMouse(y int) {
	if m.logs.rawLogContent == "" {
		return
	}
	rect, ok := m.paneRect(paneRight)
	if !ok {
		return
	}
	line := m.logs.logViewport.YOffset + y - rect.contentY()
	totalLines := m.logs.totalLogLines()
	if totalLines <= 0 {
		return
	}
	if line < 0 {
		line = 0
	}
	if line >= totalLines {
		line = totalLines - 1
	}
	m.logs.cursorLine = line
	m.logs.refreshLogContent()
}

func (m Model) mouseRegionAt(x, y int) mouseRegion {
	if y < 0 || x < 0 {
		return mouseRegionNone
	}
	if rect, ok := m.paneRect(paneLeft); ok && rect.contains(x, y) {
		return mouseRegionTasks
	}
	if rect, ok := m.paneRect(paneRight); ok && rect.contains(x, y) {
		return mouseRegionLogs
	}
	if rect, ok := m.paneRect(paneChat); ok && rect.contains(x, y) {
		return mouseRegionChat
	}
	return mouseRegionNone
}

func (m Model) mouseInChatInput(y int) bool {
	rect, ok := m.paneRect(paneChat)
	if !ok {
		return false
	}
	return y >= rect.chatInputY() && y < rect.bottomBorderY()
}

func (m Model) maxLogXOffset() int {
	maxLen := m.logs.maxLineLength()
	lineNumWidth := lenInt(m.logs.totalLogLines())
	contentWidth := m.logs.logViewport.Width - lineNumWidth - 3
	if contentWidth < 1 {
		contentWidth = 1
	}
	maxOffset := maxLen - contentWidth
	if maxOffset < 0 {
		return 0
	}
	return maxOffset
}

func (m Model) paneRect(p pane) (paneRect, bool) {
	if m.boxHeight <= 0 {
		return paneRect{}, false
	}

	leftRenderedWidth := 0
	if !m.leftHidden {
		leftRenderedWidth = renderedPaneWidth(m.leftWidth)
	}
	rightStart := leftRenderedWidth

	switch p {
	case paneLeft:
		if m.leftHidden {
			return paneRect{}, false
		}
		return paneRect{x: 0, y: 0, width: renderedPaneWidth(m.leftWidth), height: m.boxHeight}, true
	case paneRight:
		if m.rightMode == modeChat {
			return paneRect{}, false
		}
		if m.rightMode == modeSplit {
			logWidth, _ := m.splitPaneWidths()
			return paneRect{x: rightStart, y: 0, width: renderedPaneWidth(logWidth), height: m.boxHeight}, true
		}
		return paneRect{x: rightStart, y: 0, width: renderedPaneWidth(m.rightWidth), height: m.boxHeight}, true
	case paneChat:
		if m.rightMode == modeLog {
			return paneRect{}, false
		}
		if m.rightMode == modeSplit {
			logWidth, chatWidth := m.splitPaneWidths()
			logRenderedWidth := renderedPaneWidth(logWidth)
			return paneRect{x: rightStart + logRenderedWidth, y: 0, width: renderedPaneWidth(chatWidth), height: m.boxHeight}, true
		}
		return paneRect{x: rightStart, y: 0, width: renderedPaneWidth(m.rightWidth), height: m.boxHeight}, true
	default:
		return paneRect{}, false
	}
}

func renderedPaneWidth(contentWidth int) int {
	if contentWidth < 0 {
		contentWidth = 0
	}
	return contentWidth + 2
}

func (r paneRect) contains(x, y int) bool {
	return x >= r.x && x < r.x+r.width && y >= r.y && y < r.y+r.height
}

func (r paneRect) contentY() int {
	return r.y + 2
}

func (r paneRect) bottomBorderY() int {
	return r.y + r.height - 1
}

func (r paneRect) chatInputY() int {
	// Chat view is viewport, input (3 rows), help (1 row), and separators.
	// Inside the border, that places the textarea five rows above bottom.
	return r.bottomBorderY() - 5
}
