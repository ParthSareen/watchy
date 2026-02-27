package tui

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/parth/watchy/internal/agent"
	"github.com/parth/watchy/internal/config"
	"github.com/parth/watchy/internal/store"
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
	modeSplit
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
	mgr           *task.Manager
	agent         *agent.Agent
	conversation  *agent.Conversation
	cfg           *config.Config
	tickStore     *tick.Store
	historyStore  *store.HistoryStore

	tasks       []*task.Task
	selectedIdx int
	activePane  pane
	rightMode   mode
	leftHidden  bool
	themeIdx    int
	lightMode   bool

	logViewport  viewport.Model
	chatViewport viewport.Model
	chatInput    textarea.Model

	chatHistory    []chatMessage
	agentBusy      bool
	agentCancel    context.CancelFunc
	programRef     *programRef
	slashPickerIdx int

	// Terminal dimensions
	width  int
	height int

	// Reactive layout dimensions (calculated in recalcLayout)
	boxHeight   int // total height for border boxes
	innerHeight int // height inside border (for viewport content)
	leftWidth   int // width of left pane (0 if hidden)
	rightWidth  int // width of right pane

	// Log search state
	searchMode         bool
	searchInput        textinput.Model
	searchTerm         string
	searchMatches      []int
	matchIndex         int
	originalLogContent string
	rawLogContent      string // raw logs without colorization or line numbers
	copied             bool   // true briefly after copying to clipboard

	// Log cursor and visual selection (vim-style)
	logCursor    int  // current cursor line in log viewport
	visualMode   bool // true when in visual selection mode
	visualStart  int  // starting line for selection (only valid when visualMode)
}

// New creates a new TUI model
func New(mgr *task.Manager, ag *agent.Agent, cfg *config.Config, tickStore *tick.Store, historyStore *store.HistoryStore) Model {
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

	// Detect light mode from terminal
	lightMode := detectLightMode()

	return Model{
		mgr:           mgr,
		agent:         ag,
		conversation:  conv,
		cfg:           cfg,
		tickStore:     tickStore,
		historyStore:  historyStore,
		chatHistory:   loadRecentHistory(historyStore, 5),
		activePane:    paneLeft,
		rightMode:     modeLog,
		themeIdx:      themeIdx,
		lightMode:     lightMode,
		logViewport:   viewport.New(0, 0),
		chatViewport:  viewport.New(0, 0),
		chatInput:     ti,
		searchInput:   si,
		programRef:    &programRef{},
	}
}

type programRef struct {
	p *tea.Program
}

// SetProgram sets the tea.Program reference needed for streaming tool call events.
func (m Model) SetProgram(p *tea.Program) {
	m.programRef.p = p
}

// wrapLogContent wraps text to fit the viewport width for line wrapping.
func (m Model) wrapLogContent(content string) string {
	if m.logViewport.Width <= 0 {
		return content
	}
	return lipgloss.NewStyle().Width(m.logViewport.Width).Render(content)
}

// loadRecentHistory loads the last n messages from history store
func loadRecentHistory(hs *store.HistoryStore, n int) []chatMessage {
	if hs == nil {
		return nil
	}
	recent := hs.Recent(n)
	result := make([]chatMessage, len(recent))
	for i, msg := range recent {
		result[i] = chatMessage{role: msg.Role, content: msg.Content}
	}
	return result
}

// appendChatMessage adds a message to chat history and persists it
func (m *Model) appendChatMessage(role, content string) {
	m.chatHistory = append(m.chatHistory, chatMessage{role: role, content: content})
	// Keep only last 5 in memory
	if len(m.chatHistory) > 5 {
		m.chatHistory = m.chatHistory[len(m.chatHistory)-5:]
	}
	// Persist to store
	if m.historyStore != nil {
		m.historyStore.Append(role, content)
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		fetchTasks(m.mgr),
		tickEvery(2*time.Second),
	)
}
