package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/parth/watchy/internal/agent"
)

type chatEntryKind int

const (
	chatEntryUser chatEntryKind = iota
	chatEntryAssistant
	chatEntryTool
	chatEntrySystem
	chatEntryError
)

type toolStatus int

const (
	toolStatusRunning toolStatus = iota
	toolStatusDone
	toolStatusError
)

type chatEntry struct {
	kind       chatEntryKind
	content    string
	toolName   string
	toolArgs   string
	toolResult string
	status     toolStatus
	startedAt  time.Time
	elapsed    time.Duration
	expanded   bool
}

type chatPalette struct {
	bright lipgloss.Color
	dim    lipgloss.Color
	muted  lipgloss.Color
	err    lipgloss.Color
}

type chatKeyMap struct {
	Send    key.Binding
	Newline key.Binding
	Focus   key.Binding
	Expand  key.Binding
	Copy    key.Binding
	Cancel  key.Binding
}

func newChatKeyMap() chatKeyMap {
	return chatKeyMap{
		Send: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "send"),
		),
		Newline: key.NewBinding(
			key.WithKeys("ctrl+j", "shift+enter"),
			key.WithHelp("ctrl+j", "newline"),
		),
		Focus: key.NewBinding(
			key.WithKeys("i", "c"),
			key.WithHelp("i/c", "compose"),
		),
		Expand: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "expand tool"),
		),
		Copy: key.NewBinding(
			key.WithKeys("y"),
			key.WithHelp("y", "copy tool"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "blur/cancel"),
		),
	}
}

func (k chatKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Send, k.Newline, k.Cancel}
}

func (k chatKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Send, k.Newline, k.Cancel},
		{k.Focus, k.Expand, k.Copy},
	}
}

type chatModel struct {
	viewport       viewport.Model
	input          textarea.Model
	spinner        spinner.Model
	help           help.Model
	keys           chatKeyMap
	entries        []chatEntry
	maxEntries     int
	width          int
	height         int
	slashPickerIdx int
	busy           bool
	palette        chatPalette
}

func newChatModel(maxEntries int) chatModel {
	input := textarea.New()
	input.Placeholder = "Ask the agent..."
	input.SetHeight(3)
	input.ShowLineNumbers = false

	sp := spinner.New()
	sp.Spinner = spinner.Line

	h := help.New()
	h.ShowAll = false

	return chatModel{
		viewport:   viewport.New(0, 0),
		input:      input,
		spinner:    sp,
		help:       h,
		keys:       newChatKeyMap(),
		maxEntries: maxEntries,
	}
}

func (c chatModel) Init() tea.Cmd {
	if c.busy {
		return c.spinner.Tick
	}
	return nil
}

func (c chatModel) Update(msg tea.Msg) (chatModel, tea.Cmd) {
	if !c.busy {
		return c, nil
	}
	var cmd tea.Cmd
	c.spinner, cmd = c.spinner.Update(msg)
	c.refreshViewportPreservingScroll()
	return c, cmd
}

func (c *chatModel) SetPalette(p chatPalette) {
	c.palette = p
	c.input.FocusedStyle.CursorLine = lipgloss.NewStyle()
	c.input.FocusedStyle.Base = lipgloss.NewStyle().Foreground(p.bright)
	c.input.Cursor.Style = lipgloss.NewStyle().Foreground(p.bright)
	c.refreshViewport()
}

func (c *chatModel) SetSize(width, height int) {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	c.width = width
	c.height = height
	c.input.SetWidth(width)
	c.help.Width = width

	inputHeight := 3
	helpHeight := 1
	viewportHeight := height - inputHeight - helpHeight - 1
	if viewportHeight < 1 {
		viewportHeight = 1
	}
	c.viewport.Width = width
	c.viewport.Height = viewportHeight
	c.refreshViewport()
}

func (c chatModel) View(inputFocused bool) string {
	var parts []string
	parts = append(parts, c.viewport.View())
	if picker := c.renderSlashPicker(); picker != "" {
		parts = append(parts, picker)
	}
	parts = append(parts, c.input.View())

	helpText := c.help.View(c.keys)
	if !inputFocused {
		helpText = c.mutedStyle().Render(truncateRunes("j/k scroll  i compose  e expand last tool  y copy tool  enter compose", c.width))
	}
	parts = append(parts, helpText)
	return strings.Join(parts, "\n")
}

