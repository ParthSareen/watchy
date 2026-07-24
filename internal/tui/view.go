package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type theme struct {
	name     string
	bright   lipgloss.Color
	dim      lipgloss.Color
	bg       lipgloss.Color
	brightLg lipgloss.Color
	dimLg    lipgloss.Color
	bgLg     lipgloss.Color
}

var themes = []theme{
	{"green", lipgloss.Color("46"), lipgloss.Color("22"), lipgloss.Color("22"), lipgloss.Color("28"), lipgloss.Color("85"), lipgloss.Color("157")},
	{"blue", lipgloss.Color("39"), lipgloss.Color("24"), lipgloss.Color("24"), lipgloss.Color("32"), lipgloss.Color("74"), lipgloss.Color("117")},
	{"purple", lipgloss.Color("141"), lipgloss.Color("54"), lipgloss.Color("54"), lipgloss.Color("135"), lipgloss.Color("183"), lipgloss.Color("225")},
	{"orange", lipgloss.Color("208"), lipgloss.Color("94"), lipgloss.Color("94"), lipgloss.Color("202"), lipgloss.Color("220"), lipgloss.Color("223")},
	{"pink", lipgloss.Color("205"), lipgloss.Color("125"), lipgloss.Color("125"), lipgloss.Color("198"), lipgloss.Color("211"), lipgloss.Color("218")},
	{"cyan", lipgloss.Color("51"), lipgloss.Color("30"), lipgloss.Color("30"), lipgloss.Color("37"), lipgloss.Color("87"), lipgloss.Color("122")},
	{"red", lipgloss.Color("196"), lipgloss.Color("88"), lipgloss.Color("88"), lipgloss.Color("196"), lipgloss.Color("203"), lipgloss.Color("224")},
	{"white", lipgloss.Color("255"), lipgloss.Color("245"), lipgloss.Color("245"), lipgloss.Color("0"), lipgloss.Color("7"), lipgloss.Color("15")},
}

var (
	errorColor  = lipgloss.Color("124")
	errorColorL = lipgloss.Color("196")
	dimGray     = lipgloss.Color("240")
	dimGrayL    = lipgloss.Color("242")
)

func (m Model) theme() theme {
	return themes[m.themeIdx%len(themes)]
}

// bright returns the bright (foreground) color based on light/dark mode
func (m Model) bright() lipgloss.Color {
	t := m.theme()
	if m.lightMode {
		return t.brightLg
	}
	return t.bright
}

// dim returns the dim (foreground) color based on light/dark mode
func (m Model) dim() lipgloss.Color {
	t := m.theme()
	if m.lightMode {
		return t.dimLg
	}
	return t.dim
}

// bg returns the background color for selected items based on light/dark mode
func (m Model) bg() lipgloss.Color {
	t := m.theme()
	if m.lightMode {
		return t.bgLg
	}
	return t.bg
}

// dimGrayForMode returns the appropriate dim gray color
func (m Model) dimGrayForMode() lipgloss.Color {
	if m.lightMode {
		return dimGrayL
	}
	return dimGray
}

// errorColorForMode returns the appropriate error color
func (m Model) errorColorForMode() lipgloss.Color {
	if m.lightMode {
		return errorColorL
	}
	return errorColor
}

