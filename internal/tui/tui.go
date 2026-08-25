package tui

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/parth/watchy/internal/agent"
	"github.com/parth/watchy/internal/config"
	"github.com/parth/watchy/internal/logcolor"
	"github.com/parth/watchy/internal/store"
	"github.com/parth/watchy/internal/task"
	"github.com/parth/watchy/internal/termstyle"
	"github.com/parth/watchy/internal/tick"
)

type pane int

const (
	paneLeft pane = iota
	paneRight
	paneChat
)

type mode int

const (
	modeLog mode = iota
	modeChat
	modeSplit
)

type focusTarget int

const (
	focusTasks focusTarget = iota
	focusLogs
	focusChatView
	focusChatInput
)

const maxChatMessages = 60

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
	historyStore *store.HistoryStore

	tasks         []*task.Task
	selectedIdx   int
	focusedArea   focusTarget
	rightMode     mode
	leftHidden    bool
	themeIdx      int
	lightMode     bool
	tasksLoaded   bool
	pendingTaskID int64

	logs logsModel
	chat chatModel

	agentBusy   bool
	agentCancel context.CancelFunc
	programRef  *programRef

	modelPicker        bool
	modelPickerLoading bool
	modelPickerModels  []string
	modelPickerIdx     int
	modelPickerInput   textinput.Model
	modelPickerErr     error
	showHelp           bool
	showTaskDetails    bool

	statusMessage string
	statusError   bool
	statusSeq     int

	// Terminal dimensions
	width  int
	height int

	// Reactive layout dimensions (calculated in recalcLayout)
	boxHeight   int // total height for border boxes
	innerHeight int // height inside border (for viewport content)
	leftWidth   int // width of left pane (0 if hidden)
	rightWidth  int // width of right pane

	// Pending count for vim-style motions (e.g., 5j moves down 5 lines)
	pendingCount int

	// Task sidebar filter: when true only running tasks are shown
	filterRunning bool

	copied bool
}

// New creates a new TUI model
func New(mgr *task.Manager, ag *agent.Agent, cfg *config.Config, tickStore *tick.Store, historyStore *store.HistoryStore) Model {
	mi := textinput.New()
	mi.Prompt = "Filter: "
	mi.Placeholder = "type a model name"

	chat := newChatModel(maxChatMessages)
	conv := ag.NewConversation()
	for _, msg := range loadRecentHistory(historyStore, maxChatMessages) {
		chat.AppendHistory(msg.Role, msg.Content)
		conv.AppendHistory(msg.Role, msg.Content)
	}

	// Find theme index from config
	themeIdx := 0
	for i, t := range themes {
		if t.name == cfg.Theme {
			themeIdx = i
			break
		}
	}

	// Resolve light mode from config and terminal state, then sync color renderers.
	cfg.ColorMode = termstyle.NormalizeColorMode(cfg.ColorMode)
	lightMode := termstyle.ResolveLightMode(cfg.ColorMode)
	termstyle.ApplyLightMode(lightMode)
	logcolor.SetLightMode(lightMode)

	model := Model{
		mgr:              mgr,
		agent:            ag,
		conversation:     conv,
		cfg:              cfg,
		tickStore:        tickStore,
		historyStore:     historyStore,
		focusedArea:      focusTasks,
		rightMode:        modeLog,
		themeIdx:         themeIdx,
		lightMode:        lightMode,
		logs:             newLogsModel(),
		chat:             chat,
		modelPickerInput: mi,
		programRef:       &programRef{},
	}
	model.syncChatPalette()
	return model
}

type programRef struct {
	p *tea.Program
}

// SetProgram sets the tea.Program reference needed for streaming tool call events.
func (m *Model) SetProgram(p *tea.Program) {
	m.programRef.p = p
}

// loadRecentHistory loads the last n messages from history store.
func loadRecentHistory(hs *store.HistoryStore, n int) []store.ChatMessage {
	if hs == nil {
		return nil
	}
	return hs.Recent(n)
}

// appendChatMessage adds a message to chat history and persists it
func (m *Model) appendChatMessage(role, content string) {
	m.chat.AppendMessage(role, content)
	if m.historyStore != nil {
		if err := m.historyStore.Append(role, content); err != nil {
			m.statusMessage = "could not save chat history: " + err.Error()
			m.statusError = true
		}
	}
}

func (m *Model) appendToolStart(evt agent.ToolStartEvent) {
	m.chat.AppendToolStart(evt)
	if m.historyStore != nil {
		if err := m.historyStore.Append("tool", "["+evt.Tool+"] "+evt.Args); err != nil {
			m.statusMessage = "could not save tool history: " + err.Error()
			m.statusError = true
		}
	}
}

func (m *Model) appendToolResult(evt agent.ToolResultEvent) {
	m.chat.AppendToolResult(evt)
	if m.historyStore != nil {
		if err := m.historyStore.Append("tool", "-> "+limitText(evt.Result, 600)); err != nil {
			m.statusMessage = "could not save tool history: " + err.Error()
			m.statusError = true
		}
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		fetchTasks(m.mgr),
		tickEvery(2*time.Second),
		m.chat.Init(),
	)
}
