package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}
	if m.modelPicker {
		return m.renderModelPicker()
	}
	if m.showHelp {
		return m.renderHelp()
	}
	if m.showTaskDetails {
		return m.renderTaskDetails()
	}

	var leftPane string
	if !m.leftHidden {
		taskTitle := "Tasks"
		if m.filterRunning {
			taskTitle = "Tasks (running)"
		}
		leftContent := m.renderTaskList(m.leftWidth-2, m.innerHeight)
		leftPane = m.applyBorder(paneLeft, m.leftWidth, m.boxHeight, taskTitle, leftContent)
	}

	// Right pane: logs, chat, or split
	var rightPane string
	if m.rightMode == modeLog {
		rightTitle := m.logTitle(true)
		rightContent := m.logContentView()
		if m.logs.searchMode {
			rightContent += "\n" + m.logs.searchInput.View()
		}
		if m.logs.commandMode {
			rightContent += "\n" + m.logs.commandInput.View()
		}
		rightPane = m.applyBorder(paneRight, m.rightWidth, m.boxHeight, rightTitle, rightContent)
	} else if m.rightMode == modeChat {
		rightTitle := m.viewTabs()
		rightContent := m.chat.View(m.focusedArea == focusChatInput)
		rightPane = m.applyBorder(paneChat, m.rightWidth, m.boxHeight, rightTitle, rightContent)
	} else {
		// Split mode: chat and logs side by side
		logWidth, chatWidth := m.splitPaneWidths()

		// Logs pane (left half of right section)
		logsTitle := m.logTitle(false)
		logsContent := m.logContentView()
		if m.logs.searchMode {
			logsContent += "\n" + m.logs.searchInput.View()
		}
		logsPane := m.applyBorder(paneRight, logWidth, m.boxHeight, logsTitle, logsContent)

		// Chat pane (right half of right section)
		chatTitle := "Chat"
		chatContent := m.chat.View(m.focusedArea == focusChatInput)
		chatPane := m.applyBorder(paneChat, chatWidth, m.boxHeight, chatTitle, chatContent)

		rightPane = lipgloss.JoinHorizontal(lipgloss.Top, logsPane, chatPane)
	}

	var main string
	if m.leftHidden {
		main = rightPane
	} else {
		main = lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)
	}

	// Status bar
	statusBar := m.renderStatusBar()

	return main + "\n" + statusBar
}