func (c *chatModel) Focus() tea.Cmd {
	return c.input.Focus()
}

func (c *chatModel) Blur() {
	c.input.Blur()
}

func (c chatModel) Focused() bool {
	return c.input.Focused()
}

func (c chatModel) Value() string {
	return c.input.Value()
}

func (c *chatModel) ResetInput() {
	c.input.Reset()
	c.slashPickerIdx = 0
}

func (c *chatModel) SetInputValue(value string) {
	c.input.Reset()
	c.input.SetValue(value)
	c.slashPickerIdx = 0
}

func (c *chatModel) InsertString(value string) {
	c.input.InsertString(value)
}

func (c *chatModel) UpdateInput(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	c.input, cmd = c.input.Update(msg)
	c.slashPickerIdx = 0
	return cmd
}

func (c *chatModel) ScrollDown(count int) {
	for i := 0; i < count; i++ {
		c.viewport.LineDown(1)
	}
}

func (c *chatModel) ScrollUp(count int) {
	for i := 0; i < count; i++ {
		c.viewport.LineUp(1)
	}
}

func (c *chatModel) GotoTop() {
	c.viewport.GotoTop()
}

func (c *chatModel) GotoBottom() {
	c.viewport.GotoBottom()
}

func (c *chatModel) SetBusy(busy bool) tea.Cmd {
	c.busy = busy
	c.refreshViewport()
	if busy {
		return c.spinner.Tick
	}
	return nil
}

func (c *chatModel) Clear() {
	c.entries = nil
	c.slashPickerIdx = 0
	c.refreshViewport()
}

func (c *chatModel) AppendHistory(role, content string) {
	c.appendEntry(c.entryFromRole(role, content))
}

func (c *chatModel) AppendMessage(role, content string) {
	c.appendEntry(c.entryFromRole(role, content))
	c.refreshViewport()
}

func (c *chatModel) AppendToolStart(evt agent.ToolStartEvent) {
	c.appendEntry(chatEntry{
		kind:      chatEntryTool,
		toolName:  evt.Tool,
		toolArgs:  prettyJSON(evt.Args),
		status:    toolStatusRunning,
		startedAt: time.Now(),
	})
	c.refreshViewport()
}

func (c *chatModel) AppendToolResult(evt agent.ToolResultEvent) {
	idx := c.lastRunningTool(evt.Tool)
	if idx < 0 {
		c.appendEntry(chatEntry{
			kind:       chatEntryTool,
			toolName:   evt.Tool,
			toolResult: limitText(evt.Result, 4000),
			status:     toolStatusDone,
		})
		c.refreshViewport()
		return
	}

	entry := &c.entries[idx]
	entry.toolResult = limitText(evt.Result, 4000)
	entry.status = toolStatusDone
	if strings.Contains(strings.ToLower(evt.Result), "error") {
		entry.status = toolStatusError
	}
	if !entry.startedAt.IsZero() {
		entry.elapsed = time.Since(entry.startedAt)
	}
	c.refreshViewport()
}

func (c *chatModel) ToggleLastTool() bool {
	for i := len(c.entries) - 1; i >= 0; i-- {
		if c.entries[i].kind == chatEntryTool {
			c.entries[i].expanded = !c.entries[i].expanded
			c.refreshViewport()
			return true
		}
	}
	return false
}

func (c chatModel) LastToolText() string {
	for i := len(c.entries) - 1; i >= 0; i-- {
		entry := c.entries[i]
		if entry.kind != chatEntryTool {
			continue
		}
		if entry.toolResult != "" {
			return entry.toolResult
		}
		if entry.toolArgs != "" {
			return entry.toolArgs
		}
	}
	return ""
}

