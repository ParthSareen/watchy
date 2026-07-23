package logcolor

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	// slog-style key=value patterns
	kvPattern = regexp.MustCompile(`(\w+)=("(?:[^"\\]|\\.)*"|\S+)`)

	// GIN request line: [GIN] 2026/02/05 - 19:00:33 | 200 | ...
	ginReqPattern = regexp.MustCompile(`^\[GIN\]\s+(\d{4}/\d{2}/\d{2})\s+-\s+(\d{2}:\d{2}:\d{2})\s+\|\s+(\d{3})\s+\|\s+([^|]+?)\s+\|\s+([^|]+?)\s+\|\s+(\S+)\s+"([^"]*)"\s*$`)

	// GIN debug/warning: [GIN-debug] [WARNING] ...
	ginWarnPattern = regexp.MustCompile(`^(\[GIN-debug\])\s+(\[WARNING\])\s+(.*)$`)

	// Light mode state is set by the TUI after resolving config + terminal state.
	IsLightMode = false

	dimStyle   = dimStyleFor(IsLightMode)
	warnStyle  = warnStyleFor(IsLightMode)
	errorStyle = errorStyleFor(IsLightMode)
	infoStyle  = infoStyleFor(IsLightMode)
	debugStyle = debugStyleFor(IsLightMode)
	msgStyle   = msgStyleFor(IsLightMode)
	keyStyle   = keyStyleFor(IsLightMode)
	valStyle   = valStyleFor(IsLightMode)
)

type RenderOptions struct {
	ShowNoise bool
}

// SetLightMode updates the color styles for the given light/dark mode
func SetLightMode(light bool) {
	IsLightMode = light
	dimStyle = dimStyleFor(light)
	warnStyle = warnStyleFor(light)
	errorStyle = errorStyleFor(light)
	infoStyle = infoStyleFor(light)
	debugStyle = debugStyleFor(light)
	msgStyle = msgStyleFor(light)
	keyStyle = keyStyleFor(light)
	valStyle = valStyleFor(light)
}

