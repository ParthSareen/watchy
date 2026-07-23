package termstyle

import (
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	ColorModeAuto  = "auto"
	ColorModeDark  = "dark"
	ColorModeLight = "light"
)

// NormalizeColorMode returns a supported color mode, defaulting to auto.
func NormalizeColorMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case ColorModeDark:
		return ColorModeDark
	case ColorModeLight:
		return ColorModeLight
	default:
		return ColorModeAuto
	}
}

// ResolveLightMode resolves a user color-mode preference into a light/dark
// palette choice.
func ResolveLightMode(mode string) bool {
	switch NormalizeColorMode(mode) {
	case ColorModeLight:
		return true
	case ColorModeDark:
		return false
	default:
		return DetectLightMode()
	}
}

// DetectLightMode detects whether the terminal background is light. COLORFGBG,
// when present, stores foreground first and background last.
func DetectLightMode() bool {
	if light, ok := LightModeFromCOLORFGBG(os.Getenv("COLORFGBG")); ok {
		return light
	}
	return !lipgloss.HasDarkBackground()
}

// ApplyLightMode keeps lipgloss adaptive colors aligned with watchy's palette.
func ApplyLightMode(light bool) {
	lipgloss.SetHasDarkBackground(!light)
}

func LightModeFromCOLORFGBG(value string) (bool, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return false, false
	}

	parts := strings.Split(value, ";")
	bg := strings.TrimSpace(parts[len(parts)-1])
	if bg == "" {
		return false, false
	}

	n, err := strconv.Atoi(bg)
	if err != nil {
		return false, false
	}

	if n >= 0 && n <= 6 {
		return false, true
	}
	if n >= 7 && n <= 15 {
		return true, true
	}
	return false, false
}
