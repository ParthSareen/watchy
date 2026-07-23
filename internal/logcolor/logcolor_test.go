package logcolor

import (
	"strings"
	"testing"
)

func TestRenderGinAccessLine(t *testing.T) {
	line := `[GIN] 2026/05/05 - 15:49:02 | 200 |   21.621916ms |       127.0.0.1 | GET      "/api/tags"`

	rendered, hidden := RenderLine(line, RenderOptions{})
	if hidden {
		t.Fatal("GET /api/tags should be visible")
	}
	for _, want := range []string{"15:49:02", "200", "GET", "/api/tags", "21.621916ms", "127.0.0.1"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered line missing %q: %q", want, rendered)
		}
	}
}

func TestRenderHidesGinStatusPollNoise(t *testing.T) {
	line := `[GIN] 2026/05/05 - 15:49:02 |       200 |       2.292ms |       127.0.0.1 | GET      "/api/status"`

	if _, hidden := RenderLine(line, RenderOptions{}); !hidden {
		t.Fatal("GET /api/status should be hidden by default")
	}
	if rendered, hidden := RenderLine(line, RenderOptions{ShowNoise: true}); hidden || !strings.Contains(rendered, "/api/status") {
		t.Fatalf("ShowNoise should reveal status poll, hidden=%v rendered=%q", hidden, rendered)
	}
}

func TestRenderSlogLine(t *testing.T) {
	line := `time=2026-05-05T15:49:02.470-07:00 level=DEBUG source=model_recommendations.go:227 msg="model recommendations refreshed" count=7`

	rendered, hidden := RenderLine(line, RenderOptions{})
	if hidden {
		t.Fatal("recommendation refresh should be visible")
	}
	for _, want := range []string{"15:49:02.470", "DEBUG", "model_recommendations.go:227", "model recommendations refreshed", "count=7"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered line missing %q: %q", want, rendered)
		}
	}
}

func TestRenderHidesBadManifestNoise(t *testing.T) {
	line := `time=2026-05-05T15:54:41.962-07:00 level=WARN source=routes.go:1427 msg="bad manifest filepath" name=registry.ollama.ai/library/phi4:latest error="open /Users/parth/.ollama/models/blobs/sha256-f5d6: no such file or directory"`

	if _, hidden := RenderLine(line, RenderOptions{}); !hidden {
		t.Fatal("bad manifest filepath should be hidden by default")
	}
	if rendered, hidden := RenderLine(line, RenderOptions{ShowNoise: true}); hidden || !strings.Contains(rendered, "bad manifest filepath") {
		t.Fatalf("ShowNoise should reveal bad manifest line, hidden=%v rendered=%q", hidden, rendered)
	}
}
