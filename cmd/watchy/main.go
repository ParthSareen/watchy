package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/parth/watchy/internal/agent"
	"github.com/parth/watchy/internal/config"
	"github.com/parth/watchy/internal/task"
	"github.com/parth/watchy/internal/tui"
)

func main() {
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

	// Parse global --model flag
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		if args[i] == "--model" && i+1 < len(args) {
			cfg.Model = args[i+1]
			args = append(args[:i], args[i+2:]...)
			break
		}
	}

	cmd := ""
	if len(args) >= 1 {
		cmd = args[0]
	}

	subArgs := args[1:]
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
		cmdAsk(mgr, cfg, subArgs)
	case "cleanup":
		cmdCleanup(mgr, cfg)
	case "":
		cmdTUI(mgr, cfg)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`Usage: watchy [command] [args]

Running watchy with no command launches the interactive TUI.

Commands:
  start <command> [--name <name>]   Start a background task
  stop <task-id>                    Stop a running task
  list                              List all tasks
  logs <task-id> [-n <lines>]       View task logs
  ask <task-id> "<question>"        Ask the AI agent about a task
  cleanup                           Clean up old completed tasks`)
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

func cmdAsk(mgr *task.Manager, cfg *config.Config, args []string) {
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

	a, err := agent.NewAgentWithModel(mgr, cfg.Model)
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

func cmdTUI(mgr *task.Manager, cfg *config.Config) {
	a, err := agent.NewAgentWithModel(mgr, cfg.Model)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating agent: %s\n", err)
		os.Exit(1)
	}

	model := tui.New(mgr, a)
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

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
