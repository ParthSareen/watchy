package tui

import (
	"github.com/charmbracelet/lipgloss"
)

type theme struct {
	name     string
	bright   lipgloss.Color
	dim      lipgloss.Color
	bg       lipgloss.Color
	brightLg lipgloss.Color
	dimLg    lipgloss.Color
	bgLg     lipgloss.Color
}

var themes = []theme{
	{"green", lipgloss.Color("46"), lipgloss.Color("22"), lipgloss.Color("22"), lipgloss.Color("28"), lipgloss.Color("85"), lipgloss.Color("157")},
	{"blue", lipgloss.Color("39"), lipgloss.Color("24"), lipgloss.Color("24"), lipgloss.Color("32"), lipgloss.Color("74"), lipgloss.Color("117")},
	{"purple", lipgloss.Color("141"), lipgloss.Color("54"), lipgloss.Color("54"), lipgloss.Color("135"), lipgloss.Color("183"), lipgloss.Color("225")},
	{"orange", lipgloss.Color("208"), lipgloss.Color("94"), lipgloss.Color("94"), lipgloss.Color("202"), lipgloss.Color("220"), lipgloss.Color("223")},
	{"pink", lipgloss.Color("205"), lipgloss.Color("125"), lipgloss.Color("125"), lipgloss.Color("198"), lipgloss.Color("211"), lipgloss.Color("218")},
	{"cyan", lipgloss.Color("51"), lipgloss.Color("30"), lipgloss.Color("30"), lipgloss.Color("37"), lipgloss.Color("87"), lipgloss.Color("122")},
	{"red", lipgloss.Color("196"), lipgloss.Color("88"), lipgloss.Color("88"), lipgloss.Color("196"), lipgloss.Color("203"), lipgloss.Color("224")},
	{"white", lipgloss.Color("255"), lipgloss.Color("245"), lipgloss.Color("245"), lipgloss.Color("0"), lipgloss.Color("7"), lipgloss.Color("15")},
}

var (
	errorColor  = lipgloss.Color("124")
	errorColorL = lipgloss.Color("196")
	dimGray     = lipgloss.Color("240")
	dimGrayL    = lipgloss.Color("242")
)

func (m Model) theme() theme {
	return themes[m.themeIdx%len(themes)]
}

// bright returns the bright (foreground) color based on light/dark mode
func (m Model) bright() lipgloss.Color {
	t := m.theme()
	if m.lightMode {
		return t.brightLg
	}
	return t.bright
}

// dim returns the dim (foreground) color based on light/dark mode
func (m Model) dim() lipgloss.Color {
	t := m.theme()
	if m.lightMode {
		return t.dimLg
	}
	return t.dim
}

// bg returns the background color for selected items based on light/dark mode
func (m Model) bg() lipgloss.Color {
	t := m.theme()
	if m.lightMode {
		return t.bgLg
	}
	return t.bg
}

// dimGrayForMode returns the appropriate dim gray color
func (m Model) dimGrayForMode() lipgloss.Color {
	if m.lightMode {
		return dimGrayL
	}
	return dimGray
}

// errorColorForMode returns the appropriate error color
func (m Model) errorColorForMode() lipgloss.Color {
	if m.lightMode {
		return errorColorL
	}
	return errorColor
}

func (m *Model) syncChatPalette() {
	palette := chatPalette{
		bright: m.bright(),
		dim:    m.dim(),
		muted:  m.dimGrayForMode(),
		err:    m.errorColorForMode(),
	}
	m.chat.SetPalette(palette)
	m.logs.SetPalette(logPalette{
		bright:  m.bright(),
		dim:     m.dim(),
		dimGray: m.dimGrayForMode(),
		bg:      m.bg(),
	})
}