func dimStyleFor(light bool) lipgloss.Style {
	if light {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
}

func warnStyleFor(light bool) lipgloss.Style {
	if light {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("202")).Bold(true)
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
}

func errorStyleFor(light bool) lipgloss.Style {
	if light {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
}

func infoStyleFor(light bool) lipgloss.Style {
	if light {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("28"))
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
}

func debugStyleFor(light bool) lipgloss.Style {
	if light {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
}

func msgStyleFor(light bool) lipgloss.Style {
	if light {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Bold(true)
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Bold(true)
}

func keyStyleFor(light bool) lipgloss.Style {
	if light {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
}

func valStyleFor(light bool) lipgloss.Style {
	if light {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("236"))
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
}

// Colorize applies color to a single log line if it matches known log formats.
// Non-matching lines are returned as-is.
func Colorize(line string) string {
	rendered, _ := RenderLine(line, RenderOptions{ShowNoise: true})
	return rendered
}

// RenderLine formats a log line for display and reports whether it should be
// hidden as routine noise.
func RenderLine(line string, opts RenderOptions) (string, bool) {
	if !opts.ShowNoise && IsNoise(line) {
		return "", true
	}
	if strings.Contains(line, "level=") {
		return colorizeSlog(line), false
	}
	if strings.HasPrefix(line, "[GIN") {
		return colorizeGin(line), false
	}
	return line, false
}

// IsNoise identifies Ollama serve lines that are usually distracting in watchy.
func IsNoise(line string) bool {
	if isGinStatusPoll(line) {
		return true
	}
	return strings.Contains(line, `msg="bad manifest filepath"`)
}

func colorizeSlog(line string) string {
	if parsed, ok := formatSlog(line); ok {
		return parsed
	}

	matches := kvPattern.FindAllStringSubmatchIndex(line, -1)
	if len(matches) < 2 {
		return line
	}

	var b strings.Builder
	prev := 0

	for _, loc := range matches {
		if loc[0] > prev {
			b.WriteString(line[prev:loc[0]])
		}

		key := line[loc[2]:loc[3]]
		val := line[loc[4]:loc[5]]

		switch key {
		case "level":
			b.WriteString(keyStyle.Render(key + "="))
			b.WriteString(levelStyle(val).Render(val))
		case "time", "source":
			b.WriteString(dimStyle.Render(key + "=" + val))
		case "msg":
			b.WriteString(keyStyle.Render(key + "="))
			b.WriteString(msgStyle.Render(val))
		default:
			b.WriteString(keyStyle.Render(key + "="))
			b.WriteString(valStyle.Render(val))
		}

		prev = loc[1]
	}

	if prev < len(line) {
		b.WriteString(line[prev:])
	}

	return b.String()
}

func formatSlog(line string) (string, bool) {
	fields, order := parseKV(line)
	if len(fields) < 2 {
		return "", false
	}

	level := strings.ToUpper(fields["level"])
	msg := fields["msg"]
	source := fields["source"]
	timeValue := shortTime(fields["time"])
	if level == "" || msg == "" {
		return "", false
	}

	levelText := fmt.Sprintf("%-5s", level)
	parts := []string{
		dimStyle.Render(timeValue),
		levelStyle(level).Render(levelText),
	}
	if source != "" {
		parts = append(parts, dimStyle.Render(source))
	}
	parts = append(parts, msgStyle.Render(msg))

	var extras []string
	for _, key := range order {
		if key == "time" || key == "level" || key == "source" || key == "msg" {
			continue
		}
		extras = append(extras, keyStyle.Render(key+"=")+valStyle.Render(fields[key]))
	}
	if len(extras) > 0 {
		parts = append(parts, strings.Join(extras, " "))
	}

	return strings.Join(parts, " "), true
}

func parseKV(line string) (map[string]string, []string) {
	matches := kvPattern.FindAllStringSubmatch(line, -1)
	fields := make(map[string]string, len(matches))
	order := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		key := match[1]
		value := unquote(match[2])
		if _, exists := fields[key]; !exists {
			order = append(order, key)
		}
		fields[key] = value
	}
	return fields, order
}

func unquote(value string) string {
	if strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
		if unquoted, err := strconv.Unquote(value); err == nil {
			return unquoted
		}
	}
	return value
}

func shortTime(value string) string {
	if value == "" {
		return "--:--:--"
	}
	if t := strings.Index(value, "T"); t >= 0 && len(value) >= t+9 {
		value = value[t+1:]
	}
	if len(value) >= 12 && value[8] == '.' {
		return value[:12]
	}
	if len(value) >= 8 {
		return value[:8]
	}
	return value
}

func colorizeGin(line string) string {
	// [GIN-debug] [WARNING] ...
	if m := ginWarnPattern.FindStringSubmatch(line); m != nil {
		return debugStyle.Render(m[1]) + " " + warnStyle.Render(m[2]) + " " + msgStyle.Render(m[3])
	}

	// [GIN] 2026/02/05 - 19:00:33 | 200 | ...
	if m := ginReqPattern.FindStringSubmatch(line); m != nil {
		timeValue := m[2]
		status := m[3]
		duration := strings.TrimSpace(m[4])
		ip := strings.TrimSpace(m[5])
		method := strings.TrimSpace(m[6])
		path := strings.TrimSpace(m[7])

		statusStyle := infoStyle
		if status[0] == '4' {
			statusStyle = warnStyle
		} else if status[0] == '5' {
			statusStyle = errorStyle
		}

		methodStyle := valStyle
		if method == "GET" {
			methodStyle = debugStyle
		}

		return strings.Join([]string{
			dimStyle.Render(timeValue),
			statusStyle.Render(status),
			methodStyle.Render(fmt.Sprintf("%-6s", method)),
			msgStyle.Render(path),
			valStyle.Render(duration),
			dimStyle.Render(ip),
		}, " ")
	}

	return line
}

func isGinStatusPoll(line string) bool {
	m := ginReqPattern.FindStringSubmatch(line)
	if m == nil {
		return false
	}
	return m[3] == "200" && strings.TrimSpace(m[6]) == "GET" && strings.TrimSpace(m[7]) == "/api/status"
}

func levelStyle(level string) lipgloss.Style {
	switch strings.ToUpper(strings.Trim(level, "\"")) {
	case "WARN", "WARNING":
		return warnStyle
	case "ERROR", "FATAL":
		return errorStyle
	case "INFO":
		return infoStyle
	case "DEBUG":
		return debugStyle
	default:
		return valStyle
	}
}
