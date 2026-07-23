package tui

import (
	"errors"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/parth/watchy/internal/agent"
	"github.com/parth/watchy/internal/task"
)

func TestModelPickerIncludesManualChoiceBeforeMatches(t *testing.T) {
	input := textinput.New()
	input.SetValue("llama")
	model := Model{
		modelPickerInput:  input,
		modelPickerModels: []string{"gemma3", "llama3.2", "llama3.3"},
	}
	choices := model.modelPickerChoices()
	if len(choices) != 3 {
		t.Fatalf("choices = %v", choices)
	}
	if choices[0] != "llama" || choices[1] != "llama3.2" || choices[2] != "llama3.3" {
		t.Fatalf("choices = %v", choices)
	}
}

func TestLoadedModelsSelectCurrentModel(t *testing.T) {
	agentModel, err := agent.NewAgentWithModel(nil, "llama3.2", "")
	if err != nil {
		t.Fatal(err)
	}
	model := Model{
		agent:            agentModel,
		modelPickerInput: textinput.New(),
	}
	updated, _ := model.Update(modelsLoadedMsg{models: []string{"gemma3", "llama3.2", "qwen3"}})
	got := updated.(Model)
	if got.modelPickerIdx != 1 {
		t.Fatalf("modelPickerIdx = %d, want 1", got.modelPickerIdx)
	}
}

func TestShowChatDoesNotFocusComposer(t *testing.T) {
	model := Model{
		rightMode:   modeLog,
		focusedArea: focusLogs,
		chat:        newChatModel(maxChatMessages),
	}
	model.showRightMode(modeChat)
	if model.focusedArea != focusChatView {
		t.Fatalf("focusedArea = %v, want focusChatView", model.focusedArea)
	}
	if model.chat.Focused() {
		t.Fatal("showing chat should not focus the composer")
	}
}

func TestTaskRefreshErrorPreservesExistingTasks(t *testing.T) {
	model := Model{
		tasks: []*task.Task{{ID: 1, Name: "serve"}},
		chat:  newChatModel(maxChatMessages),
	}
	updated, _ := model.Update(tasksUpdatedMsg{err: errors.New("database unavailable")})
	got := updated.(Model)
	if len(got.tasks) != 1 || got.tasks[0].ID != 1 {
		t.Fatalf("tasks changed after refresh failure: %v", got.tasks)
	}
	if !got.statusError {
		t.Fatal("refresh failure should be visible")
	}
}

func TestSearchUsesRawTextAndCanonicalVisibleLines(t *testing.T) {
	model := Model{
		searchTerm:        "error",
		allRawLogContent:  "ok\nERROR one\nok\nerror two",
		allLogContent:     "ok\n\x1b[31mERROR one\x1b[0m\nok\n\x1b[31merror two\x1b[0m",
		allLogLineNumbers: []int{10, 11, 12, 13},
		logViewport:       viewport.New(80, 20),
	}
	model.applySearchFilter()
	if model.rawLogContent != "ERROR one\nerror two" {
		t.Fatalf("rawLogContent = %q", model.rawLogContent)
	}
	if len(model.logLineNumbers) != 2 || model.logLineNumbers[0] != 11 || model.logLineNumbers[1] != 13 {
		t.Fatalf("line numbers = %v", model.logLineNumbers)
	}
	if model.totalLogLines() != 2 {
		t.Fatalf("totalLogLines = %d", model.totalLogLines())
	}
}
