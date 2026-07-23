package tui

import (
	"testing"

	"github.com/parth/watchy/internal/agent"
)

func TestChatToolResultUpdatesRunningTool(t *testing.T) {
	chat := newChatModel(maxChatMessages)

	chat.AppendToolStart(agent.ToolStartEvent{
		Tool: "start_task",
		Args: `{"command":"go test ./...","name":"tests"}`,
	})
	chat.AppendToolResult(agent.ToolResultEvent{
		Tool:   "start_task",
		Result: "started task 12",
	})

	if len(chat.entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(chat.entries))
	}
	entry := chat.entries[0]
	if entry.status != toolStatusDone {
		t.Fatalf("status = %v, want done", entry.status)
	}
	if entry.toolResult != "started task 12" {
		t.Fatalf("toolResult = %q", entry.toolResult)
	}
}

func TestChatFindsLastStartTaskCommand(t *testing.T) {
	chat := newChatModel(maxChatMessages)
	chat.AppendToolStart(agent.ToolStartEvent{
		Tool: "bash_command",
		Args: `{"command":"ls"}`,
	})
	chat.AppendToolStart(agent.ToolStartEvent{
		Tool: "start_task",
		Args: `{"command":"npm run dev","name":"dev"}`,
	})

	if got := chat.LastStartTaskCommand(); got != "npm run dev" {
		t.Fatalf("LastStartTaskCommand() = %q", got)
	}
}

func TestChatToggleLastTool(t *testing.T) {
	chat := newChatModel(maxChatMessages)
	chat.AppendToolStart(agent.ToolStartEvent{Tool: "read_file", Args: `{"path":"/tmp/a"}`})

	if !chat.ToggleLastTool() {
		t.Fatal("ToggleLastTool returned false")
	}
	if !chat.entries[0].expanded {
		t.Fatal("tool should be expanded")
	}
}
