package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type theme struct {
	name      string
	bright    lipgloss.Color
	dim       lipgloss.Color
	bg        lipgloss.Color
	brightLg  lipgloss.Color
	dimLg     lipgloss.Color
	bgLg      lipgloss.Color
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
	dimGrayL    = lipgloss.Color("250")
)

func (m Model) theme() theme {
	return themes[m.themeIdx%len(themes)]
}

func detectLightMode() bool {
	colorfgbg := os.Getenv("COLORFGBG")
	if colorfgbg == "" {
		return false
	}
	// Format: COLORFGBG=bg;fg (e.g., "0;15" for dark, "7;15" for light)
	// Background values: 0-6 = dark, 7-15 = light
	for _, part := range strings.Split(colorfgbg, ";") {
		var val int
		if _, err := fmt.Sscanf(part, "%d", &val); err == nil {
			if val >= 7 && val <= 15 {
				return true
			}
		}
	}
	return false
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

func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	var leftPane string
	if !m.leftHidden {
		leftContent := m.renderTaskList(m.leftWidth-2, 0)
		leftPane = m.applyBorder(paneLeft, m.leftWidth, m.boxHeight, "Tasks", leftContent)
	}

	// Right pane: logs, chat, or split
	var rightPane string
	if m.rightMode == modeLog {
		rightTitle := "Logs"
		if len(m.tasks) > 0 && m.selectedIdx < len(m.tasks) {
			rightTitle = fmt.Sprintf("Logs [%d: %s]", m.tasks[m.selectedIdx].ID, m.tasks[m.selectedIdx].Name)
		}
		if m.searchTerm != "" && !m.searchMode {
			rightTitle += fmt.Sprintf(" [%q %d/%d]", m.searchTerm, m.matchIndex+1, len(m.searchMatches))
		}
		rightContent := m.logViewport.View()
		if m.searchMode {
			rightContent += "\n" + m.searchInput.View()
		}
		rightPane = m.applyBorder(paneRight, m.rightWidth, m.boxHeight, rightTitle, rightContent)
	} else if m.rightMode == modeChat {
		rightTitle := "Chat"
		picker := m.renderSlashPicker()
		rightContent := m.chatViewport.View() + "\n" + picker + m.chatInput.View()
		rightPane = m.applyBorder(paneRight, m.rightWidth, m.boxHeight, rightTitle, rightContent)
	} else {
		// Split mode: chat and logs side by side
		splitWidth := m.rightWidth/2 - 1

		// Logs pane (left half of right section)
		logsTitle := "Logs"
		if len(m.tasks) > 0 && m.selectedIdx < len(m.tasks) {
			logsTitle = fmt.Sprintf("Logs [%d: %s]", m.tasks[m.selectedIdx].ID, m.tasks[m.selectedIdx].Name)
		}
		if m.searchTerm != "" && !m.searchMode {
			logsTitle += fmt.Sprintf(" [%q %d/%d]", m.searchTerm, m.matchIndex+1, len(m.searchMatches))
		}
		logsContent := m.logViewport.View()
		if m.searchMode {
			logsContent += "\n" + m.searchInput.View()
		}
		logsPane := m.applyBorder(paneRight, splitWidth, m.boxHeight, logsTitle, logsContent)

		// Chat pane (right half of right section)
		chatTitle := "Chat"
		picker := m.renderSlashPicker()
		chatContent := m.chatViewport.View() + "\n" + picker + m.chatInput.View()
		chatPane := m.applyBorder(paneRight, splitWidth, m.boxHeight, chatTitle, chatContent)

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
	if m.activePane == p {
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
			indicator = lipgloss.NewStyle().Foreground(bright).Render("[R]")
		case "crashed":
			indicator = lipgloss.NewStyle().Foreground(errColor).Render("[X]")
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
			selectedStyle := lipgloss.NewStyle().Background(bgColor).Bold(true).Foreground(bright)
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

	bright := m.bright()
	dimColor := m.dim()
	dimGrayColor := m.dimGrayForMode()

	dimStyle := lipgloss.NewStyle().Foreground(dimGrayColor)
	selectedStyle := lipgloss.NewStyle().Bold(true).Foreground(bright)

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
		BorderForeground(dimColor).
		Padding(0, 1)

	return border.Render(strings.Join(lines, "\n")) + "\n"
}

func (m Model) renderStatusBar() string {
	bright := m.bright()
	dimGrayColor := m.dimGrayForMode()

	dimStyle := lipgloss.NewStyle().Foreground(dimGrayColor)

	var parts []string

	if m.copied {
		parts = append(parts, lipgloss.NewStyle().Foreground(bright).Render("[copied!]"))
	}

	if m.visualMode {
		parts = append(parts, lipgloss.NewStyle().Foreground(bright).Render("[visual]"))
	}

	if m.agentBusy {
		parts = append(parts, lipgloss.NewStyle().Foreground(bright).Render("[agent working... esc:cancel]"))
	}

	modeIndicator := "dark"
	if m.lightMode {
		modeIndicator = "light"
	}
	keys := fmt.Sprintf("j/k:nav  g/G:top/bottom  v:visual  y:copy  /:search  n/N:match  tab:cycle  t:theme(%s)  m:mode(%s)  x:stop  r:restart  q:quit", m.theme().name, modeIndicator)
	parts = append(parts, dimStyle.Render(keys))

	return strings.Join(parts, "  ")
}

