package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/parth/watchy/internal/logcolor"
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
		if len(m.tasks) > 0 && m.selectedIdx < len(m.tasks) && (m.rightMode == modeLog || m.rightMode == modeSplit) {
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
		oldTotalLines := m.totalLogLines()
		cursorAtBottom := oldTotalLines > 0 && m.cursorLine >= oldTotalLines-1

		m.rawLogContent = msg.raw
		m.originalLogContent = msg.colored
		m.logLineNumbers = msg.lineNumbers

		newTotalLines := m.totalLogLines()

		// Update cursor position
		if newTotalLines == 0 {
			m.cursorLine = 0
		} else if cursorAtBottom {
			// Keep cursor at bottom if it was at bottom
			m.cursorLine = newTotalLines - 1
		} else if m.cursorLine >= newTotalLines {
			// Clamp cursor if content shrunk
			m.cursorLine = newTotalLines - 1
		}

		// Exit visual mode when content changes (selection would be invalid)
		if m.visualMode && oldTotalLines != newTotalLines {
			m.visualMode = false
		}

		atBottom := m.logViewport.AtBottom()
		offset := m.logViewport.YOffset

		if m.searchTerm != "" {
			m.applySearchFilter()
		} else {
			m.logViewport.SetContent(m.wrapLogContent(m.addLineNumbers(msg.colored)))
		}

		// Preserve scroll position when not at bottom
		if atBottom {
			m.logViewport.GotoBottom()
		} else {
			m.logViewport.SetYOffset(offset)
		}
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

	case taskRestartedMsg:
		newID := int64(msg)
		cmds = append(cmds, fetchTasks(m.mgr))
		if newID > 0 {
			// After tasks refresh, select the new task
			cmds = append(cmds, func() tea.Msg {
				// Small delay to let task list update
				time.Sleep(100 * time.Millisecond)
				tasks, _ := m.mgr.ListTasks()
				for i, t := range tasks {
					if int64(t.ID) == newID {
						return selectTaskMsg(i)
					}
				}
				return nil
			})
		}
		return m, tea.Batch(cmds...)

	case selectTaskMsg:
		m.selectedIdx = int(msg)
		m.cursorLine = 0 // Reset cursor when switching tasks
		m.visualMode = false
		if (m.rightMode == modeLog || m.rightMode == modeSplit) && len(m.tasks) > 0 && m.selectedIdx < len(m.tasks) {
			return m, fetchLogs(m.mgr, m.tasks[m.selectedIdx].ID)
		}
		return m, nil

	case clipboardCopiedMsg:
		m.copied = true
		// Reset copied flag after a short delay
		return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
			return clearCopiedMsg{}
		})

	case clearCopiedMsg:
		m.copied = false
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Pass to active components
	if m.searchMode {
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	} else if m.activePane == paneRight && m.rightMode == modeChat {
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

	// Search mode input handling
	if m.searchMode {
		switch key {
		case "enter":
			term := m.searchInput.Value()
			m.searchMode = false
			m.searchInput.Blur()
			if term != "" {
				m.searchTerm = term
				m.matchIndex = 0
				m.applySearchFilter()
			}
			return m, nil
		case "esc":
			m.searchMode = false
			m.searchInput.Blur()
			m.searchInput.SetValue("")
			return m, nil
		default:
			var cmd tea.Cmd
			m.searchInput, cmd = m.searchInput.Update(msg)
			return m, cmd
		}
	}

	// Command mode input handling (for :<line_number>)
	if m.commandMode {
		switch key {
		case "enter":
			cmd := m.commandInput.Value()
			m.commandMode = false
			m.commandInput.Blur()
			// Parse line number and jump to it
			var lineNum int
			if n, err := fmt.Sscanf(cmd, "%d", &lineNum); err == nil && n == 1 {
				// Convert to 0-indexed display line
				targetDisplayLine := -1
				for i, origLineNum := range m.logLineNumbers {
					if origLineNum == lineNum {
						targetDisplayLine = i
						break
					}
					if origLineNum > lineNum {
						// Line number is between available lines, go to previous line
						targetDisplayLine = i - 1
						if targetDisplayLine < 0 {
							targetDisplayLine = 0
						}
						break
					}
				}
				if targetDisplayLine >= 0 {
					m.cursorLine = targetDisplayLine
					m.logViewport.SetContent(m.wrapLogContent(m.addLineNumbers(m.originalLogContent)))
					// Ensure cursor is visible
					if m.cursorLine < m.logViewport.YOffset {
						m.logViewport.SetYOffset(m.cursorLine)
					} else if m.cursorLine >= m.logViewport.YOffset+m.logViewport.Height {
						m.logViewport.SetYOffset(m.cursorLine - m.logViewport.Height + 1)
					}
				}
			}
			return m, nil
		case "esc":
			m.commandMode = false
			m.commandInput.Blur()
			m.commandInput.SetValue("")
			return m, nil
		default:
			var cmd tea.Cmd
			m.commandInput, cmd = m.commandInput.Update(msg)
			return m, cmd
		}
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

		// Allow switching to logs without unfocusing first (ctrl+left arrow)
		if key == "ctrl+left" {
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
			return m, nil
		}

		// Slash picker navigation (check first so tab completion works)
		if m.showSlashPicker() {
			filtered := m.filteredSlashCommands()
			if len(filtered) > 0 {
				switch key {
				case "up":
					m.slashPickerIdx--
					if m.slashPickerIdx < 0 {
						m.slashPickerIdx = len(filtered) - 1
					}
					return m, nil
				case "down":
					m.slashPickerIdx++
					if m.slashPickerIdx >= len(filtered) {
						m.slashPickerIdx = 0
					}
					return m, nil
				case "tab":
					// Complete the selected command
					m.chatInput.Reset()
					m.chatInput.SetValue(filtered[m.slashPickerIdx].name + " ")
					m.slashPickerIdx = 0
					return m, nil
				}
			}
		}

		// Tab to switch panes from chat input (only if slash picker not showing)
		if key == "tab" {
			m.activePane = paneLeft
			m.chatInput.Blur()
			return m, nil
		}

		if key == "enter" && !m.agentBusy {
			text := m.chatInput.Value()
			if text != "" {
				m.chatInput.Reset()

				// Handle slash commands
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

				if strings.HasPrefix(text, "/save") {
					m.handleSaveCommand(text)
					m.updateChatViewport()
					return m, nil
				}

				if text == "/new" {
					m.chatHistory = nil
					m.conversation = m.agent.NewConversation()
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
		m.slashPickerIdx = 0
		return m, cmd
	}

	switch key {
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		// Build pending count for numbered motions (e.g., "5j" to move down 5 lines)
		var digit int
		fmt.Sscanf(key, "%d", &digit)
		m.pendingCount = m.pendingCount*10 + digit
		return m, nil
	case "0":
		// 0 without pending count = go to start of line (horizontal)
		if m.pendingCount == 0 {
			if m.activePane == paneRight && (m.rightMode == modeLog || m.rightMode == modeSplit) && m.rawLogContent != "" {
				m.logXOffset = 0
				m.logViewport.SetContent(m.wrapLogContent(m.addLineNumbers(m.originalLogContent)))
			}
		} else {
			// 0 as part of a number (e.g., "10j")
			m.pendingCount = m.pendingCount * 10
		}
		return m, nil
	case ":":
		// Enter command mode to jump to a specific line
		if m.activePane == paneRight && (m.rightMode == modeLog || m.rightMode == modeSplit) && m.rawLogContent != "" {
			m.commandMode = true
			m.commandInput.SetValue("")
			cmd := m.commandInput.Focus()
			return m, cmd
		}
	case "j", "down":
		count := m.pendingCount
		if count == 0 {
			count = 1
		}
		m.pendingCount = 0
		if m.activePane == paneLeft && len(m.tasks) > 0 {
			m.selectedIdx += count
			if m.selectedIdx >= len(m.tasks) {
				m.selectedIdx = len(m.tasks) - 1
			}
			if m.rightMode == modeLog {
				return m, fetchLogs(m.mgr, m.tasks[m.selectedIdx].ID)
			}
		} else if m.activePane == paneRight {
			if m.rightMode == modeLog || m.rightMode == modeSplit {
				totalLines := m.totalLogLines()
				if totalLines > 0 && m.cursorLine < totalLines-1 {
					m.cursorLine += count
					if m.cursorLine >= totalLines {
						m.cursorLine = totalLines - 1
					}
					// Set content first
					m.logViewport.SetContent(m.wrapLogContent(m.addLineNumbers(m.originalLogContent)))
					// Ensure cursor is visible
					if m.cursorLine >= m.logViewport.YOffset+m.logViewport.Height {
						m.logViewport.SetYOffset(m.cursorLine - m.logViewport.Height + 1)
					}
				}
			} else {
				for i := 0; i < count; i++ {
					m.chatViewport.LineDown(1)
				}
			}
		}
	case "k", "up":
		count := m.pendingCount
		if count == 0 {
			count = 1
		}
		m.pendingCount = 0
		if m.activePane == paneLeft && len(m.tasks) > 0 {
			m.selectedIdx -= count
			if m.selectedIdx < 0 {
				m.selectedIdx = 0
			}
			if m.rightMode == modeLog {
				return m, fetchLogs(m.mgr, m.tasks[m.selectedIdx].ID)
			}
		} else if m.activePane == paneRight {
			if m.rightMode == modeLog || m.rightMode == modeSplit {
				if m.cursorLine > 0 {
					m.cursorLine -= count
					if m.cursorLine < 0 {
						m.cursorLine = 0
					}
					// Set content first
					m.logViewport.SetContent(m.wrapLogContent(m.addLineNumbers(m.originalLogContent)))
					// Ensure cursor is visible
					if m.cursorLine < m.logViewport.YOffset {
						m.logViewport.SetYOffset(m.cursorLine)
					}
				}
			} else {
				m.chatViewport.LineUp(1)
			}
		}
	case "g":
		if m.activePane == paneRight && (m.rightMode == modeLog || m.rightMode == modeSplit) {
			m.cursorLine = 0
			m.logViewport.SetContent(m.wrapLogContent(m.addLineNumbers(m.originalLogContent)))
			m.logViewport.GotoTop()
		}
	case "G":
		if m.activePane == paneRight && (m.rightMode == modeLog || m.rightMode == modeSplit) {
			totalLines := m.totalLogLines()
			if totalLines > 0 {
				m.cursorLine = totalLines - 1
			}
			m.logViewport.SetContent(m.wrapLogContent(m.addLineNumbers(m.originalLogContent)))
			m.logViewport.GotoBottom()
		}
	case "^":
		// Scroll horizontally to start of line
		if m.activePane == paneRight && (m.rightMode == modeLog || m.rightMode == modeSplit) && m.rawLogContent != "" {
			m.logXOffset = 0
			m.logViewport.SetContent(m.wrapLogContent(m.addLineNumbers(m.originalLogContent)))
		}
	case ">":
		// Scroll right by 20 chars
		if m.activePane == paneRight && (m.rightMode == modeLog || m.rightMode == modeSplit) && m.rawLogContent != "" {
			maxLen := m.maxLineLength()
			lineNumWidth := len(fmt.Sprintf("%d", m.totalLogLines()))
			prefixWidth := lineNumWidth + 3
			contentWidth := m.logViewport.Width - prefixWidth
			if contentWidth <= 0 {
				contentWidth = 80
			}
			maxOffset := maxLen - contentWidth
			if maxOffset < 0 {
				maxOffset = 0
			}
			m.logXOffset += 20
			if m.logXOffset > maxOffset {
				m.logXOffset = maxOffset
			}
			m.logViewport.SetContent(m.wrapLogContent(m.addLineNumbers(m.originalLogContent)))
		}
	case "<":
		// Scroll left by 20 chars
		if m.activePane == paneRight && (m.rightMode == modeLog || m.rightMode == modeSplit) && m.rawLogContent != "" {
			if m.logXOffset >= 20 {
				m.logXOffset -= 20
			} else {
				m.logXOffset = 0
			}
			m.logViewport.SetContent(m.wrapLogContent(m.addLineNumbers(m.originalLogContent)))
		}
	case "tab":
		// Cycle: sidebar → logs → chat → sidebar
		// When sidebar hidden: logs → chat → logs
		if m.leftHidden {
			// Sidebar hidden: toggle between logs and chat
			if m.rightMode == modeLog {
				m.rightMode = modeChat
				m.chatInput.Focus()
			} else {
				m.rightMode = modeLog
				m.chatInput.Blur()
			}
		} else {
			// Sidebar visible: cycle through all three
			if m.activePane == paneLeft {
				m.activePane = paneRight
				m.rightMode = modeLog
				m.chatInput.Blur()
			} else if m.rightMode == modeLog {
				m.rightMode = modeChat
				m.chatInput.Focus()
			} else {
				// In chat, go back to sidebar
				m.activePane = paneLeft
				m.chatInput.Blur()
			}
		}
	case "t":
		m.themeIdx = (m.themeIdx + 1) % len(themes)
		m.cfg.Theme = themes[m.themeIdx].name
		m.cfg.Save()
	case "m":
		// Toggle light/dark mode
		m.lightMode = !m.lightMode
		logcolor.SetLightMode(m.lightMode)
	case "h":
		m.leftHidden = !m.leftHidden
		m.recalcLayout()
	case "l":
		if m.rightMode == modeSplit {
			m.rightMode = modeLog
		} else {
			m.rightMode = modeLog
		}
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
		m.recalcLayout()
	case "c":
		m.rightMode = modeChat
		m.activePane = paneRight
		m.chatInput.Focus()
		m.recalcLayout()
	case "s":
		if m.rightMode == modeSplit {
			m.rightMode = modeLog
		} else {
			m.rightMode = modeSplit
		}
		m.chatInput.Blur()
		m.recalcLayout()
	case "enter":
		if m.activePane == paneLeft && len(m.tasks) > 0 && m.selectedIdx < len(m.tasks) {
			// Open logs for selected task
			m.rightMode = modeLog
			m.activePane = paneRight
			return m, fetchLogs(m.mgr, m.tasks[m.selectedIdx].ID)
		} else if m.activePane == paneRight && m.rightMode == modeLog && !m.chatInput.Focused() {
			// Maximize logs (toggle sidebar)
			m.leftHidden = !m.leftHidden
			m.recalcLayout()
		} else if m.activePane == paneRight && m.rightMode == modeChat && !m.chatInput.Focused() {
			// Expand sidebar if hidden, then focus input
			if m.leftHidden {
				m.leftHidden = false
				m.recalcLayout()
			}
			m.chatInput.Focus()
		}
	case "x":
		if len(m.tasks) > 0 && m.selectedIdx < len(m.tasks) {
			t := m.tasks[m.selectedIdx]
			if t.Status == "running" {
				return m, stopTask(m.mgr, t.ID)
			}
		}
	case "r":
		if m.activePane == paneLeft && len(m.tasks) > 0 && m.selectedIdx < len(m.tasks) {
			t := m.tasks[m.selectedIdx]
			if t.Status == "stopped" || t.Status == "crashed" {
				return m, restartTaskCmd(m.mgr, t.ID)
			}
		}
	case "v":
		if m.activePane == paneRight && (m.rightMode == modeLog || m.rightMode == modeSplit) && m.rawLogContent != "" {
			m.visualMode = !m.visualMode
			if m.visualMode {
				// Start visual selection at cursor position
				m.visualStart = m.cursorLine
			}
			m.refreshLogContent()
			return m, nil
		}
	case "y":
		if m.activePane == paneRight && (m.rightMode == modeLog || m.rightMode == modeSplit) && m.rawLogContent != "" {
			if m.visualMode {
				// Copy selected lines
				selected := m.getSelectedLines()
				m.visualMode = false
				m.refreshLogContent()
				if selected != "" {
					return m, copyToClipboard(selected)
				}
				return m, nil
			}
			return m, copyToClipboard(m.rawLogContent)
		}
	case "/":
		if m.activePane == paneRight && (m.rightMode == modeLog || m.rightMode == modeSplit) {
			m.searchMode = true
			m.searchInput.SetValue("")
			cmd := m.searchInput.Focus()
			return m, cmd
		}
	case "n":
		if m.activePane == paneRight && (m.rightMode == modeLog || m.rightMode == modeSplit) && m.searchTerm != "" && len(m.searchMatches) > 0 {
			m.matchIndex = (m.matchIndex + 1) % len(m.searchMatches)
			m.scrollToMatch()
		}
	case "N":
		if m.activePane == paneRight && (m.rightMode == modeLog || m.rightMode == modeSplit) && m.searchTerm != "" && len(m.searchMatches) > 0 {
			m.matchIndex--
			if m.matchIndex < 0 {
				m.matchIndex = len(m.searchMatches) - 1
			}
			m.scrollToMatch()
		}
	case "esc":
		if m.visualMode {
			m.visualMode = false
			m.refreshLogContent()
			return m, nil
		}
		if m.searchTerm != "" {
			m.searchTerm = ""
			m.searchMatches = nil
			m.matchIndex = 0
			m.logViewport.SetContent(m.wrapLogContent(m.addLineNumbers(m.originalLogContent)))
			return m, nil
		}
		// Restore sidebar if hidden
		if m.leftHidden {
			m.leftHidden = false
			m.recalcLayout()
			return m, nil
		}
	}

	return m, nil
}

// showSlashPicker returns true when the input starts with "/" and hasn't been completed yet
func (m Model) showSlashPicker() bool {
	val := m.chatInput.Value()
	return strings.HasPrefix(val, "/") && !strings.Contains(val, " ")
}

// filteredSlashCommands returns commands matching the current input prefix
func (m Model) filteredSlashCommands() []slashCommand {
	val := m.chatInput.Value()
	var result []slashCommand
	for _, cmd := range slashCommands {
		if strings.HasPrefix(cmd.name, val) {
			result = append(result, cmd)
		}
	}
	return result
}

func (m *Model) recalcLayout() {
	// Store reactive dimensions in model
	m.boxHeight = m.height - 2 // account for status bar (1) + newline (1)
	m.innerHeight = m.boxHeight - 3 // subtract border top/bottom (2) + title (1)

	if m.leftHidden {
		m.leftWidth = 0
		m.rightWidth = m.width - 2 // just borders
	} else {
		m.leftWidth = m.width * 30 / 100
		m.rightWidth = m.width - m.leftWidth - 3 // borders
	}

	if m.rightMode == modeSplit {
		// Split mode: chat and logs side by side
		splitWidth := m.rightWidth/2 - 1 // divide and account for border
		chatInputHeight := 3

		m.logViewport.Width = splitWidth
		m.logViewport.Height = m.innerHeight

		m.chatViewport.Width = splitWidth
		m.chatViewport.Height = m.innerHeight - chatInputHeight - 1
		m.chatInput.SetWidth(splitWidth)
	} else {
		// Single pane mode (log or chat)
		m.logViewport.Width = m.rightWidth
		m.logViewport.Height = m.innerHeight

		chatInputHeight := 3
		m.chatViewport.Width = m.rightWidth
		m.chatViewport.Height = m.innerHeight - chatInputHeight - 1
		m.chatInput.SetWidth(m.rightWidth)
	}
}

func (m *Model) handleSaveCommand(text string) {
	parts := strings.Fields(text)

	if len(parts) < 2 {
		m.chatHistory = append(m.chatHistory, chatMessage{
			role: "agent", content: "usage: /save <name> [command]\n  /save <name> <command>  save a specific command\n  /save <name>            save the last command the agent started",
		})
		return
	}

	name := parts[1]

	if len(parts) >= 3 {
		// /save <name> <command...>
		command := strings.Join(parts[2:], " ")
		if err := m.tickStore.Save(name, command, ""); err != nil {
			m.chatHistory = append(m.chatHistory, chatMessage{
				role: "agent", content: fmt.Sprintf("error: %s", err),
			})
			return
		}
		m.chatHistory = append(m.chatHistory, chatMessage{
			role: "agent", content: fmt.Sprintf("saved tick %q: %s", name, command),
		})
		return
	}

	// /save <name> - find the last start_task from chat history
	command := m.findLastStartTaskCommand()
	if command == "" {
		m.chatHistory = append(m.chatHistory, chatMessage{
			role: "agent", content: "no start_task found in chat history",
		})
		return
	}

	if err := m.tickStore.Save(name, command, ""); err != nil {
		m.chatHistory = append(m.chatHistory, chatMessage{
			role: "agent", content: fmt.Sprintf("error: %s", err),
		})
		return
	}
	m.chatHistory = append(m.chatHistory, chatMessage{
		role: "agent", content: fmt.Sprintf("saved tick %q: %s", name, command),
	})
}

// findLastStartTaskCommand scans chat history backwards for the last start_task tool call
// and extracts the command from its JSON args.
func (m *Model) findLastStartTaskCommand() string {
	for i := len(m.chatHistory) - 1; i >= 0; i-- {
		msg := m.chatHistory[i]
		if msg.role != "tool" {
			continue
		}
		if !strings.HasPrefix(msg.content, "[start_task]") {
			continue
		}
		argsStr := strings.TrimPrefix(msg.content, "[start_task] ")
		var args map[string]interface{}
		if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
			continue
		}
		if cmd, ok := args["command"].(string); ok {
			return cmd
		}
	}
	return ""
}

func (m *Model) applySearchFilter() {
	lines := strings.Split(m.originalLogContent, "\n")
	termLower := strings.ToLower(m.searchTerm)
	m.searchMatches = nil
	m.searchMatchLines = nil
	var filtered []string
	for i, line := range lines {
		if strings.Contains(strings.ToLower(line), termLower) {
			m.searchMatches = append(m.searchMatches, len(filtered))
			// Store the original line number for this match
			origLineNum := i + 1
			if i < len(m.logLineNumbers) {
				origLineNum = m.logLineNumbers[i]
			}
			m.searchMatchLines = append(m.searchMatchLines, origLineNum)
			filtered = append(filtered, line)
		}
	}
	if len(filtered) == 0 {
		m.logViewport.SetContent(fmt.Sprintf("no matches for %q", m.searchTerm))
	} else {
		// Create filtered line numbers for display
		filteredLineNumbers := make([]int, len(filtered))
		for i := range filtered {
			if i < len(m.searchMatchLines) {
				filteredLineNumbers[i] = m.searchMatchLines[i]
			} else {
				filteredLineNumbers[i] = i + 1
			}
		}
		// Temporarily set logLineNumbers to filtered line numbers for addLineNumbers
		oldLineNumbers := m.logLineNumbers
		m.logLineNumbers = filteredLineNumbers
		m.logViewport.SetContent(m.wrapLogContent(m.addLineNumbers(strings.Join(filtered, "\n"))))
		m.logLineNumbers = oldLineNumbers
	}
	if m.matchIndex >= len(m.searchMatches) {
		m.matchIndex = 0
	}
}

func (m *Model) scrollToMatch() {
	if len(m.searchMatches) == 0 {
		return
	}
	line := m.searchMatches[m.matchIndex]
	m.logViewport.SetYOffset(line)
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

// addLineNumbers prepends line numbers to each line of content
func (m *Model) addLineNumbers(content string) string {
	lines := strings.Split(content, "\n")
	lineNumStyle := lipgloss.NewStyle().Foreground(m.dimGrayForMode())
	sepStyle := lipgloss.NewStyle().Foreground(m.dim())
	cursorStyle := lipgloss.NewStyle().Bold(true).Foreground(m.bright())
	selectedStyle := lipgloss.NewStyle().Background(m.bg()).Foreground(m.bright())

	// Calculate width needed for line numbers based on max possible line number
	maxLineNum := len(lines)
	if len(m.logLineNumbers) > 0 {
		for _, ln := range m.logLineNumbers {
			if ln > maxLineNum {
				maxLineNum = ln
			}
		}
	}
	lineNumWidth := len(fmt.Sprintf("%d", maxLineNum))

	// Selection bounds for visual mode (cursorLine is selection end)
	start, end := m.visualStart, m.cursorLine
	if start > end {
		start, end = end, start
	}

	var result strings.Builder
	for i, line := range lines {
		if i > 0 {
			result.WriteString("\n")
		}

		// Get the original line number
		lineNum := i + 1
		if i < len(m.logLineNumbers) {
			lineNum = m.logLineNumbers[i]
		}

		// Render line number with padding
		lineNumStr := fmt.Sprintf("%*d", lineNumWidth, lineNum)

		// Check if in selection (visual mode)
		inSelection := m.visualMode && i >= start && i <= end
		// Check if cursor line (normal mode)
		isCursor := !m.visualMode && i == m.cursorLine

		// Apply horizontal scroll offset while preserving ANSI codes
		displayLine := skipANSI(line, m.logXOffset)

		if inSelection {
			// Strip ANSI codes for clean highlight
			cleanLine := ansiRegex.ReplaceAllString(displayLine, "")
			fullLine := lineNumStr + " │ " + cleanLine
			result.WriteString(selectedStyle.Render(fullLine))
		} else if isCursor {
			// Show cursor with bold line number and arrow
			result.WriteString(cursorStyle.Render(lineNumStr))
			result.WriteString(cursorStyle.Render(" ► "))
			result.WriteString(displayLine)
		} else {
			result.WriteString(lineNumStyle.Render(lineNumStr))
			result.WriteString(sepStyle.Render(" │ "))
			result.WriteString(displayLine)
		}
	}
	return result.String()
}

// skipANSI skips n visible characters while preserving ANSI escape codes
func skipANSI(s string, offset int) string {
	if offset <= 0 {
		return s
	}

	visible := 0
	inEscape := false
	i := 0

	for i < len(s) {
		if s[i] == '\x1b' {
			// Start of ANSI escape sequence
			inEscape = true
			i++
			continue
		}

		if inEscape {
			// Check for end of escape sequence (usually 'm')
			if s[i] >= 'A' && s[i] <= 'Z' {
				inEscape = false
			}
			i++
			continue
		}

		// Visible character
		if visible >= offset {
			return s[i:]
		}
		visible++
		i++
	}

	return ""
}

// ansiRegex matches ANSI escape sequences
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// totalLogLines returns the total number of lines in the raw log content
func (m *Model) totalLogLines() int {
	if m.rawLogContent == "" {
		return 0
	}
	totalLines := strings.Count(m.rawLogContent, "\n")
	if !strings.HasSuffix(m.rawLogContent, "\n") {
		totalLines++
	}
	return totalLines
}

// maxLineLength returns the length of the longest line (ANSI codes stripped)
func (m *Model) maxLineLength() int {
	if m.originalLogContent == "" {
		return 0
	}
	lines := strings.Split(m.originalLogContent, "\n")
	maxLen := 0
	for _, line := range lines {
		plain := ansiRegex.ReplaceAllString(line, "")
		if len(plain) > maxLen {
			maxLen = len(plain)
		}
	}
	return maxLen
}


// refreshLogContent refreshes the log viewport content with current line numbers and selection
func (m *Model) refreshLogContent() {
	// Preserve viewport position when refreshing content
	yOffset := m.logViewport.YOffset
	m.logViewport.SetContent(m.wrapLogContent(m.addLineNumbers(m.originalLogContent)))
	m.logViewport.SetYOffset(yOffset)
}

// getSelectedLines returns the raw log lines within the visual selection
func (m *Model) getSelectedLines() string {
	if !m.visualMode {
		return ""
	}
	lines := strings.Split(m.rawLogContent, "\n")
	start, end := m.visualStart, m.cursorLine
	if start > end {
		start, end = end, start
	}

	if start < 0 {
		start = 0
	}
	if end >= len(lines) {
		end = len(lines) - 1
	}

	if start >= len(lines) {
		return ""
	}

	selected := lines[start : end+1]
	return strings.Join(selected, "\n")
}
