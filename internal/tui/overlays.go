package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m *Model) openModelPicker() tea.Cmd {
	m.modelPicker = true
	m.modelPickerLoading = true
	m.modelPickerErr = nil
	m.modelPickerIdx = 0
	m.modelPickerInput.SetValue("")
	m.chat.Blur()
	return tea.Batch(m.modelPickerInput.Focus(), listModels(m.agent))
}

func (m Model) modelPickerChoices() []string {
	query := strings.TrimSpace(m.modelPickerInput.Value())
	queryLower := strings.ToLower(query)
	choices := make([]string, 0, len(m.modelPickerModels)+1)
	exact := false
	for _, model := range m.modelPickerModels {
		if queryLower != "" && !strings.Contains(strings.ToLower(model), queryLower) {
			continue
		}
		if model == query {
			exact = true
		}
		choices = append(choices, model)
	}
	if query != "" && !exact {
		choices = append([]string{query}, choices...)
	}
	if len(choices) == 0 && query == "" {
		return []string{m.agent.Model()}
	}
	return choices
}

func (m Model) handleModelPickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	choices := m.modelPickerChoices()
	switch key {
	case "esc":
		m.modelPicker = false
		m.modelPickerInput.Blur()
		return m, nil
	case "up":
		if len(choices) > 0 {
			m.modelPickerIdx--
			if m.modelPickerIdx < 0 {
				m.modelPickerIdx = len(choices) - 1
			}
		}
		return m, nil
	case "down", "tab":
		if len(choices) > 0 {
			m.modelPickerIdx = (m.modelPickerIdx + 1) % len(choices)
		}
		return m, nil
	case "enter", "ctrl+s":
		if len(choices) == 0 {
			return m, nil
		}
		if m.modelPickerIdx >= len(choices) {
			m.modelPickerIdx = len(choices) - 1
		}
		model := choices[m.modelPickerIdx]
		m.agent.SetModel(model)
		m.modelPicker = false
		m.modelPickerInput.Blur()
		if key == "ctrl+s" {
			if m.cfg == nil {
				return m, m.setStatus("model set for this session; no config is available to save", true)
			}
			previous := m.cfg.Model
			m.cfg.Model = model
			if err := m.cfg.Save(); err != nil {
				m.cfg.Model = previous
				return m, m.setStatus("could not save default model: "+err.Error(), true)
			}
			return m, m.setStatus("model set and saved as default: "+model, false)
		}
		return m, m.setStatus("model set for this session: "+model, false)
	}

	before := m.modelPickerInput.Value()
	var cmd tea.Cmd
	m.modelPickerInput, cmd = m.modelPickerInput.Update(msg)
	if m.modelPickerInput.Value() != before {
		m.modelPickerIdx = 0
	}
	return m, cmd
}

