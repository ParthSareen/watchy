package tui

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/parth/watchy/internal/agent"
	"github.com/parth/watchy/internal/config"
	"github.com/parth/watchy/internal/task"
)

type pane int

const (
	paneLeft pane = iota
	paneRight
)

type mode int

const (
	modeLog mode = iota
	modeChat
)

type chatMessage struct {
	role    string // "user", "agent", or "tool"
	content string
}

// Model is the root bubbletea model
type Model struct {
	mgr          *task.Manager
	agent        *agent.Agent
	conversation *agent.Conversation
	cfg          *config.Config

	tasks       []*task.Task
	selectedIdx int
	activePane  pane
	rightMode   mode
	leftHidden  bool
	themeIdx    int

	logViewport  viewport.Model
	chatViewport viewport.Model
	chatInput    textarea.Model

	chatHistory  []chatMessage
	agentBusy    bool
	agentCancel  context.CancelFunc
	programRef   *programRef
	width        int
	height       int

}

// New creates a new TUI model
func New(mgr *task.Manager, ag *agent.Agent, cfg *config.Config) Model {
	ti := textarea.New()
	ti.Placeholder = "Ask the agent..."
	ti.SetHeight(3)
	ti.ShowLineNumbers = false

	conv := ag.NewConversation()

	// Find theme index from config
	themeIdx := 0
	for i, t := range themes {
		if t.name == cfg.Theme {
			themeIdx = i
			break
		}
	}

	return Model{
		mgr:          mgr,
		agent:        ag,
		conversation: conv,
		cfg:          cfg,
		activePane:   paneLeft,
		rightMode:    modeLog,
		themeIdx:     themeIdx,
		logViewport:  viewport.New(0, 0),
		chatViewport: viewport.New(0, 0),
		chatInput:    ti,
		programRef:   &programRef{},
	}
}

type programRef struct {
	p *tea.Program
}

// SetProgram sets the tea.Program reference needed for streaming tool call events.
func (m Model) SetProgram(p *tea.Program) {
	m.programRef.p = p
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		fetchTasks(m.mgr),
		tickEvery(2*time.Second),
	)
}
