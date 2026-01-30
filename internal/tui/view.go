package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	green     = lipgloss.Color("34")
	brightGreen = lipgloss.Color("46")
	dimGreen  = lipgloss.Color("22")
	red       = lipgloss.Color("124")
	dim       = lipgloss.Color("240")

	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(dim))

	activeBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(brightGreen)

	selectedStyle = lipgloss.NewStyle().Background(dimGreen).Bold(true).Foreground(brightGreen)
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(brightGreen)
	dimStyle      = lipgloss.NewStyle().Foreground(dim)
)

func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	leftWidth := m.width * 30 / 100
	rightWidth := m.width - leftWidth - 3
	contentHeight := m.height - 4

	// Left pane: task list
	leftContent := m.renderTaskList(leftWidth-2, contentHeight)
	leftPane := m.applyBorder(paneLeft, leftWidth, contentHeight, "Tasks", leftContent)

	// Right pane: logs or chat
	var rightContent, rightTitle string
	if m.rightMode == modeLog {
		rightTitle = "Logs"
		rightContent = m.logViewport.View()
	} else {
		rightTitle = "Chat"
		rightContent = m.chatViewport.View() + "\n" + m.chatInput.View()
	}
	rightPane := m.applyBorder(paneRight, rightWidth, contentHeight, rightTitle, rightContent)

	main := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)

	// Status bar
	statusBar := m.renderStatusBar()

	return main + "\n" + statusBar
}

func (m Model) applyBorder(p pane, width, height int, title, content string) string {
	style := borderStyle
	if m.activePane == p {
		style = activeBorderStyle
	}
	return style.
		Width(width).
		Height(height).
		Render(titleStyle.Render(title) + "\n" + content)
}

func (m Model) renderTaskList(width, height int) string {
	if len(m.tasks) == 0 {
		return dimStyle.Render("No tasks. Use chat to start one.")
	}

	var lines []string
	for i, t := range m.tasks {
		var indicator string
		switch t.Status {
		case "running":
			indicator = lipgloss.NewStyle().Foreground(brightGreen).Render("[R]")
		case "crashed":
			indicator = lipgloss.NewStyle().Foreground(red).Render("[X]")
		default:
			indicator = lipgloss.NewStyle().Foreground(dim).Render("[-]")
		}

		name := t.Name
		maxName := width - 10
		if maxName < 10 {
			maxName = 10
		}
		if len(name) > maxName {
			name = name[:maxName-3] + "..."
		}

		line := fmt.Sprintf(" %s %-3d %s", indicator, t.ID, name)

		if i == m.selectedIdx {
			line = selectedStyle.Render(line)
		}

		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

func (m Model) renderStatusBar() string {
	var parts []string

	if m.agentBusy {
		parts = append(parts, lipgloss.NewStyle().Foreground(brightGreen).Render("[agent working... esc:cancel]"))
	}

	keys := "j/k:navigate  tab:switch pane  l:logs  c:chat  x:stop  q:quit"
	parts = append(parts, dimStyle.Render(keys))

	return strings.Join(parts, "  ")
}

