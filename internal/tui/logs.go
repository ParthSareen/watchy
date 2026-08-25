package tui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// logPalette holds the colors the logs sub-model needs, pushed from the root
// model whenever the theme or color mode changes (mirrors chatPalette).
type logPalette struct {
	bright  lipgloss.Color
	dim     lipgloss.Color
	dimGray lipgloss.Color
	bg      lipgloss.Color
}

// logsModel owns all log-pane state: the raw and colorized content (with their
// line-number maps), search and command-mode input, the cursor/visual selection,
// and horizontal scroll. The root Model embeds it and delegates content updates
// through SetContent so the raw/colorized/line-number triple stays in sync in
// one place rather than across update.go, view.go, and mouse.go.
type logsModel struct {
	logViewport viewport.Model
	logsLoading bool

	// Search
	searchMode       bool
	searchInput      textinput.Model
	searchTerm       string
	searchMatches    []int
	searchMatchLines []int
	matchIndex       int

	// Command mode (:<line>)
	commandMode  bool
	commandInput textinput.Model

	// Canonical (all) log content and the active (possibly filtered) view.
	allLogContent      string
	allRawLogContent   string
	allLogLineNumbers  []int
	originalLogContent string
	rawLogContent      string
	logLineNumbers     []int

	showLogNoise   bool
	hiddenLogNoise int

	// Cursor and visual selection (vim-style)
	cursorLine  int
	visualMode  bool
	visualStart int

	// Horizontal scroll
	logXOffset int

	palette logPalette
}

func newLogsModel() logsModel {
	si := textinput.New()
	si.Placeholder = "Search..."
	si.Prompt = "/"
	si.Width = 30

	ci := textinput.New()
	ci.Prompt = ":"

	return logsModel{
		logViewport:  viewport.New(0, 0),
		searchInput:  si,
		commandInput: ci,
	}
}

// SetPalette pushes theme colors and re-renders with the new palette.
func (l *logsModel) SetPalette(p logPalette) {
	l.palette = p
	l.refreshLogContent()
}

// SetSize resizes the log viewport.
func (l *logsModel) SetSize(width, height int) {
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}
	l.logViewport.Width = width
	l.logViewport.Height = height
}

// SetContent replaces the canonical log content and refreshes the active view,
// preserving the scroll position and keeping the cursor glued to the bottom
// when it was already there.
func (l *logsModel) SetContent(colored, raw string, lineNumbers []int, hiddenNoise int) {
	oldTotalLines := l.totalLogLines()
	cursorAtBottom := oldTotalLines > 0 && l.cursorLine >= oldTotalLines-1
	atBottom := l.logViewport.AtBottom()
	offset := l.logViewport.YOffset

	l.allRawLogContent = raw
	l.allLogContent = colored
	l.allLogLineNumbers = append(l.allLogLineNumbers[:0], lineNumbers...)
	l.hiddenLogNoise = hiddenNoise

	if l.searchTerm != "" {
		l.applySearchFilter()
	} else {
		l.restoreAllLogs()
		l.refreshLogContent()
	}

	newTotalLines := l.totalLogLines()
	if newTotalLines == 0 {
		l.cursorLine = 0
	} else if cursorAtBottom {
		l.cursorLine = newTotalLines - 1
	} else if l.cursorLine >= newTotalLines {
		l.cursorLine = newTotalLines - 1
	}

	// Exit visual mode when content changes (selection would be invalid).
	if l.visualMode && oldTotalLines != newTotalLines {
		l.visualMode = false
	}

	if atBottom {
		l.logViewport.GotoBottom()
	} else {
		l.logViewport.SetYOffset(offset)
	}
}

func (l *logsModel) applySearchFilter() {
	rawLines := splitLogLines(l.allRawLogContent)
	coloredLines := splitLogLines(l.allLogContent)
	termLower := strings.ToLower(l.searchTerm)
	l.searchMatches = nil
	l.searchMatchLines = nil
	var filteredRaw, filteredColored []string
	var filteredLineNumbers []int
	for i, rawLine := range rawLines {
		if !strings.Contains(strings.ToLower(rawLine), termLower) {
			continue
		}
		l.searchMatches = append(l.searchMatches, len(filteredRaw))
		originalLineNumber := i + 1
		if i < len(l.allLogLineNumbers) {
			originalLineNumber = l.allLogLineNumbers[i]
		}
		l.searchMatchLines = append(l.searchMatchLines, originalLineNumber)
		filteredLineNumbers = append(filteredLineNumbers, originalLineNumber)
		filteredRaw = append(filteredRaw, rawLine)
		if i < len(coloredLines) {
			filteredColored = append(filteredColored, coloredLines[i])
		} else {
			filteredColored = append(filteredColored, rawLine)
		}
	}
	l.rawLogContent = strings.Join(filteredRaw, "\n")
	l.originalLogContent = strings.Join(filteredColored, "\n")
	l.logLineNumbers = filteredLineNumbers
	if len(filteredRaw) == 0 {
		l.logViewport.SetContent(fmt.Sprintf("no matches for %q", l.searchTerm))
	} else {
		l.refreshLogContent()
	}
	if l.matchIndex >= len(l.searchMatches) {
		l.matchIndex = 0
	}
	if l.cursorLine >= len(filteredRaw) {
		l.cursorLine = maxInt(0, len(filteredRaw)-1)
	}
}