func (c chatModel) LastStartTaskCommand() string {
	for i := len(c.entries) - 1; i >= 0; i-- {
		entry := c.entries[i]
		if entry.kind != chatEntryTool || entry.toolName != "start_task" {
			continue
		}
		var args map[string]interface{}
		if err := json.Unmarshal([]byte(entry.toolArgs), &args); err != nil {
			continue
		}
		if command, ok := args["command"].(string); ok {
			return command
		}
	}
	return ""
}

func (c chatModel) ShowSlashPicker() bool {
	value := c.input.Value()
	return strings.HasPrefix(value, "/") && !strings.Contains(value, " ")
}

func (c chatModel) FilteredSlashCommands() []slashCommand {
	value := c.input.Value()
	var result []slashCommand
	for _, cmd := range slashCommands {
		if strings.HasPrefix(cmd.name, value) {
			result = append(result, cmd)
		}
	}
	return result
}

func (c *chatModel) SlashPickerUp() {
	filtered := c.FilteredSlashCommands()
	if len(filtered) == 0 {
		return
	}
	c.slashPickerIdx--
	if c.slashPickerIdx < 0 {
		c.slashPickerIdx = len(filtered) - 1
	}
}

func (c *chatModel) SlashPickerDown() {
	filtered := c.FilteredSlashCommands()
	if len(filtered) == 0 {
		return
	}
	c.slashPickerIdx++
	if c.slashPickerIdx >= len(filtered) {
		c.slashPickerIdx = 0
	}
}

func (c *chatModel) CompleteSlashCommand() bool {
	filtered := c.FilteredSlashCommands()
	if len(filtered) == 0 {
		return false
	}
	idx := c.slashPickerIdx % len(filtered)
	c.SetInputValue(filtered[idx].name + " ")
	return true
}

func (c *chatModel) appendEntry(entry chatEntry) {
	c.entries = append(c.entries, entry)
	if c.maxEntries > 0 && len(c.entries) > c.maxEntries {
		c.entries = c.entries[len(c.entries)-c.maxEntries:]
	}
}

func (c chatModel) entryFromRole(role, content string) chatEntry {
	switch role {
	case "user":
		return chatEntry{kind: chatEntryUser, content: content}
	case "agent", "assistant":
		return chatEntry{kind: chatEntryAssistant, content: content}
	case "tool":
		return parseToolHistory(content)
	case "error":
		return chatEntry{kind: chatEntryError, content: content}
	default:
		return chatEntry{kind: chatEntrySystem, content: content}
	}
}

func parseToolHistory(content string) chatEntry {
	if strings.HasPrefix(content, "[") {
		end := strings.Index(content, "]")
		if end > 1 {
			return chatEntry{
				kind:     chatEntryTool,
				toolName: content[1:end],
				toolArgs: prettyJSON(strings.TrimSpace(content[end+1:])),
				status:   toolStatusDone,
			}
		}
	}
	if strings.HasPrefix(content, "->") {
		return chatEntry{
			kind:       chatEntryTool,
			toolName:   "result",
			toolResult: strings.TrimSpace(strings.TrimPrefix(content, "->")),
			status:     toolStatusDone,
		}
	}
	return chatEntry{kind: chatEntryTool, toolName: "tool", toolResult: content, status: toolStatusDone}
}

func (c chatModel) lastRunningTool(name string) int {
	for i := len(c.entries) - 1; i >= 0; i-- {
		entry := c.entries[i]
		if entry.kind == chatEntryTool && entry.toolName == name && entry.status == toolStatusRunning {
			return i
		}
	}
	return -1
}

func (c *chatModel) refreshViewport() {
	c.setViewportContent(true)
}

func (c *chatModel) refreshViewportPreservingScroll() {
	c.setViewportContent(false)
}

func (c *chatModel) setViewportContent(forceBottom bool) {
	atBottom := c.viewport.AtBottom()
	offset := c.viewport.YOffset

	var content strings.Builder
	for i, entry := range c.entries {
		if i > 0 {
			content.WriteString("\n\n")
		}
		content.WriteString(c.renderEntry(entry))
	}

	if c.busy {
		if content.Len() > 0 {
			content.WriteString("\n\n")
		}
		content.WriteString(c.renderBusy())
	}

	c.viewport.SetContent(content.String())
	if forceBottom || atBottom {
		c.viewport.GotoBottom()
	} else {
		c.viewport.SetYOffset(offset)
	}
}

