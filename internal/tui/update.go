package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/parth/watchy/internal/task"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.recalcLayout()
		return m, nil

	case tickMsg:
		cmds = append(cmds, tickEvery(2*time.Second))
		cmds = append(cmds, fetchTasks(m.mgr))
		if len(m.tasks) > 0 && m.selectedIdx < len(m.tasks) && m.rightMode == modeLog {
			cmds = append(cmds, fetchLogs(m.mgr, m.tasks[m.selectedIdx].ID))
		}
		return m, tea.Batch(cmds...)

	case tasksUpdatedMsg:
		m.tasks = ([]*task.Task)(msg)
		if m.selectedIdx >= len(m.tasks) && len(m.tasks) > 0 {
			m.selectedIdx = len(m.tasks) - 1
		}
		m.conversation.RefreshSystemPrompt()
		return m, nil

	case logContentMsg:
		m.logViewport.SetContent(string(msg))
		m.logViewport.GotoBottom()
		return m, nil

	case agentToolStartMsg:
		m.chatHistory = append(m.chatHistory, chatMessage{
			role:    "tool",
			content: fmt.Sprintf("[%s] %s", msg.Tool, msg.Args),
		})
		m.updateChatViewport()
		return m, nil

	case agentToolResultMsg:
		truncResult := msg.Result
		if len(truncResult) > 300 {
			truncResult = truncResult[:300] + "..."
		}
		m.chatHistory = append(m.chatHistory, chatMessage{
			role:    "tool",
			content: fmt.Sprintf("-> %s", truncResult),
		})
		m.updateChatViewport()
		return m, nil

	case agentResponseMsg:
		m.agentBusy = false
		m.agentCancel = nil
		m.chatHistory = append(m.chatHistory, chatMessage{role: "agent", content: string(msg)})
		m.updateChatViewport()
		return m, nil

	case agentErrorMsg:
		m.agentBusy = false
		m.agentCancel = nil
		m.chatHistory = append(m.chatHistory, chatMessage{role: "agent", content: fmt.Sprintf("Error: %s", msg.err)})
		m.updateChatViewport()
		return m, nil

	case taskStoppedMsg:
		return m, fetchTasks(m.mgr)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Pass to active components
	if m.activePane == paneRight && m.rightMode == modeChat {
		var cmd tea.Cmd
		m.chatInput, cmd = m.chatInput.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Esc cancels in-flight agent request
	if key == "esc" && m.agentBusy && m.agentCancel != nil {
		m.agentCancel()
		m.agentBusy = false
		m.agentCancel = nil
		m.chatHistory = append(m.chatHistory, chatMessage{role: "agent", content: "[cancelled]"})
		m.updateChatViewport()
		return m, nil
	}

	// Global quit
	if key == "q" || key == "ctrl+c" {
		if m.activePane == paneRight && m.rightMode == modeChat && m.chatInput.Focused() {
			if key == "ctrl+c" {
				return m, tea.Quit
			}
			// q in chat input is just a character
			var cmd tea.Cmd
			m.chatInput, cmd = m.chatInput.Update(msg)
			return m, cmd
		}
		return m, tea.Quit
	}

	// Chat input handling when focused
	if m.activePane == paneRight && m.rightMode == modeChat && m.chatInput.Focused() {
		if key == "esc" {
			m.chatInput.Blur()
			return m, nil
		}
		if key == "enter" && !m.agentBusy {
			text := m.chatInput.Value()
			if text != "" {
				m.chatInput.Reset()

				// Handle /model command
				if strings.HasPrefix(text, "/model") {
					parts := strings.Fields(text)
					if len(parts) == 1 {
						m.chatHistory = append(m.chatHistory, chatMessage{
							role: "agent", content: "current model: " + m.agent.Model(),
						})
					} else {
						newModel := parts[1]
						m.agent.SetModel(newModel)
						m.chatHistory = append(m.chatHistory, chatMessage{
							role: "agent", content: "model set to: " + newModel,
						})
					}
					m.updateChatViewport()
					return m, nil
				}

				m.chatHistory = append(m.chatHistory, chatMessage{role: "user", content: text})
				m.updateChatViewport()
				m.agentBusy = true
				ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				m.agentCancel = cancel
				return m, sendToAgent(m.conversation, text, ctx, m.programRef.p)
			}
			return m, nil
		}
		var cmd tea.Cmd
		m.chatInput, cmd = m.chatInput.Update(msg)
		return m, cmd
	}

	switch key {
	case "j", "down":
		if m.activePane == paneLeft && len(m.tasks) > 0 {
			m.selectedIdx++
			if m.selectedIdx >= len(m.tasks) {
				m.selectedIdx = len(m.tasks) - 1
			}
			if m.rightMode == modeLog {
				return m, fetchLogs(m.mgr, m.tasks[m.selectedIdx].ID)
			}
		} else if m.activePane == paneRight {
			if m.rightMode == modeLog {
				m.logViewport.LineDown(1)
			} else {
				m.chatViewport.LineDown(1)
			}
		}
	case "k", "up":
		if m.activePane == paneLeft && len(m.tasks) > 0 {
			m.selectedIdx--
			if m.selectedIdx < 0 {
				m.selectedIdx = 0
			}
			if m.rightMode == modeLog {
				return m, fetchLogs(m.mgr, m.tasks[m.selectedIdx].ID)
			}
		} else if m.activePane == paneRight {
			if m.rightMode == modeLog {
				m.logViewport.LineUp(1)
			} else {
				m.chatViewport.LineUp(1)
			}
		}
	case "tab":
		if m.activePane == paneLeft {
			m.activePane = paneRight
		} else {
			m.activePane = paneLeft
			m.chatInput.Blur()
		}
	case "t":
		m.themeIdx = (m.themeIdx + 1) % len(themes)
		m.cfg.Theme = themes[m.themeIdx].name
		m.cfg.Save()
	case "h":
		m.leftHidden = !m.leftHidden
		m.recalcLayout()
	case "l":
		m.rightMode = modeLog
		m.chatInput.Blur()
		// Find latest running task
		for i := len(m.tasks) - 1; i >= 0; i-- {
			if m.tasks[i].Status == "running" {
				m.selectedIdx = i
				break
			}
		}
		if len(m.tasks) > 0 && m.selectedIdx < len(m.tasks) {
			return m, fetchLogs(m.mgr, m.tasks[m.selectedIdx].ID)
		}
	case "c":
		m.rightMode = modeChat
		m.activePane = paneRight
		m.chatInput.Focus()
	case "enter":
		if m.activePane == paneLeft && len(m.tasks) > 0 && m.selectedIdx < len(m.tasks) {
			// Open logs for selected task
			m.rightMode = modeLog
			m.activePane = paneRight
			return m, fetchLogs(m.mgr, m.tasks[m.selectedIdx].ID)
		} else if m.activePane == paneRight && m.rightMode == modeChat {
			m.chatInput.Focus()
		}
	case "x":
		if len(m.tasks) > 0 && m.selectedIdx < len(m.tasks) {
			t := m.tasks[m.selectedIdx]
			if t.Status == "running" {
				return m, stopTask(m.mgr, t.ID)
			}
		}
	}

	return m, nil
}

func (m *Model) recalcLayout() {
	var rightWidth int
	contentHeight := m.height - 4 // status bar + borders

	if m.leftHidden {
		rightWidth = m.width - 2 // just borders
	} else {
		leftWidth := m.width * 30 / 100
		rightWidth = m.width - leftWidth - 3 // borders
	}

	m.logViewport.Width = rightWidth
	m.logViewport.Height = contentHeight

	chatInputHeight := 3
	m.chatViewport.Width = rightWidth
	m.chatViewport.Height = contentHeight - chatInputHeight - 1
	m.chatInput.SetWidth(rightWidth)
}

func (m *Model) updateChatViewport() {
	content := ""
	for i, msg := range m.chatHistory {
		if i > 0 {
			content += "\n\n"
		}
		switch msg.role {
		case "user":
			content += "> " + msg.content
		case "tool":
			content += "  " + msg.content
		default:
			content += msg.content
		}
	}
	if m.agentBusy {
		if content != "" {
			content += "\n\n"
		}
		content += "thinking..."
	}
	m.chatViewport.SetContent(content)
	m.chatViewport.GotoBottom()
}