func (m Model) renderModelPicker() string {
	width := 64
	if m.width-4 < width {
		width = m.width - 4
	}
	if width < 24 {
		width = 24
	}

	var lines []string
	lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(m.bright()).Render("Choose Ollama model"))
	lines = append(lines, m.modelPickerInput.View(), "")

	choices := m.modelPickerChoices()
	if m.modelPickerLoading {
		lines = append(lines, m.dimText("Loading available models…"))
	} else if m.modelPickerErr != nil {
		lines = append(lines, m.errorText("Could not list models; manual entry still works."))
	}

	const visible = 9
	start := 0
	if m.modelPickerIdx >= visible {
		start = m.modelPickerIdx - visible + 1
	}
	end := start + visible
	if end > len(choices) {
		end = len(choices)
	}
	for i := start; i < end; i++ {
		prefix := "  "
		style := lipgloss.NewStyle()
		if i == m.modelPickerIdx {
			prefix = "› "
			style = style.Bold(true).Foreground(m.bright())
		}
		suffix := ""
		if choices[i] == m.agent.Model() {
			suffix = "  current"
		}
		lines = append(lines, style.Render(prefix+choices[i]+suffix))
	}
	lines = append(lines, "", m.dimText("↑/↓ select  enter use this session  ctrl+s save default  esc cancel"))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.bright()).
		Padding(1, 2).
		Width(width).
		Render(strings.Join(lines, "\n"))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m Model) renderHelp() string {
	sections := []string{
		"Navigation\n  tab / shift+tab  focus visible panes\n  l / c / s          logs, chat, or split view\n  [ / ]              cycle logs, chat, and split views\n  h                  hide or show tasks\n  enter              open task, toggle sidebar, or compose\n  esc                leave the current mode",
		"Tasks\n  j/k or ↑/↓         select task\n  f                  show running tasks only\n  x                  stop selected running task\n  r                  restart selected finished task\n  d                  show full task details",
		"Logs\n  j/k, g/G           move or jump (5j moves 5 lines)\n  :                  jump to a line number\n  /, n/N             search and move matches\n  v, y               select and copy\n  </> 0 ^            horizontal scroll and home\n  u                  show or hide routine noise",
		"Chat\n  i / enter          focus composer\n  ctrl+left          switch to logs from composer\n  ctrl+j             newline\n  esc                blur or cancel request\n  e / y              expand or copy latest tool",
		"General\n  M                  choose Ollama model\n  t / m              theme and light/dark mode\n  ?                  close this help\n  q / ctrl+c         quit",
	}
	content := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.bright()).
		Padding(1, 2).
		Width(minInt(70, maxInt(28, m.width-4))).
		Render(lipgloss.NewStyle().Bold(true).Foreground(m.bright()).Render("Watchy help") + "\n\n" + strings.Join(sections, "\n\n"))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

func (m Model) renderTaskDetails() string {
	vis := m.visibleTasks()
	if len(vis) == 0 || m.selectedIdx >= len(vis) {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, "No task selected")
	}
	task := vis[m.selectedIdx]
	end := "running"
	if task.EndTime != nil {
		end = task.EndTime.Format("2006-01-02 15:04:05")
	}
	lines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(m.bright()).Render(fmt.Sprintf("Task %d • %s", task.ID, task.Name)),
		"",
		fmt.Sprintf("Status:    %s", task.Status),
		fmt.Sprintf("PID:       %d", task.PID),
		fmt.Sprintf("Started:   %s", task.StartTime.Format("2006-01-02 15:04:05")),
		fmt.Sprintf("Ended:     %s", end),
		fmt.Sprintf("Directory: %s", task.WorkDir),
		fmt.Sprintf("Log:       %s", task.LogPath),
		"",
		"Command:",
		task.Command,
		"",
		m.dimText("d / esc close"),
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.bright()).
		Padding(1, 2).
		Width(minInt(78, maxInt(28, m.width-4))).
		Render(strings.Join(lines, "\n"))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m *Model) setStatus(message string, isError bool) tea.Cmd {
	m.statusSeq++
	seq := m.statusSeq
	m.statusMessage = message
	m.statusError = isError
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg {
		return clearStatusMsg(seq)
	})
}

func (m Model) dimText(text string) string {
	return lipgloss.NewStyle().Foreground(m.dimGrayForMode()).Render(text)
}

func (m Model) errorText(text string) string {
	return lipgloss.NewStyle().Foreground(m.errorColorForMode()).Render(text)
}

func (m Model) viewTabs() string {
	labels := []string{"Logs", "Chat", "Split"}
	active := int(m.rightMode)
	for i := range labels {
		if i == active {
			labels[i] = "[" + labels[i] + "]"
		}
	}
	return strings.Join(labels, " ")
}

func (m Model) modelStatus() string {
	if m.agent == nil {
		return "model unavailable"
	}
	kind := "local"
	if strings.HasSuffix(m.agent.Model(), ":cloud") {
		kind = "cloud"
	}
	return fmt.Sprintf("%s • %s", m.agent.Model(), kind)
}
