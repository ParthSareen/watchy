package logcolor

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	// slog-style key=value patterns
	kvPattern = regexp.MustCompile(`(\w+)=("(?:[^"\\]|\\.)*"|\S+)`)

	// GIN request line: [GIN] 2026/02/05 - 19:00:33 | 200 | ...
	ginReqPattern = regexp.MustCompile(`^(\[GIN\]\s+\S+\s+-\s+\S+)\s+\|\s+(\d{3})\s+\|(.*)$`)

	// GIN debug/warning: [GIN-debug] [WARNING] ...
	ginWarnPattern = regexp.MustCompile(`^(\[GIN-debug\])\s+(\[WARNING\])\s+(.*)$`)

	// Light mode state - can be toggled at runtime
	IsLightMode = detectLightMode()

	dimStyle   = dimStyleFor(IsLightMode)
	warnStyle  = warnStyleFor(IsLightMode)
	errorStyle = errorStyleFor(IsLightMode)
	infoStyle  = infoStyleFor(IsLightMode)
	debugStyle = debugStyleFor(IsLightMode)
	msgStyle   = msgStyleFor(IsLightMode)
	keyStyle   = keyStyleFor(IsLightMode)
	valStyle   = valStyleFor(IsLightMode)
)

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

func detectLightMode() bool {
	colorfgbg := os.Getenv("COLORFGBG")
	if colorfgbg == "" {
		return false
	}
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
	if strings.Contains(line, "level=") {
		return colorizeSlog(line)
	}
	if strings.HasPrefix(line, "[GIN") {
		return colorizeGin(line)
	}
	return line
}

func colorizeSlog(line string) string {
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

func colorizeGin(line string) string {
	// [GIN-debug] [WARNING] ...
	if m := ginWarnPattern.FindStringSubmatch(line); m != nil {
		return debugStyle.Render(m[1]) + " " + warnStyle.Render(m[2]) + " " + msgStyle.Render(m[3])
	}

	// [GIN] 2026/02/05 - 19:00:33 | 200 | ...
	if m := ginReqPattern.FindStringSubmatch(line); m != nil {
		prefix := m[1]
		status := m[2]
		rest := m[3]

		statusStyle := infoStyle
		if status[0] == '4' {
			statusStyle = warnStyle
		} else if status[0] == '5' {
			statusStyle = errorStyle
		}

		return prefix + " | " + statusStyle.Render(status) + " |" + rest
	}

	return line
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
