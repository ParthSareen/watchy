package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/parth/watchy/internal/agent"
	"github.com/parth/watchy/internal/logcolor"
	"github.com/parth/watchy/internal/termstyle"
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
			cmds = append(cmds, fetchLogs(m.mgr, m.tasks[m.selectedIdx].ID, m.showLogNoise))
		}
		return m, tea.Batch(cmds...)

	case tasksUpdatedMsg:
		if msg.err != nil {
			return m, m.setStatus("could not refresh tasks: "+msg.err.Error(), true)
		}
		firstLoad := !m.tasksLoaded
		m.tasksLoaded = true
		m.tasks = msg.tasks
		if m.pendingTaskID > 0 {
			for i, task := range m.tasks {
				if int64(task.ID) == m.pendingTaskID {
					m.selectedIdx = i
					m.pendingTaskID = 0
					break
				}
			}
		}
		if m.selectedIdx >= len(m.tasks) && len(m.tasks) > 0 {
			m.selectedIdx = len(m.tasks) - 1
		}
		if len(m.tasks) == 0 {
			m.selectedIdx = 0
		}
		m.conversation.RefreshSystemPrompt()
		if firstLoad && len(m.tasks) > 0 && (m.rightMode == modeLog || m.rightMode == modeSplit) {
			m.logsLoading = true
			return m, fetchLogs(m.mgr, m.tasks[m.selectedIdx].ID, m.showLogNoise)
		}
		return m, nil

	case logContentMsg:
		if len(m.tasks) == 0 || m.selectedIdx >= len(m.tasks) || m.tasks[m.selectedIdx].ID != msg.taskID {
			return m, nil
		}
		m.logsLoading = false
		if msg.err != nil {
			return m, m.setStatus("could not refresh logs: "+msg.err.Error(), true)
		}
		oldTotalLines := m.totalLogLines()
		cursorAtBottom := oldTotalLines > 0 && m.cursorLine >= oldTotalLines-1
		atBottom := m.logViewport.AtBottom()
		offset := m.logViewport.YOffset

		m.allRawLogContent = msg.visibleRaw
		m.allLogContent = msg.colored
		m.allLogLineNumbers = append(m.allLogLineNumbers[:0], msg.lineNumbers...)
		m.hiddenLogNoise = msg.hiddenNoise

		if m.searchTerm != "" {
			m.applySearchFilter()
		} else {
			m.restoreAllLogs()
			m.refreshLogContent()
		}

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

		// Preserve scroll position when not at bottom
		if atBottom {
			m.logViewport.GotoBottom()
		} else {
			m.logViewport.SetYOffset(offset)
		}
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.chat, cmd = m.chat.Update(msg)
		return m, cmd

	case agentToolStartMsg:
		m.appendToolStart(agent.ToolStartEvent(msg))
		return m, nil

	case agentToolResultMsg:
		m.appendToolResult(agent.ToolResultEvent(msg))
		return m, nil

	case agentResponseMsg:
		m.agentBusy = false
		if m.agentCancel != nil {
			m.agentCancel()
		}
		m.agentCancel = nil
		m.chat.SetBusy(false)
		m.appendChatMessage("agent", string(msg))
		return m, nil

	case agentErrorMsg:
		m.agentBusy = false
		if m.agentCancel != nil {
			m.agentCancel()
		}
		m.agentCancel = nil
		m.chat.SetBusy(false)
		if errors.Is(msg.err, context.Canceled) {
			m.appendChatMessage("agent", "Request cancelled.")
		} else {
			m.appendChatMessage("agent", fmt.Sprintf("Error: %s", msg.err))
		}
		return m, nil

	case taskStoppedMsg:
		if msg.err != nil {
			return m, m.setStatus("could not stop task: "+msg.err.Error(), true)
		}
		return m, tea.Batch(fetchTasks(m.mgr), m.setStatus(fmt.Sprintf("stopped task %d", msg.id), false))

	case taskRestartedMsg:
		if msg.err != nil {
			return m, m.setStatus("could not restart task: "+msg.err.Error(), true)
		}
		m.pendingTaskID = msg.id
		return m, tea.Batch(fetchTasks(m.mgr), m.setStatus(fmt.Sprintf("restarted as task %d", msg.id), false))

	case clipboardCopiedMsg:
		m.copied = true
		// Reset copied flag after a short delay
		return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
			return clearCopiedMsg{}
		})

	case clearCopiedMsg:
		m.copied = false
		return m, nil

	case clearStatusMsg:
		if int(msg) == m.statusSeq {
			m.statusMessage = ""
			m.statusError = false
		}
		return m, nil

	case modelsLoadedMsg:
		m.modelPickerLoading = false
		m.modelPickerModels = msg.models
		m.modelPickerErr = msg.err
		m.modelPickerIdx = 0
		for i, model := range m.modelPickerChoices() {
			if model == m.agent.Model() {
				m.modelPickerIdx = i
				break
			}
		}
		return m, nil

	case tea.MouseMsg:
		return m.handleMouse(msg)

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
	} else if m.focusedArea == focusChatInput && m.chat.Focused() {
		var cmd tea.Cmd
		cmd = m.chat.UpdateInput(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if m.modelPicker {
		return m.handleModelPickerKey(msg)
	}
	if m.showHelp {
		if key == "?" || key == "esc" || key == "q" {
			m.showHelp = false
		}
		return m, nil
	}
	if m.showTaskDetails {
		if key == "d" || key == "esc" || key == "enter" || key == "q" {
			m.showTaskDetails = false
		}
		return m, nil
	}

	// Esc cancels in-flight agent request
	if key == "esc" && m.agentBusy && m.agentCancel != nil {
		m.agentCancel()
		m.chat.SetBusy(false)
		return m, m.setStatus("cancelling request…", false)
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
					m.logViewport.SetContent(m.addLineNumbers(m.originalLogContent))
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
		if m.chat.Focused() {
			if key == "ctrl+c" {
				return m, tea.Quit
			}
			// q in chat input is just a character
			return m, m.chat.UpdateInput(msg)
		}
		return m, tea.Quit
	}

	// Chat input handling when focused
	if m.focusedArea == focusChatInput && m.chat.Focused() {
		if key == "esc" {
			return m, m.setFocus(focusChatView)
		}

		// Allow switching to logs without unfocusing first (ctrl+left arrow)
		if key == "ctrl+left" {
			return m, m.showRightMode(modeLog)
		}

		// Slash picker navigation (check first so tab completion works)
		if m.chat.ShowSlashPicker() {
			filtered := m.chat.FilteredSlashCommands()
			if len(filtered) > 0 {
				switch key {
				case "up":
					m.chat.SlashPickerUp()
					return m, nil
				case "down":
					m.chat.SlashPickerDown()
					return m, nil
				case "tab":
					// Complete the selected command
					m.chat.CompleteSlashCommand()
					return m, nil
				}
			}
		}

		// Tab to switch focus from chat input (only if slash picker not showing).
		if key == "tab" || key == "shift+tab" {
			return m, m.cycleFocus(key == "tab")
		}

		if key == "ctrl+j" || key == "shift+enter" {
			m.chat.InsertString("\n")
			return m, nil
		}

		if key == "enter" {
			if m.agentBusy {
				return m, nil
			}
			text := strings.TrimSpace(m.chat.Value())
			if text != "" {
				m.chat.ResetInput()

				// Handle slash commands
				if strings.HasPrefix(text, "/model") {
					parts := strings.Fields(text)
					if len(parts) == 1 {
						return m, m.openModelPicker()
					} else {
						newModel := parts[1]
						m.agent.SetModel(newModel)
						return m, m.setStatus("model set for this session: "+newModel, false)
					}
				}

				if strings.HasPrefix(text, "/save") {
					m.handleSaveCommand(text)
					return m, nil
				}

				if text == "/new" {
					m.chat.Clear()
					m.conversation = m.agent.NewConversation()
					if m.historyStore != nil {
						if err := m.historyStore.Clear(); err != nil {
							return m, m.setStatus("could not clear chat history: "+err.Error(), true)
						}
					}
					return m, nil
				}

				m.appendChatMessage("user", text)
				m.agentBusy = true
				busyCmd := m.chat.SetBusy(true)
				ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				m.agentCancel = cancel
				return m, tea.Batch(busyCmd, sendToAgent(m.conversation, text, ctx, m.programRef.p))
			}
			return m, nil
		}
		return m, m.chat.UpdateInput(msg)
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
			if m.logsFocused() && m.rawLogContent != "" {
				m.logXOffset = 0
				m.logViewport.SetContent(m.addLineNumbers(m.originalLogContent))
			}
		} else {
			// 0 as part of a number (e.g., "10j")
			m.pendingCount = m.pendingCount * 10
		}
		return m, nil
	case ":":
		// Enter command mode to jump to a specific line
		if m.logsFocused() && m.rawLogContent != "" {
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
		if m.focusedArea == focusTasks && len(m.tasks) > 0 {
			m.selectedIdx += count
			if m.selectedIdx >= len(m.tasks) {
				m.selectedIdx = len(m.tasks) - 1
			}
			if m.rightMode == modeLog {
				return m, fetchLogs(m.mgr, m.tasks[m.selectedIdx].ID, m.showLogNoise)
			}
		} else if m.logsFocused() {
			totalLines := m.totalLogLines()
			if totalLines > 0 && m.cursorLine < totalLines-1 {
				m.cursorLine += count
				if m.cursorLine >= totalLines {
					m.cursorLine = totalLines - 1
				}
				// Set content first
				m.logViewport.SetContent(m.addLineNumbers(m.originalLogContent))
				// Ensure cursor is visible
				if m.cursorLine >= m.logViewport.YOffset+m.logViewport.Height {
					m.logViewport.SetYOffset(m.cursorLine - m.logViewport.Height + 1)
				}
			}
		} else if m.chatViewFocused() {
			m.chat.ScrollDown(count)
		}
	case "k", "up":
		count := m.pendingCount
		if count == 0 {
			count = 1
		}
		m.pendingCount = 0
		if m.focusedArea == focusTasks && len(m.tasks) > 0 {
			m.selectedIdx -= count
			if m.selectedIdx < 0 {
				m.selectedIdx = 0
			}
			if m.rightMode == modeLog {
				return m, fetchLogs(m.mgr, m.tasks[m.selectedIdx].ID, m.showLogNoise)
			}
		} else if m.logsFocused() {
			if m.cursorLine > 0 {
				m.cursorLine -= count
				if m.cursorLine < 0 {
					m.cursorLine = 0
				}
				// Set content first
				m.logViewport.SetContent(m.addLineNumbers(m.originalLogContent))
				// Ensure cursor is visible
				if m.cursorLine < m.logViewport.YOffset {
					m.logViewport.SetYOffset(m.cursorLine)
				}
			}
		} else if m.chatViewFocused() {
			m.chat.ScrollUp(count)
		}
	case "g":
		if m.logsFocused() {
			m.cursorLine = 0
			m.logViewport.SetContent(m.addLineNumbers(m.originalLogContent))
			m.logViewport.GotoTop()
		} else if m.chatViewFocused() {
			m.chat.GotoTop()
		}
	case "G":
		if m.logsFocused() {
			totalLines := m.totalLogLines()
			if totalLines > 0 {
				m.cursorLine = totalLines - 1
			}
			m.logViewport.SetContent(m.addLineNumbers(m.originalLogContent))
			m.logViewport.GotoBottom()
		} else if m.chatViewFocused() {
			m.chat.GotoBottom()
		}
	case "^":
		// Scroll horizontally to start of line
		if m.logsFocused() && m.rawLogContent != "" {
			m.logXOffset = 0
			m.logViewport.SetContent(m.addLineNumbers(m.originalLogContent))
		}
	case ">":
		// Scroll right by 20 chars
		if m.logsFocused() && m.rawLogContent != "" {
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
			m.logViewport.SetContent(m.addLineNumbers(m.originalLogContent))
		}
	case "<":
		// Scroll left by 20 chars
		if m.logsFocused() && m.rawLogContent != "" {
			if m.logXOffset >= 20 {
				m.logXOffset -= 20
			} else {
				m.logXOffset = 0
			}
			m.logViewport.SetContent(m.addLineNumbers(m.originalLogContent))
		}
	case "tab", "shift+tab":
		return m, m.cycleFocus(key == "tab")
	case "[", "]":
		return m, m.cycleRightMode(key == "]")
	case "?":
		m.showHelp = true
		return m, nil
	case "d":
		if m.focusedArea == focusTasks && len(m.tasks) > 0 {
			m.showTaskDetails = true
		}
		return m, nil
	case "M":
		return m, m.openModelPicker()
	case "t":
		m.themeIdx = (m.themeIdx + 1) % len(themes)
		if m.cfg != nil {
			previous := m.cfg.Theme
			m.cfg.Theme = themes[m.themeIdx].name
			if err := m.cfg.Save(); err != nil {
				m.cfg.Theme = previous
				m.syncChatPalette()
				return m, m.setStatus("theme changed for this session; could not save: "+err.Error(), true)
			}
		}
		m.syncChatPalette()
	case "m":
		// Toggle manual light/dark mode.
		m.lightMode = !m.lightMode
		termstyle.ApplyLightMode(m.lightMode)
		logcolor.SetLightMode(m.lightMode)
		if m.cfg != nil {
			previous := m.cfg.ColorMode
			if m.lightMode {
				m.cfg.ColorMode = termstyle.ColorModeLight
			} else {
				m.cfg.ColorMode = termstyle.ColorModeDark
			}
			if err := m.cfg.Save(); err != nil {
				m.cfg.ColorMode = previous
				m.syncChatPalette()
				return m, m.setStatus("color mode changed for this session; could not save: "+err.Error(), true)
			}
		}
		m.syncChatPalette()
	case "u":
		if m.logsFocused() {
			m.showLogNoise = !m.showLogNoise
			if len(m.tasks) > 0 && m.selectedIdx < len(m.tasks) {
				return m, fetchLogs(m.mgr, m.tasks[m.selectedIdx].ID, m.showLogNoise)
			}
		}
	case "h":
		m.leftHidden = !m.leftHidden
		if m.leftHidden && m.focusedArea == focusTasks {
			m.setFocus(focusLogs)
		}
		m.recalcLayout()
	case "l":
		return m, m.showRightMode(modeLog)
	case "c":
		return m, m.showRightMode(modeChat)
	case "s":
		if m.rightMode == modeSplit {
			return m, m.showRightMode(modeLog)
		}
		return m, m.showRightMode(modeSplit)
	case "enter":
		if m.focusedArea == focusTasks && len(m.tasks) > 0 && m.selectedIdx < len(m.tasks) {
			// Open logs for selected task
			return m, m.showRightMode(modeLog)
		} else if m.chatViewFocused() {
			return m, m.setFocus(focusChatInput)
		}
	case "i":
		if m.chatViewFocused() {
			return m, m.setFocus(focusChatInput)
		}
	case "e":
		if m.chatViewFocused() {
			m.chat.ToggleLastTool()
			return m, nil
		}
	case "x":
		if m.focusedArea == focusTasks && len(m.tasks) > 0 && m.selectedIdx < len(m.tasks) {
			t := m.tasks[m.selectedIdx]
			if t.Status == "running" {
				return m, stopTask(m.mgr, t.ID)
			}
		}
	case "r":
		if m.focusedArea == focusTasks && len(m.tasks) > 0 && m.selectedIdx < len(m.tasks) {
			t := m.tasks[m.selectedIdx]
			if t.Status == "stopped" || t.Status == "crashed" {
				return m, restartTaskCmd(m.mgr, t.ID)
			}
		}
	case "v":
		if m.logsFocused() && m.rawLogContent != "" {
			m.visualMode = !m.visualMode
			if m.visualMode {
				// Start visual selection at cursor position
				m.visualStart = m.cursorLine
			}
			m.refreshLogContent()
			return m, nil
		}
	case "y":
		if m.logsFocused() && m.rawLogContent != "" {
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
		if m.chatViewFocused() {
			if text := m.chat.LastToolText(); text != "" {
				return m, copyToClipboard(text)
			}
		}
	case "/":
		if m.logsFocused() {
			m.searchMode = true
			m.searchInput.SetValue("")
			cmd := m.searchInput.Focus()
			return m, cmd
		}
	case "n":
		if m.logsFocused() && m.searchTerm != "" && len(m.searchMatches) > 0 {
			m.matchIndex = (m.matchIndex + 1) % len(m.searchMatches)
			m.scrollToMatch()
		}
	case "N":
		if m.logsFocused() && m.searchTerm != "" && len(m.searchMatches) > 0 {
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
			m.searchMatchLines = nil
			m.matchIndex = 0
			m.restoreAllLogs()
			m.refreshLogContent()
			return m, nil
		}
		if m.focusedArea == focusChatView {
			if !m.leftHidden {
				return m, m.setFocus(focusTasks)
			}
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