func (c chatModel) renderEntry(entry chatEntry) string {
	switch entry.kind {
	case chatEntryUser:
		return c.renderMessage("you", entry.content, c.palette.bright)
	case chatEntryAssistant:
		return c.renderMessage("watchy", entry.content, c.palette.dim)
	case chatEntryTool:
		return c.renderTool(entry)
	case chatEntryError:
		return c.renderMessage("error", entry.content, c.palette.err)
	default:
		return c.renderMessage("note", entry.content, c.palette.muted)
	}
}

func (c chatModel) renderMessage(label, content string, color lipgloss.Color) string {
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(color)
	bodyStyle := lipgloss.NewStyle().PaddingLeft(2)
	return labelStyle.Render(label) + "\n" + bodyStyle.Render(strings.TrimSpace(content))
}

func (c chatModel) renderTool(entry chatEntry) string {
	color := c.palette.dim
	status := "done"
	if entry.status == toolStatusRunning {
		color = c.palette.bright
		status = "run"
	} else if entry.status == toolStatusError {
		color = c.palette.err
		status = "error"
	}

	duration := ""
	if entry.status == toolStatusRunning && !entry.startedAt.IsZero() {
		duration = " " + time.Since(entry.startedAt).Round(100*time.Millisecond).String()
	} else if entry.elapsed > 0 {
		duration = " " + entry.elapsed.Round(time.Millisecond).String()
	}

	prefix := fmt.Sprintf("[%s] %s%s", status, entry.toolName, duration)
	if entry.status == toolStatusRunning {
		prefix = fmt.Sprintf("%s %s %s%s", c.spinner.View(), "[run]", entry.toolName, duration)
	}

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(color)
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(color).
		PaddingLeft(1)

	lines := []string{headerStyle.Render(prefix)}
	if entry.toolArgs != "" {
		lines = append(lines, c.mutedStyle().Render("args: ")+oneLine(entry.toolArgs, c.contentWidth()-8))
	}
	if entry.toolResult != "" {
		result := oneLine(entry.toolResult, c.contentWidth()-10)
		if entry.expanded {
			result = "\n" + indent(strings.TrimSpace(entry.toolResult), 2)
		}
		lines = append(lines, c.mutedStyle().Render("result: ")+result)
	}
	if entry.expanded && entry.toolArgs != "" {
		lines = append(lines, c.mutedStyle().Render("full args:")+"\n"+indent(entry.toolArgs, 2))
	}
	return boxStyle.Render(strings.Join(lines, "\n"))
}

func (c chatModel) renderBusy() string {
	style := lipgloss.NewStyle().Foreground(c.palette.bright)
	return style.Render(c.spinner.View() + " agent working")
}

func (c chatModel) renderSlashPicker() string {
	if !c.ShowSlashPicker() {
		return ""
	}

	filtered := c.FilteredSlashCommands()
	if len(filtered) == 0 {
		return ""
	}

	selectedStyle := lipgloss.NewStyle().Bold(true).Foreground(c.palette.bright)
	dimStyle := c.mutedStyle()

	var lines []string
	idx := c.slashPickerIdx % len(filtered)
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
		BorderForeground(c.palette.dim).
		Padding(0, 1)

	return border.Render(strings.Join(lines, "\n"))
}

func (c chatModel) mutedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(c.palette.muted)
}

func (c chatModel) contentWidth() int {
	if c.width <= 0 {
		return 80
	}
	return c.width
}

func prettyJSON(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var obj interface{}
	if err := json.Unmarshal([]byte(value), &obj); err != nil {
		return value
	}
	pretty, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return value
	}
	return string(pretty)
}

func oneLine(value string, limit int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	return limitText(value, limit)
}

func limitText(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit <= 3 {
		return string(runes[:limit])
	}
	return string(runes[:limit-3]) + "..."
}

func indent(value string, spaces int) string {
	padding := strings.Repeat(" ", spaces)
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		lines[i] = padding + line
	}
	return strings.Join(lines, "\n")
}