func (l *logsModel) restoreAllLogs() {
	l.rawLogContent = l.allRawLogContent
	l.originalLogContent = l.allLogContent
	l.logLineNumbers = append(l.logLineNumbers[:0], l.allLogLineNumbers...)
}

func splitLogLines(content string) []string {
	if content == "" {
		return nil
	}
	return strings.Split(content, "\n")
}

func (l *logsModel) scrollToMatch() {
	if len(l.searchMatches) == 0 {
		return
	}
	l.logViewport.SetYOffset(l.searchMatches[l.matchIndex])
}

func (l *logsModel) addLineNumbers(content string) string {
	lines := strings.Split(content, "\n")
	lineNumberStyle := lipgloss.NewStyle().Foreground(l.palette.dimGray)
	separatorStyle := lipgloss.NewStyle().Foreground(l.palette.dim)
	cursorStyle := lipgloss.NewStyle().Bold(true).Foreground(l.palette.bright)
	selectedStyle := lipgloss.NewStyle().Background(l.palette.bg).Foreground(l.palette.bright)

	maxLineNumber := len(lines)
	for _, lineNumber := range l.logLineNumbers {
		if lineNumber > maxLineNumber {
			maxLineNumber = lineNumber
		}
	}
	lineNumberWidth := len(fmt.Sprintf("%d", maxLineNumber))
	start, end := l.visualStart, l.cursorLine
	if start > end {
		start, end = end, start
	}

	var result strings.Builder
	for i, line := range lines {
		if i > 0 {
			result.WriteString("\n")
		}
		lineNumber := i + 1
		if i < len(l.logLineNumbers) {
			lineNumber = l.logLineNumbers[i]
		}
		lineNumberText := fmt.Sprintf("%*d", lineNumberWidth, lineNumber)
		inSelection := l.visualMode && i >= start && i <= end
		isCursor := !l.visualMode && i == l.cursorLine
		displayLine := skipANSI(line, l.logXOffset)

		switch {
		case inSelection:
			cleanLine := ansiRegex.ReplaceAllString(displayLine, "")
			result.WriteString(selectedStyle.Render(lineNumberText + " │ " + cleanLine))
		case isCursor:
			result.WriteString(cursorStyle.Render(lineNumberText))
			result.WriteString(cursorStyle.Render(" ► "))
			result.WriteString(displayLine)
		default:
			result.WriteString(lineNumberStyle.Render(lineNumberText))
			result.WriteString(separatorStyle.Render(" │ "))
			result.WriteString(displayLine)
		}
	}
	return result.String()
}

func skipANSI(value string, offset int) string {
	if offset <= 0 {
		return value
	}
	visible := 0
	inEscape := false
	for i := 0; i < len(value); i++ {
		if value[i] == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if value[i] >= '@' && value[i] <= '~' {
				inEscape = false
			}
			continue
		}
		if visible >= offset {
			return value[i:]
		}
		visible++
	}
	return ""
}

func (l *logsModel) totalLogLines() int {
	return len(l.logLineNumbers)
}

func (l *logsModel) maxLineLength() int {
	maxLength := 0
	for _, line := range splitLogLines(l.originalLogContent) {
		plain := ansiRegex.ReplaceAllString(line, "")
		if len(plain) > maxLength {
			maxLength = len(plain)
		}
	}
	return maxLength
}

func (l *logsModel) refreshLogContent() {
	yOffset := l.logViewport.YOffset
	l.logViewport.SetContent(l.addLineNumbers(l.originalLogContent))
	l.logViewport.SetYOffset(yOffset)
}

func (l *logsModel) getSelectedLines() string {
	if !l.visualMode {
		return ""
	}
	lines := splitLogLines(l.rawLogContent)
	start, end := l.visualStart, l.cursorLine
	if start > end {
		start, end = end, start
	}
	if start < 0 {
		start = 0
	}
	if end >= len(lines) {
		end = len(lines) - 1
	}
	if start >= len(lines) || end < start {
		return ""
	}
	return strings.Join(lines[start:end+1], "\n")
}

func (l logsModel) logNoiseLabel() string {
	if l.showLogNoise {
		return " [noise shown]"
	}
	if l.hiddenLogNoise > 0 {
		return fmt.Sprintf(" [%d noise hidden]", l.hiddenLogNoise)
	}
	return ""
}
