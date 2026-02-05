package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/parth/watchy/internal/agent"
	"github.com/parth/watchy/internal/config"
	"github.com/parth/watchy/internal/ollama"
	"github.com/parth/watchy/internal/task"
	"github.com/parth/watchy/internal/tick"
	"github.com/parth/watchy/internal/tui"
)

// version is set via ldflags at build time: -ldflags "-X main.version=v0.2.0"
var version = "dev"

const (
	ollamaPort     = 11439
	ollamaCloudURL = "https://ollama.com"
)

func main() {
	// Check --version early before any setup
	for _, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-v" {
			fmt.Println(version)
			return
		}
	}

	cfg, err := config.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	storage, err := task.NewStorage(cfg.DBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
	defer storage.Close()

	mgr := task.NewManager(storage, cfg.LogsDir)

	// Sync task statuses on startup
	mgr.SyncTaskStatus()

	// Parse global flags
	args := os.Args[1:]
	onlineMode := false
	for i := 0; i < len(args); i++ {
		if args[i] == "--online" {
			onlineMode = true
			args = append(args[:i], args[i+1:]...)
			i--
		} else if args[i] == "--model" && i+1 < len(args) {
			cfg.Model = args[i+1]
			args = append(args[:i], args[i+2:]...)
			i--
		}
	}

	// Determine Ollama host
	var srv *ollama.Server
	ollamaHost := ""
	if onlineMode {
		ollamaHost = ollamaCloudURL
	} else {
		// Start managed Ollama server
		srv = ollama.NewServer(ollamaPort)
		if err := srv.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not start managed Ollama: %s\n", err)
			// ollamaHost stays empty, agent will fall back to environment
		} else {
			defer srv.Stop()
			if err := srv.WaitReady(); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: managed Ollama not ready: %s\n", err)
				srv.Stop()
			} else {
				ollamaHost = srv.Host()
			}
		}
	}

	tickStore, err := tick.NewStore(cfg.TicksPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading ticks: %s\n", err)
		os.Exit(1)
	}

	cmd := ""
	if len(args) >= 1 {
		cmd = args[0]
	}

	var subArgs []string
	if len(args) > 1 {
		subArgs = args[1:]
	}
	switch cmd {
	case "start":
		cmdStart(mgr, subArgs)
	case "stop":
		cmdStop(mgr, subArgs)
	case "list":
		cmdList(mgr)
	case "logs":
		cmdLogs(mgr, subArgs)
	case "ask":
		cmdAsk(mgr, cfg, ollamaHost, subArgs)
	case "cleanup":
		cmdCleanup(mgr, cfg)
	case "tick":
		cmdTick(tickStore, subArgs)
	case "":
		cmdTUI(mgr, cfg, ollamaHost)
	default:
		if tickStore.Has(cmd) {
			cmdRunTick(mgr, tickStore, cmd)
		} else {
			fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
			printUsage()
			os.Exit(1)
		}
	}
}

func printUsage() {
	fmt.Println(`Usage: watchy [--online] [--model <model>] [command] [args]

Running watchy with no command launches the interactive TUI.

Global flags:
  --online              Use ollama.com cloud API instead of local server
  --model <model>       Specify which model to use
  --version, -v         Print version and exit

Commands:
  start <command> [--name <name>]   Start a background task
  stop <task-id>                    Stop a running task
  list                              List all tasks
  logs <task-id> [-n <lines>]       View task logs
  ask <task-id> "<question>"        Ask the AI agent about a task
  cleanup                           Clean up old completed tasks
  tick save <name> <command>        Save a command as a named tick
  tick list                         List all saved ticks
  tick rm <name>                    Remove a saved tick
  <tick-name>                       Run a saved tick as a task`)
}

func cmdStart(mgr *task.Manager, args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Error: command is required")
		os.Exit(1)
	}

	name := ""
	command := ""

	// Parse --name flag
	for i := 0; i < len(args); i++ {
		if args[i] == "--name" && i+1 < len(args) {
			name = args[i+1]
			i++
		} else {
			if command != "" {
				command += " "
			}
			command += args[i]
		}
	}

	if name == "" {
		name = command
		if len(name) > 40 {
			name = name[:40] + "..."
		}
	}

	taskID, err := mgr.StartTask(name, command)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	fmt.Printf("Started task %d: %s\n", taskID, name)
}

func cmdStop(mgr *task.Manager, args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Error: task ID is required")
		os.Exit(1)
	}

	id, err := strconv.Atoi(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid task ID: %s\n", args[0])
		os.Exit(1)
	}

	if err := mgr.StopTask(id); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	fmt.Printf("Stopped task %d\n", id)
}