func (m Model) applyBorder(p pane, width, height int, title, content string) string {
	borderColor := m.dimGrayForMode()
	bright := m.bright()
	if m.paneIsActive(p) {
		borderColor = bright
	}
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(width)

	contentHeight := height - 2
	if contentHeight > 0 {
		style = style.Height(contentHeight)
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(bright).MaxWidth(width)
	return style.Render(titleStyle.Render(title) + "\n" + content)
}

func (m Model) renderTaskList(width, height int) string {
	bright := m.bright()
	bgColor := m.bg()
	dimGrayColor := m.dimGrayForMode()
	errColor := m.errorColorForMode()

	dimStyle := lipgloss.NewStyle().Foreground(dimGrayColor)

	if height <= 0 {
		return ""
	}
	vis := m.visibleTasks()
	if len(vis) == 0 {
		if m.filterRunning {
			return dimStyle.Render("No running tasks. (f to clear filter)")
		}
		return dimStyle.Render("No tasks. Use chat to start one.")
	}

	start, end := m.taskWindow(height)
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		task := vis[i]
		var indicator string
		switch task.Status {
		case "running":
			indicator = lipgloss.NewStyle().Foreground(bright).Render("running")
		case "crashed":
			indicator = lipgloss.NewStyle().Foreground(errColor).Render("crashed")
		default:
			indicator = dimStyle.Render(task.Status)
		}

		name := task.Name
		maxName := width - 15
		if maxName < 0 {
			maxName = 0
		}
		name = truncateRunes(name, maxName)

		line := fmt.Sprintf(" %-7s %-3d %s", indicator, task.ID, name)

		if i == m.selectedIdx {
			selectedStyle := lipgloss.NewStyle().Background(bgColor).Bold(true).Foreground(bright)
			line = selectedStyle.Render(line)
		}

		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

func (m Model) renderStatusBar() string {
	bright := m.bright()
	dimGrayColor := m.dimGrayForMode()
	left := m.contextualHelp()
	leftStyle := lipgloss.NewStyle().Foreground(dimGrayColor)
	if m.statusMessage != "" {
		left = m.statusMessage
		leftStyle = lipgloss.NewStyle().Foreground(bright)
		if m.statusError {
			leftStyle = lipgloss.NewStyle().Foreground(m.errorColorForMode())
		}
	}
	if m.pendingCount > 0 {
		left = fmt.Sprintf("[%d] %s", m.pendingCount, left)
	}
	if m.copied {
		left = "copied • " + left
	}
	if m.logs.visualMode {
		left = "visual • " + left
	}
	if m.logs.commandMode {
		left = "command • " + left
	}
	if m.agentBusy {
		left = "agent working, esc cancels • " + left
	}

	right := m.modelStatus()
	available := m.width - lipgloss.Width(right) - 3
	if available < 8 {
		return leftStyle.Render(truncateRunes(left, m.width))
	}
	left = truncateRunes(left, available)
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return leftStyle.Render(left) + strings.Repeat(" ", gap) + lipgloss.NewStyle().Foreground(bright).Render(right)
}

func (m Model) contextualHelp() string {
	switch m.focusedArea {
	case focusTasks:
		return "j/k move • enter logs • d details • x stop • r restart • f filter • tab focus • ? help"
	case focusLogs:
		return "j/k move • enter sidebar • / search • v select • y copy • l/c/s view • tab focus • ? help"
	case focusChatView:
		return "j/k scroll • i compose • e expand • l/c/s view • tab focus • ? help"
	case focusChatInput:
		return "enter send • ctrl+j newline • esc blur/cancel • ? help"
	default:
		return "? help • q quit"
	}
}

// logTitle builds the log-pane title: view tabs, the selected task, the line
// range, the current search match, and the noise label. includeStatus adds
// the task status suffix (shown in the full-width logs view, omitted in split).
func (m Model) logTitle(includeStatus bool) string {
	title := m.viewTabs()
	vis := m.visibleTasks()
	if len(vis) > 0 && m.selectedIdx < len(vis) {
		if includeStatus {
			title += fmt.Sprintf(" • %d %s • %s", vis[m.selectedIdx].ID, vis[m.selectedIdx].Name, vis[m.selectedIdx].Status)
		} else {
			title += fmt.Sprintf(" • %d %s", vis[m.selectedIdx].ID, vis[m.selectedIdx].Name)
		}
	}
	if len(m.logs.logLineNumbers) > 0 && m.logs.searchTerm == "" {
		firstLine := m.logs.logLineNumbers[0]
		lastLine := m.logs.logLineNumbers[len(m.logs.logLineNumbers)-1]
		if firstLine == lastLine {
			title += fmt.Sprintf(" [L%d]", firstLine)
		} else {
			title += fmt.Sprintf(" [L%d-%d]", firstLine, lastLine)
		}
	}
	if m.logs.searchTerm != "" && !m.logs.searchMode {
		matchLine := ""
		if m.logs.matchIndex < len(m.logs.searchMatchLines) {
			matchLine = fmt.Sprintf(" L%d", m.logs.searchMatchLines[m.logs.matchIndex])
		}
		title += fmt.Sprintf(" [%q %d/%d%s]", m.logs.searchTerm, m.logs.matchIndex+1, len(m.logs.searchMatches), matchLine)
	}
	title += m.logs.logNoiseLabel()
	return title
}

func (m Model) logContentView() string {
	if m.logs.logsLoading && m.logs.originalLogContent == "" {
		return m.dimText("Loading logs…")
	}
	if len(m.tasks) == 0 {
		return m.dimText("No tasks yet. Use chat or `watchy start` to create one.")
	}
	if m.logs.originalLogContent == "" {
		return m.dimText("No output yet.")
	}
	viewport := m.logs.logViewport
	viewport.Height = m.innerHeight
	if m.logs.searchMode {
		viewport.Height--
	}
	if m.logs.commandMode {
		viewport.Height--
	}
	if viewport.Height < 0 {
		viewport.Height = 0
	}
	return viewport.View()
}

func (m Model) colorModeLabel() string {
	mode := "auto"
	if m.cfg != nil && m.cfg.ColorMode != "" {
		mode = m.cfg.ColorMode
	}

	resolved := "dark"
	if m.lightMode {
		resolved = "light"
	}
	if mode == "auto" {
		return "auto:" + resolved
	}
	return resolved
}