func (m *Model) syncChatPalette() {
	m.chat.SetPalette(chatPalette{
		bright: m.bright(),
		dim:    m.dim(),
		muted:  m.dimGrayForMode(),
		err:    m.errorColorForMode(),
	})
}

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
		leftContent := m.renderTaskList(m.leftWidth-2, 0)
		leftPane = m.applyBorder(paneLeft, m.leftWidth, m.boxHeight, "Tasks", leftContent)
	}

	// Right pane: logs, chat, or split
	var rightPane string
	if m.rightMode == modeLog {
		rightTitle := m.viewTabs()
		if len(m.tasks) > 0 && m.selectedIdx < len(m.tasks) {
			rightTitle += fmt.Sprintf(" • %d %s • %s", m.tasks[m.selectedIdx].ID, m.tasks[m.selectedIdx].Name, m.tasks[m.selectedIdx].Status)
		}
		// Show line number range when available
		if len(m.logLineNumbers) > 0 && m.searchTerm == "" {
			firstLine := m.logLineNumbers[0]
			lastLine := m.logLineNumbers[len(m.logLineNumbers)-1]
			if firstLine == lastLine {
				rightTitle += fmt.Sprintf(" [L%d]", firstLine)
			} else {
				rightTitle += fmt.Sprintf(" [L%d-%d]", firstLine, lastLine)
			}
		}
		if m.searchTerm != "" && !m.searchMode {
			// Show original line number for current match
			matchLine := ""
			if m.matchIndex < len(m.searchMatchLines) {
				matchLine = fmt.Sprintf(" L%d", m.searchMatchLines[m.matchIndex])
			}
			rightTitle += fmt.Sprintf(" [%q %d/%d%s]", m.searchTerm, m.matchIndex+1, len(m.searchMatches), matchLine)
		}
		rightTitle += m.logNoiseLabel()
		rightContent := m.logContentView()
		if m.searchMode {
			rightContent += "\n" + m.searchInput.View()
		}
		if m.commandMode {
			rightContent += "\n" + m.commandInput.View()
		}
		rightPane = m.applyBorder(paneRight, m.rightWidth, m.boxHeight, rightTitle, rightContent)
	} else if m.rightMode == modeChat {
		rightTitle := m.viewTabs()
		rightContent := m.chat.View(m.focusedArea == focusChatInput)
		rightPane = m.applyBorder(paneChat, m.rightWidth, m.boxHeight, rightTitle, rightContent)
	} else {
		// Split mode: chat and logs side by side
		splitWidth := m.rightWidth/2 - 1

		// Logs pane (left half of right section)
		logsTitle := m.viewTabs()
		if len(m.tasks) > 0 && m.selectedIdx < len(m.tasks) {
			logsTitle += fmt.Sprintf(" • %d %s", m.tasks[m.selectedIdx].ID, m.tasks[m.selectedIdx].Name)
		}
		// Show line number range when available
		if len(m.logLineNumbers) > 0 && m.searchTerm == "" {
			firstLine := m.logLineNumbers[0]
			lastLine := m.logLineNumbers[len(m.logLineNumbers)-1]
			if firstLine == lastLine {
				logsTitle += fmt.Sprintf(" [L%d]", firstLine)
			} else {
				logsTitle += fmt.Sprintf(" [L%d-%d]", firstLine, lastLine)
			}
		}
		if m.searchTerm != "" && !m.searchMode {
			// Show original line number for current match
			matchLine := ""
			if m.matchIndex < len(m.searchMatchLines) {
				matchLine = fmt.Sprintf(" L%d", m.searchMatchLines[m.matchIndex])
			}
			logsTitle += fmt.Sprintf(" [%q %d/%d%s]", m.searchTerm, m.matchIndex+1, len(m.searchMatches), matchLine)
		}
		logsTitle += m.logNoiseLabel()
		logsContent := m.logContentView()
		if m.searchMode {
			logsContent += "\n" + m.searchInput.View()
		}
		logsPane := m.applyBorder(paneRight, splitWidth, m.boxHeight, logsTitle, logsContent)

		// Chat pane (right half of right section)
		chatTitle := "Chat"
		chatContent := m.chat.View(m.focusedArea == focusChatInput)
		chatPane := m.applyBorder(paneChat, splitWidth, m.boxHeight, chatTitle, chatContent)

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

	if height > 0 {
		style = style.Height(height)
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(bright)
	return style.Render(titleStyle.Render(title) + "\n" + content)
}

func (m Model) renderTaskList(width, height int) string {
	bright := m.bright()
	bgColor := m.bg()
	dimGrayColor := m.dimGrayForMode()
	errColor := m.errorColorForMode()

	dimStyle := lipgloss.NewStyle().Foreground(dimGrayColor)

	if len(m.tasks) == 0 {
		return dimStyle.Render("No tasks. Use chat to start one.")
	}

	var lines []string
	for i, task := range m.tasks {
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
		if maxName < 10 {
			maxName = 10
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
	if m.visualMode {
		left = "visual • " + left
	}
	if m.commandMode {
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
		return "j/k move • enter logs • d details • x stop • r restart • tab focus • ? help"
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

func (m Model) logContentView() string {
	if m.logsLoading && m.originalLogContent == "" {
		return m.dimText("Loading logs…")
	}
	if len(m.tasks) == 0 {
		return m.dimText("No tasks yet. Use chat or `watchy start` to create one.")
	}
	if m.originalLogContent == "" {
		return m.dimText("No output yet.")
	}
	return m.logViewport.View()
}

func truncateRunes(value string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width == 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}

func (m Model) logNoiseLabel() string {
	if m.showLogNoise {
		return " [noise shown]"
	}
	if m.hiddenLogNoise > 0 {
		return fmt.Sprintf(" [%d noise hidden]", m.hiddenLogNoise)
	}
	return ""
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
