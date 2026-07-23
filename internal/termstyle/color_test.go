package termstyle

import "testing"

func TestLightModeFromCOLORFGBGUsesLastSegmentAsBackground(t *testing.T) {
	tests := []struct {
		name  string
		value string
		light bool
		ok    bool
	}{
		{name: "dark background with light foreground", value: "15;0", light: false, ok: true},
		{name: "light background", value: "0;15", light: true, ok: true},
		{name: "multi segment tmux style value", value: "15;2;0", light: false, ok: true},
		{name: "missing value", value: "", ok: false},
		{name: "out of ANSI range", value: "0;255", ok: false},
		{name: "non numeric", value: "0;light", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			light, ok := LightModeFromCOLORFGBG(tt.value)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if light != tt.light {
				t.Fatalf("light = %v, want %v", light, tt.light)
			}
		})
	}
}
