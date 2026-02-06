package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type theme struct {
	name   string
	bright lipgloss.Color
	dim    lipgloss.Color
}

var themes = []theme{
	{"green", lipgloss.Color("46"), lipgloss.Color("22")},
	{"blue", lipgloss.Color("39"), lipgloss.Color("24")},
	{"purple", lipgloss.Color("141"), lipgloss.Color("54")},
	{"orange", lipgloss.Color("208"), lipgloss.Color("94")},
	{"pink", lipgloss.Color("205"), lipgloss.Color("125")},
	{"cyan", lipgloss.Color("51"), lipgloss.Color("30")},
	{"red", lipgloss.Color("196"), lipgloss.Color("88")},
	{"white", lipgloss.Color("255"), lipgloss.Color("245")},
}

var (
	errorColor = lipgloss.Color("124")
	dimGray    = lipgloss.Color("240")
)

func (m Model) theme() theme {
	return themes[m.themeIdx%len(themes)]
}

func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	contentHeight := m.height - 4

	var rightWidth int
	var leftPane string

	if m.leftHidden {
		rightWidth = m.width - 2
	} else {
		leftWidth := m.width * 30 / 100
		rightWidth = m.width - leftWidth - 3

		// Left pane: task list
		leftContent := m.renderTaskList(leftWidth-2, contentHeight)
		leftPane = m.applyBorder(paneLeft, leftWidth, contentHeight, "Tasks", leftContent)
	}

	// Right pane: logs or chat
	var rightContent, rightTitle string
	if m.rightMode == modeLog {
		rightTitle = "Logs"
		if len(m.tasks) > 0 && m.selectedIdx < len(m.tasks) {
			rightTitle = fmt.Sprintf("Logs [%d: %s]", m.tasks[m.selectedIdx].ID, m.tasks[m.selectedIdx].Name)
		}
		if m.searchTerm != "" && !m.searchMode {
			rightTitle += fmt.Sprintf(" [%q %d/%d]", m.searchTerm, m.matchIndex+1, len(m.searchMatches))
		}
		rightContent = m.logViewport.View()
		if m.searchMode {
			rightContent += "\n" + m.searchInput.View()
		}
	} else {
		rightTitle = "Chat"
		picker := m.renderSlashPicker()
		rightContent = m.chatViewport.View() + "\n" + picker + m.chatInput.View()
	}
	rightPane := m.applyBorder(paneRight, rightWidth, contentHeight, rightTitle, rightContent)

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
	t := m.theme()
	borderColor := dimGray
	if m.activePane == p {
		borderColor = t.bright
	}
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor)
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(t.bright)
	return style.
		Width(width).
		Height(height).
		Render(titleStyle.Render(title) + "\n" + content)
}

func (m Model) renderTaskList(width, height int) string {
	t := m.theme()
	dimStyle := lipgloss.NewStyle().Foreground(dimGray)

	if len(m.tasks) == 0 {
		return dimStyle.Render("No tasks. Use chat to start one.")
	}

	var lines []string
	for i, task := range m.tasks {
		var indicator string
		switch task.Status {
		case "running":
			indicator = lipgloss.NewStyle().Foreground(t.bright).Render("[R]")
		case "crashed":
			indicator = lipgloss.NewStyle().Foreground(errorColor).Render("[X]")
		default:
			indicator = dimStyle.Render("[-]")
		}

		name := task.Name
		maxName := width - 10
		if maxName < 10 {
			maxName = 10
		}
		if len(name) > maxName {
			name = name[:maxName-3] + "..."
		}

		line := fmt.Sprintf(" %s %-3d %s", indicator, task.ID, name)

		if i == m.selectedIdx {
			selectedStyle := lipgloss.NewStyle().Background(t.dim).Bold(true).Foreground(t.bright)
			line = selectedStyle.Render(line)
		}

		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

func (m Model) renderSlashPicker() string {
	if !m.showSlashPicker() {
		return ""
	}

	filtered := m.filteredSlashCommands()
	if len(filtered) == 0 {
		return ""
	}

	t := m.theme()
	dimStyle := lipgloss.NewStyle().Foreground(dimGray)
	selectedStyle := lipgloss.NewStyle().Bold(true).Foreground(t.bright)

	var lines []string
	idx := m.slashPickerIdx % len(filtered)
	for i, cmd := range filtered {
		line := fmt.Sprintf("  %-10s %s", cmd.name, cmd.desc)
		if i == idx {
			line = selectedStyle.Render(line)
		} else {
			line = dimStyle.Render(line)
		}
		lines = append(lines, line)
	}

	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.dim).
		Padding(0, 1)

	return border.Render(strings.Join(lines, "\n")) + "\n"
}

func (m Model) renderStatusBar() string {
	t := m.theme()
	dimStyle := lipgloss.NewStyle().Foreground(dimGray)

	var parts []string

	if m.agentBusy {
		parts = append(parts, lipgloss.NewStyle().Foreground(t.bright).Render("[agent working... esc:cancel]"))
	}

	keys := fmt.Sprintf("j/k:nav  g/G:top/bottom  /:search  n/N:match  tab:pane  l:logs  c:chat  h:hide  t:theme(%s)  x:stop  r:restart  q:quit", t.name)
	parts = append(parts, dimStyle.Render(keys))

	return strings.Join(parts, "  ")
}

