package tui

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/parth/watchy/internal/agent"
	"github.com/parth/watchy/internal/config"
	"github.com/parth/watchy/internal/task"
	"github.com/parth/watchy/internal/tick"
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

type slashCommand struct {
	name string
	desc string
}

var slashCommands = []slashCommand{
	{"/model", "Show or change the model"},
	{"/save", "Save a command as a tick"},
	{"/new", "Clear chat and start fresh"},
}

// Model is the root bubbletea model
type Model struct {
	mgr          *task.Manager
	agent        *agent.Agent
	conversation *agent.Conversation
	cfg          *config.Config
	tickStore    *tick.Store

	tasks       []*task.Task
	selectedIdx int
	activePane  pane
	rightMode   mode
	leftHidden  bool
	themeIdx    int

	logViewport  viewport.Model
	chatViewport viewport.Model
	chatInput    textarea.Model

	chatHistory    []chatMessage
	agentBusy      bool
	agentCancel    context.CancelFunc
	programRef     *programRef
	slashPickerIdx int
	width          int
	height         int

	// Log search state
	searchMode         bool
	searchInput        textinput.Model
	searchTerm         string
	searchMatches      []int
	matchIndex         int
	originalLogContent string
}

// New creates a new TUI model
func New(mgr *task.Manager, ag *agent.Agent, cfg *config.Config, tickStore *tick.Store) Model {
	ti := textarea.New()
	ti.Placeholder = "Ask the agent..."
	ti.SetHeight(3)
	ti.ShowLineNumbers = false

	si := textinput.New()
	si.Placeholder = "Search..."
	si.Prompt = "/"
	si.Width = 30

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
		tickStore:    tickStore,
		activePane:   paneLeft,
		rightMode:    modeLog,
		themeIdx:     themeIdx,
		logViewport:  viewport.New(0, 0),
		chatViewport: viewport.New(0, 0),
		chatInput:    ti,
		searchInput:  si,
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
