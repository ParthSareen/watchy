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
		vis := m.visibleTasks()
		if len(vis) > 0 && m.selectedIdx < len(vis) && (m.rightMode == modeLog || m.rightMode == modeSplit) {
			cmds = append(cmds, fetchLogs(m.mgr, vis[m.selectedIdx].ID, m.logs.showLogNoise))
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
			for i, task := range m.visibleTasks() {
				if int64(task.ID) == m.pendingTaskID {
					m.selectedIdx = i
					m.pendingTaskID = 0
					break
				}
			}
		}
		m.clampSelection()
		m.conversation.RefreshSystemPrompt()
		if firstLoad {
			vis := m.visibleTasks()
			if len(vis) > 0 && (m.rightMode == modeLog || m.rightMode == modeSplit) {
				m.logs.logsLoading = true
				return m, fetchLogs(m.mgr, vis[m.selectedIdx].ID, m.logs.showLogNoise)
			}
		}
		return m, nil

	case logContentMsg:
		vis := m.visibleTasks()
		if len(vis) == 0 || m.selectedIdx >= len(vis) || vis[m.selectedIdx].ID != msg.taskID {
			return m, nil
		}
		m.logs.logsLoading = false
		if msg.err != nil {
			return m, m.setStatus("could not refresh logs: "+msg.err.Error(), true)
		}
		m.logs.SetContent(msg.colored, msg.visibleRaw, msg.lineNumbers, msg.hiddenNoise)
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
	if m.logs.searchMode {
		var cmd tea.Cmd
		m.logs.searchInput, cmd = m.logs.searchInput.Update(msg)
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
	if m.logs.searchMode {
		switch key {
		case "enter":
			term := m.logs.searchInput.Value()
			m.logs.searchMode = false
			m.logs.searchInput.Blur()
			if term != "" {
				m.logs.searchTerm = term
				m.logs.matchIndex = 0
				m.logs.applySearchFilter()
			}
			return m, nil
		case "esc":
			m.logs.searchMode = false
			m.logs.searchInput.Blur()
			m.logs.searchInput.SetValue("")
			return m, nil
		default:
			var cmd tea.Cmd
			m.logs.searchInput, cmd = m.logs.searchInput.Update(msg)
			return m, cmd
		}
	}

	// Command mode input handling (for :<line_number>)
	if m.logs.commandMode {
		switch key {
		case "enter":
			cmd := m.logs.commandInput.Value()
			m.logs.commandMode = false
			m.logs.commandInput.Blur()
			var lineNum int
			if n, err := fmt.Sscanf(cmd, "%d", &lineNum); err != nil || n != 1 {
				return m, m.setStatus("invalid line number", true)
			}
			// Convert to 0-indexed display line
			targetDisplayLine := -1
			for i, origLineNum := range m.logs.logLineNumbers {
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
			if targetDisplayLine < 0 {
				last := 0
				if len(m.logs.logLineNumbers) > 0 {
					last = m.logs.logLineNumbers[len(m.logs.logLineNumbers)-1]
				}
				return m, m.setStatus(fmt.Sprintf("line %d not found (last: %d)", lineNum, last), true)
			}
			m.logs.cursorLine = targetDisplayLine
			m.logs.refreshLogContent()
			// Ensure cursor is visible
			if m.logs.cursorLine < m.logs.logViewport.YOffset {
				m.logs.logViewport.SetYOffset(m.logs.cursorLine)
			} else if m.logs.cursorLine >= m.logs.logViewport.YOffset+m.logs.logViewport.Height {
				m.logs.logViewport.SetYOffset(m.logs.cursorLine - m.logs.logViewport.Height + 1)
			}
			return m, nil
		case "esc":
			m.logs.commandMode = false
			m.logs.commandInput.Blur()
			m.logs.commandInput.SetValue("")
			return m, nil
		default:
			var cmd tea.Cmd
			m.logs.commandInput, cmd = m.logs.commandInput.Update(msg)
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
			if m.logsFocused() && m.logs.rawLogContent != "" {
				m.logs.logXOffset = 0
				m.logs.refreshLogContent()
			}
		} else {
			// 0 as part of a number (e.g., "10j")
			m.pendingCount = m.pendingCount * 10
		}
		return m, nil
	case ":":
		// Enter command mode to jump to a specific line
		if m.logsFocused() && m.logs.rawLogContent != "" {
			m.logs.commandMode = true
			m.logs.commandInput.SetValue("")
			cmd := m.logs.commandInput.Focus()
			return m, cmd
		}
	case "j", "down":
		count := m.pendingCount
		if count == 0 {
			count = 1
		}
		m.pendingCount = 0
		if m.focusedArea == focusTasks && len(m.visibleTasks()) > 0 {
			vis := m.visibleTasks()
			m.selectedIdx += count
			if m.selectedIdx >= len(vis) {
				m.selectedIdx = len(vis) - 1
			}
			if m.rightMode == modeLog {
				return m, fetchLogs(m.mgr, vis[m.selectedIdx].ID, m.logs.showLogNoise)
			}
		} else if m.logsFocused() {
			totalLines := m.logs.totalLogLines()
			if totalLines > 0 && m.logs.cursorLine < totalLines-1 {
				m.logs.cursorLine += count
				if m.logs.cursorLine >= totalLines {
					m.logs.cursorLine = totalLines - 1
				}
				// Set content first
				m.logs.refreshLogContent()
				// Ensure cursor is visible
				if m.logs.cursorLine >= m.logs.logViewport.YOffset+m.logs.logViewport.Height {
					m.logs.logViewport.SetYOffset(m.logs.cursorLine - m.logs.logViewport.Height + 1)
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
		if m.focusedArea == focusTasks && len(m.visibleTasks()) > 0 {
			vis := m.visibleTasks()
			m.selectedIdx -= count
			if m.selectedIdx < 0 {
				m.selectedIdx = 0
			}
			if m.rightMode == modeLog {
				return m, fetchLogs(m.mgr, vis[m.selectedIdx].ID, m.logs.showLogNoise)
			}
		} else if m.logsFocused() {
			if m.logs.cursorLine > 0 {
				m.logs.cursorLine -= count
				if m.logs.cursorLine < 0 {
					m.logs.cursorLine = 0
				}
				// Set content first
				m.logs.refreshLogContent()
				// Ensure cursor is visible
				if m.logs.cursorLine < m.logs.logViewport.YOffset {
					m.logs.logViewport.SetYOffset(m.logs.cursorLine)
				}
			}
		} else if m.chatViewFocused() {
			m.chat.ScrollUp(count)
		}
	case "g":
		if m.logsFocused() {
			m.logs.cursorLine = 0
			m.logs.refreshLogContent()
			m.logs.logViewport.GotoTop()
		} else if m.chatViewFocused() {
			m.chat.GotoTop()
		}
	case "G":
		if m.logsFocused() {
			totalLines := m.logs.totalLogLines()
			if totalLines > 0 {
				m.logs.cursorLine = totalLines - 1
			}
			m.logs.refreshLogContent()
			m.logs.logViewport.GotoBottom()
		} else if m.chatViewFocused() {
			m.chat.GotoBottom()
		}
	case "^":
		// Scroll horizontally to start of line
		if m.logsFocused() && m.logs.rawLogContent != "" {
			m.logs.logXOffset = 0
			m.logs.refreshLogContent()
		}
	case ">":
		// Scroll right by 20 chars
		if m.logsFocused() && m.logs.rawLogContent != "" {
			maxLen := m.logs.maxLineLength()
			lineNumWidth := len(fmt.Sprintf("%d", m.logs.totalLogLines()))
			prefixWidth := lineNumWidth + 3
			contentWidth := m.logs.logViewport.Width - prefixWidth
			if contentWidth <= 0 {
				contentWidth = 80
			}
			maxOffset := maxLen - contentWidth
			if maxOffset < 0 {
				maxOffset = 0
			}
			m.logs.logXOffset += 20
			if m.logs.logXOffset > maxOffset {
				m.logs.logXOffset = maxOffset
			}
			m.logs.refreshLogContent()
		}
	case "<":
		// Scroll left by 20 chars
		if m.logsFocused() && m.logs.rawLogContent != "" {
			if m.logs.logXOffset >= 20 {
				m.logs.logXOffset -= 20
			} else {
				m.logs.logXOffset = 0
			}
			m.logs.refreshLogContent()
		}
	case "tab", "shift+tab":
		return m, m.cycleFocus(key == "tab")
	case "[", "]":
		return m, m.cycleRightMode(key == "]")
	case "?":
		m.showHelp = true
		return m, nil
	case "d":
		if m.focusedArea == focusTasks && len(m.visibleTasks()) > 0 {
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
		return m, m.setStatus("theme: "+themes[m.themeIdx].name, false)
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
		colorMode := "dark"
		if m.lightMode {
			colorMode = "light"
		}
		return m, m.setStatus("color mode: "+colorMode, false)
	case "u":
		if m.logsFocused() {
			m.logs.showLogNoise = !m.logs.showLogNoise
			noiseMsg := "noise shown"
			if !m.logs.showLogNoise {
				noiseMsg = "noise hidden"
			}
			vis := m.visibleTasks()
			if len(vis) > 0 && m.selectedIdx < len(vis) {
				return m, tea.Batch(fetchLogs(m.mgr, vis[m.selectedIdx].ID, m.logs.showLogNoise), m.setStatus(noiseMsg, false))
			}
			return m, m.setStatus(noiseMsg, false)
		}
	case "f":
		// Toggle running-only filter in the task sidebar
		if m.focusedArea == focusTasks {
			m.filterRunning = !m.filterRunning
			m.clampSelection()
			if m.filterRunning {
				return m, m.setStatus("filter: running only", false)
			}
			return m, m.setStatus("filter: off", false)
		}
	case "h":
		m.toggleTaskSidebar()
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
		if m.focusedArea == focusTasks && len(m.visibleTasks()) > 0 && m.selectedIdx < len(m.visibleTasks()) {
			// Open logs for selected task
			return m, m.showRightMode(modeLog)
		} else if m.logsFocused() && m.rightMode == modeLog {
			m.toggleTaskSidebar()
			return m, nil
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
		if m.focusedArea != focusTasks {
			return m, m.setStatus("focus the task list (tab)", true)
		}
		vis := m.visibleTasks()
		if len(vis) == 0 || m.selectedIdx >= len(vis) {
			return m, nil
		}
		t := vis[m.selectedIdx]
		if t.Status != "running" {
			return m, m.setStatus(fmt.Sprintf("task %d isn't running", t.ID), true)
		}
		return m, stopTask(m.mgr, t.ID)
	case "r":
		if m.focusedArea != focusTasks {
			return m, m.setStatus("focus the task list (tab)", true)
		}
		vis := m.visibleTasks()
		if len(vis) == 0 || m.selectedIdx >= len(vis) {
			return m, nil
		}
		t := vis[m.selectedIdx]
		if t.Status != "stopped" && t.Status != "crashed" {
			return m, m.setStatus(fmt.Sprintf("task %d is running", t.ID), true)
		}
		return m, restartTaskCmd(m.mgr, t.ID)
	case "v":
		if m.logsFocused() && m.logs.rawLogContent != "" {
			m.logs.visualMode = !m.logs.visualMode
			if m.logs.visualMode {
				// Start visual selection at cursor position
				m.logs.visualStart = m.logs.cursorLine
			}
			m.logs.refreshLogContent()
			return m, nil
		}
	case "y":
		if m.logsFocused() && m.logs.rawLogContent != "" {
			if m.logs.visualMode {
				// Copy selected lines
				selected := m.logs.getSelectedLines()
				m.logs.visualMode = false
				m.logs.refreshLogContent()
				if selected != "" {
					return m, copyToClipboard(selected)
				}
				return m, nil
			}
			return m, copyToClipboard(m.logs.rawLogContent)
		}
		if m.chatViewFocused() {
			if text := m.chat.LastToolText(); text != "" {
				return m, copyToClipboard(text)
			}
		}
	case "/":
		if m.logsFocused() {
			m.logs.searchMode = true
			m.logs.searchInput.SetValue("")
			cmd := m.logs.searchInput.Focus()
			return m, cmd
		}
	case "n":
		if !m.logsFocused() {
			return m, nil
		}
		if m.logs.searchTerm == "" || len(m.logs.searchMatches) == 0 {
			return m, m.setStatus("no active search", true)
		}
		m.logs.matchIndex = (m.logs.matchIndex + 1) % len(m.logs.searchMatches)
		m.logs.scrollToMatch()
	case "N":
		if !m.logsFocused() {
			return m, nil
		}
		if m.logs.searchTerm == "" || len(m.logs.searchMatches) == 0 {
			return m, m.setStatus("no active search", true)
		}
		m.logs.matchIndex--
		if m.logs.matchIndex < 0 {
			m.logs.matchIndex = len(m.logs.searchMatches) - 1
		}
		m.logs.scrollToMatch()
	case "esc":
		m.pendingCount = 0
		if m.logs.visualMode {
			m.logs.visualMode = false
			m.logs.refreshLogContent()
			return m, nil
		}
		if m.logs.searchTerm != "" {
			m.logs.searchTerm = ""
			m.logs.searchMatches = nil
			m.logs.searchMatchLines = nil
			m.logs.matchIndex = 0
			m.logs.restoreAllLogs()
			m.logs.refreshLogContent()
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
