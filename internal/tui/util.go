package tui

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func truncateRunes(value string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width == 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}

// lenInt returns the number of digits in n (base 10), minimum 1.
func lenInt(n int) int {
	if n < 10 {
		return 1
	}
	width := 0
	for n > 0 {
		n /= 10
		width++
	}
	return width
}
