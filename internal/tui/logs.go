package tui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func (m *Model) applySearchFilter() {
	rawLines := splitLogLines(m.allRawLogContent)
	coloredLines := splitLogLines(m.allLogContent)
	termLower := strings.ToLower(m.searchTerm)
	m.searchMatches = nil
	m.searchMatchLines = nil
	var filteredRaw, filteredColored []string
	var filteredLineNumbers []int
	for i, rawLine := range rawLines {
		if !strings.Contains(strings.ToLower(rawLine), termLower) {
			continue
		}
		m.searchMatches = append(m.searchMatches, len(filteredRaw))
		originalLineNumber := i + 1
		if i < len(m.allLogLineNumbers) {
			originalLineNumber = m.allLogLineNumbers[i]
		}
		m.searchMatchLines = append(m.searchMatchLines, originalLineNumber)
		filteredLineNumbers = append(filteredLineNumbers, originalLineNumber)
		filteredRaw = append(filteredRaw, rawLine)
		if i < len(coloredLines) {
			filteredColored = append(filteredColored, coloredLines[i])
		} else {
			filteredColored = append(filteredColored, rawLine)
		}
	}
	m.rawLogContent = strings.Join(filteredRaw, "\n")
	m.originalLogContent = strings.Join(filteredColored, "\n")
	m.logLineNumbers = filteredLineNumbers
	if len(filteredRaw) == 0 {
		m.logViewport.SetContent(fmt.Sprintf("no matches for %q", m.searchTerm))
	} else {
		m.refreshLogContent()
	}
	if m.matchIndex >= len(m.searchMatches) {
		m.matchIndex = 0
	}
	if m.cursorLine >= len(filteredRaw) {
		m.cursorLine = maxInt(0, len(filteredRaw)-1)
	}
}

func (m *Model) restoreAllLogs() {
	m.rawLogContent = m.allRawLogContent
	m.originalLogContent = m.allLogContent
	m.logLineNumbers = append(m.logLineNumbers[:0], m.allLogLineNumbers...)
}

func splitLogLines(content string) []string {
	if content == "" {
		return nil
	}
	return strings.Split(content, "\n")
}

func (m *Model) scrollToMatch() {
	if len(m.searchMatches) == 0 {
		return
	}
	m.logViewport.SetYOffset(m.searchMatches[m.matchIndex])
}

func (m *Model) addLineNumbers(content string) string {
	lines := strings.Split(content, "\n")
	lineNumberStyle := lipgloss.NewStyle().Foreground(m.dimGrayForMode())
	separatorStyle := lipgloss.NewStyle().Foreground(m.dim())
	cursorStyle := lipgloss.NewStyle().Bold(true).Foreground(m.bright())
	selectedStyle := lipgloss.NewStyle().Background(m.bg()).Foreground(m.bright())

	maxLineNumber := len(lines)
	for _, lineNumber := range m.logLineNumbers {
		if lineNumber > maxLineNumber {
			maxLineNumber = lineNumber
		}
	}
	lineNumberWidth := len(fmt.Sprintf("%d", maxLineNumber))
	start, end := m.visualStart, m.cursorLine
	if start > end {
		start, end = end, start
	}

	var result strings.Builder
	for i, line := range lines {
		if i > 0 {
			result.WriteString("\n")
		}
		lineNumber := i + 1
		if i < len(m.logLineNumbers) {
			lineNumber = m.logLineNumbers[i]
		}
		lineNumberText := fmt.Sprintf("%*d", lineNumberWidth, lineNumber)
		inSelection := m.visualMode && i >= start && i <= end
		isCursor := !m.visualMode && i == m.cursorLine
		displayLine := skipANSI(line, m.logXOffset)

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

func (m *Model) totalLogLines() int {
	return len(m.logLineNumbers)
}

func (m *Model) maxLineLength() int {
	maxLength := 0
	for _, line := range splitLogLines(m.originalLogContent) {
		plain := ansiRegex.ReplaceAllString(line, "")
		if len(plain) > maxLength {
			maxLength = len(plain)
		}
	}
	return maxLength
}

func (m *Model) refreshLogContent() {
	yOffset := m.logViewport.YOffset
	m.logViewport.SetContent(m.addLineNumbers(m.originalLogContent))
	m.logViewport.SetYOffset(yOffset)
}

func (m *Model) getSelectedLines() string {
	if !m.visualMode {
		return ""
	}
	lines := splitLogLines(m.rawLogContent)
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
	if start >= len(lines) || end < start {
		return ""
	}
	return strings.Join(lines[start:end+1], "\n")
}
