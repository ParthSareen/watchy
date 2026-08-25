package tui

import "testing"

func TestLogsModelGetSelectedLines(t *testing.T) {
	l := logsModel{rawLogContent: "line1\nline2\nline3\nline4"}

	cases := []struct {
		name       string
		start, end int
		want       string
	}{
		{"single line", 1, 1, "line2"},
		{"range forward", 1, 3, "line2\nline3\nline4"},
		{"full range", 0, 3, "line1\nline2\nline3\nline4"},
		{"range reversed", 3, 0, "line1\nline2\nline3\nline4"},
		{"end clamped to last line", 2, 10, "line3\nline4"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l.visualMode = true
			l.visualStart = c.start
			l.cursorLine = c.end
			if got := l.getSelectedLines(); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}

	l.visualMode = false
	if l.getSelectedLines() != "" {
		t.Fatalf("expected empty selection outside visual mode, got %q", l.getSelectedLines())
	}
}

func TestLogsModelSetContentUpdatesLines(t *testing.T) {
	l := newLogsModel()
	l.logViewport.Width = 40
	l.logViewport.Height = 10

	l.SetContent("a\nb\nc", "a\nb\nc", []int{1, 2, 3}, 0)
	if l.totalLogLines() != 3 {
		t.Fatalf("totalLogLines = %d, want 3", l.totalLogLines())
	}
	if l.allLogContent != "a\nb\nc" {
		t.Fatalf("allLogContent = %q", l.allLogContent)
	}
	// Active view mirrors the canonical content when no search filter is set.
	if l.rawLogContent != l.allRawLogContent {
		t.Fatalf("rawLogContent = %q, want %q", l.rawLogContent, l.allRawLogContent)
	}
}