func cmdList(mgr *task.Manager) {
	tasks, err := mgr.ListTasks()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	if len(tasks) == 0 {
		fmt.Println("No tasks")
		return
	}

	fmt.Printf("%-4s %-10s %-30s %-8s %s\n", "ID", "STATUS", "NAME", "PID", "STARTED")
	fmt.Println(strings.Repeat("-", 80))
	for _, t := range tasks {
		fmt.Printf("%-4d %-10s %-30s %-8d %s\n",
			t.ID, t.Status, truncate(t.Name, 30), t.PID,
			t.StartTime.Format("2006-01-02 15:04:05"))
	}
}

func cmdLogs(mgr *task.Manager, args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Error: task ID is required")
		os.Exit(1)
	}

	id, err := strconv.Atoi(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid task ID: %s\n", args[0])
		os.Exit(1)
	}

	lines := 50
	for i := 1; i < len(args); i++ {
		if args[i] == "-n" && i+1 < len(args) {
			lines, err = strconv.Atoi(args[i+1])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: invalid line count: %s\n", args[i+1])
				os.Exit(1)
			}
			i++
		}
	}

	logLines, err := mgr.TailLogs(id, lines)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	for _, line := range logLines {
		fmt.Println(line)
	}
}

func cmdAsk(mgr *task.Manager, cfg *config.Config, ollamaHost string, args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "Error: task ID and question are required")
		fmt.Fprintln(os.Stderr, "Usage: watchy ask <task-id> \"<question>\"")
		os.Exit(1)
	}

	id, err := strconv.Atoi(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid task ID: %s\n", args[0])
		os.Exit(1)
	}

	question := strings.Join(args[1:], " ")

	a, err := agent.NewAgentWithModel(mgr, cfg.Model, ollamaHost)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	fmt.Println("Asking agent...")
	answer, err := a.Ask(id, question)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	fmt.Println(answer)
}

func cmdTUI(mgr *task.Manager, cfg *config.Config, ollamaHost string) {
	a, err := agent.NewAgentWithModel(mgr, cfg.Model, ollamaHost)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating agent: %s\n", err)
		os.Exit(1)
	}

	model := tui.New(mgr, a, cfg)
	p := tea.NewProgram(model, tea.WithAltScreen())
	model.SetProgram(p)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

func cmdCleanup(mgr *task.Manager, cfg *config.Config) {
	count, err := mgr.Cleanup(cfg.RetentionDays)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	fmt.Printf("Cleaned up %d old task(s)\n", count)
}

func cmdTick(store *tick.Store, args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage:")
		fmt.Fprintln(os.Stderr, "  watchy tick save <name> <command>")
		fmt.Fprintln(os.Stderr, "  watchy tick list")
		fmt.Fprintln(os.Stderr, "  watchy tick rm <name>")
		os.Exit(1)
	}

	switch args[0] {
	case "save":
		cmdTickSave(store, args[1:])
	case "list":
		cmdTickList(store)
	case "rm":
		cmdTickRm(store, args[1:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown tick subcommand: %s\n", args[0])
		os.Exit(1)
	}
}

func cmdTickSave(store *tick.Store, args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: watchy tick save <name> <command>")
		os.Exit(1)
	}

	name := args[0]
	command := strings.Join(args[1:], " ")

	if err := store.Save(name, command, ""); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	fmt.Printf("Saved tick %q: %s\n", name, command)
}

func cmdTickList(store *tick.Store) {
	ticks := store.List()
	if len(ticks) == 0 {
		fmt.Println("No ticks saved")
		fmt.Println("Save one with: watchy tick save <name> <command>")
		return
	}

	fmt.Printf("%-15s %s\n", "NAME", "COMMAND")
	fmt.Println(strings.Repeat("-", 60))
	for _, t := range ticks {
		fmt.Printf("%-15s %s\n", t.Name, t.Tick.Command)
	}
}

func cmdTickRm(store *tick.Store, args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: watchy tick rm <name>")
		os.Exit(1)
	}

	if err := store.Remove(args[0]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	fmt.Printf("Removed tick %q\n", args[0])
}

func cmdRunTick(mgr *task.Manager, store *tick.Store, name string) {
	t, err := store.Get(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	taskID, err := mgr.StartTask(name, t.Command)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	fmt.Printf("Started tick %q as task %d: %s\n", name, taskID, t.Command)
	fmt.Printf("View logs: watchy logs %d\n", taskID)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
