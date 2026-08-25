package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/parth/watchy/internal/agent"
	"github.com/parth/watchy/internal/config"
	"github.com/parth/watchy/internal/ollama"
	"github.com/parth/watchy/internal/store"
	"github.com/parth/watchy/internal/task"
	"github.com/parth/watchy/internal/tick"
	"github.com/parth/watchy/internal/tui"
)

var version = "dev"

const (
	ollamaPort     = 11439
	ollamaCloudURL = "https://ollama.com"
)

type options struct {
	command   string
	args      []string
	model     string
	modelSet  bool
	online    bool
	help      bool
	showBuild bool
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	opts, err := parseOptions(args)
	if err != nil {
		return err
	}
	if opts.showBuild {
		fmt.Fprintln(stdout, version)
		return nil
	}
	if opts.help {
		printUsage(stdout)
		return nil
	}

	cfg, err := config.New()
	if err != nil {
		return err
	}
	if opts.modelSet {
		cfg.Model = opts.model
	}

	if opts.command == "tick" {
		tickStore, err := tick.NewStore(cfg.TicksPath)
		if err != nil {
			return fmt.Errorf("load ticks: %w", err)
		}
		return cmdTick(tickStore, opts.args, stdout)
	}

	var tickStore *tick.Store
	if opts.command == "" || !isBuiltInCommand(opts.command) {
		tickStore, err = tick.NewStore(cfg.TicksPath)
		if err != nil {
			return fmt.Errorf("load ticks: %w", err)
		}
	}
	if opts.command != "" && !isBuiltInCommand(opts.command) {
		if !tickStore.Has(opts.command) {
			return fmt.Errorf("unknown command %q; run watchy --help for usage", opts.command)
		}
	}

	storage, err := task.NewStorage(cfg.DBPath)
	if err != nil {
		return err
	}
	defer storage.Close()
	manager := task.NewManager(storage, cfg.LogsDir)
	if err := manager.SyncTaskStatus(); err != nil {
		return fmt.Errorf("sync task status: %w", err)
	}

	switch opts.command {
	case "start":
		return cmdStart(manager, opts.args, stdout)
	case "stop":
		return cmdStop(manager, opts.args, stdout)
	case "list":
		return cmdList(manager, opts.args, stdout)
	case "info":
		return cmdInfo(manager, opts.args, stdout)
	case "logs":
		return cmdLogs(manager, opts.args, stdout)
	case "restart":
		return cmdRestart(manager, opts.args, stdout)
	case "cleanup":
		return cmdCleanup(manager, cfg, stdout)
	case "ask":
		host, stop, err := ollamaHost(opts.online, stderr)
		if err != nil {
			return err
		}
		defer stop()
		return cmdAsk(manager, cfg, host, opts.args, stdout)
	case "":
		host, stop, err := ollamaHost(opts.online, stderr)
		if err != nil {
			return err
		}
		defer stop()
		historyStore, err := store.NewHistoryStore(cfg.HistoryPath)
		if err != nil {
			return fmt.Errorf("load history: %w", err)
		}
		return cmdTUI(manager, cfg, host, tickStore, historyStore)
	default:
		return cmdRunTick(manager, tickStore, opts.command, stdout)
	}
}

func parseOptions(args []string) (options, error) {
	var opts options
	remaining := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--online":
			opts.online = true
		case arg == "--help" || arg == "-h":
			opts.help = true
		case arg == "--version" || arg == "-v":
			opts.showBuild = true
		case arg == "--model":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return options{}, fmt.Errorf("--model requires a model name")
			}
			i++
			opts.model = args[i]
			opts.modelSet = true
		case strings.HasPrefix(arg, "--model="):
			opts.model = strings.TrimPrefix(arg, "--model=")
			if opts.model == "" {
				return options{}, fmt.Errorf("--model requires a model name")
			}
			opts.modelSet = true
		default:
			remaining = append(remaining, arg)
		}
	}
	if len(remaining) > 0 {
		opts.command = canonicalCommand(remaining[0])
		opts.args = remaining[1:]
	}
	return opts, nil
}

func canonicalCommand(command string) string {
	if command == "run" {
		return "start"
	}
	return command
}

func isBuiltInCommand(command string) bool {
	switch command {
	case "start", "stop", "list", "info", "logs", "restart", "ask", "cleanup":
		return true
	default:
		return false
	}
}

func ollamaHost(online bool, stderr io.Writer) (string, func(), error) {
	if online {
		return ollamaCloudURL, func() {}, nil
	}
	server := ollama.NewServer(ollamaPort)
	if err := server.Start(); err != nil {
		fmt.Fprintf(stderr, "Warning: could not start managed Ollama: %s\n", err)
		return "", func() {}, nil
	}
	if err := server.WaitReady(); err != nil {
		server.Stop()
		return "", func() {}, fmt.Errorf("start Ollama: %w", err)
	}
	return server.Host(), func() {
		if err := server.Stop(); err != nil {
			fmt.Fprintf(stderr, "Warning: could not stop managed Ollama: %s\n", err)
		}
	}, nil
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, `Usage: watchy [--online] [--model <model>] [command] [args]

Running watchy with no command launches the interactive TUI.

Global flags:
  --online              Use ollama.com instead of local Ollama
  --model <model>       Override the model for this session
  --version, -v         Print version and exit
  --help, -h            Show this help

Commands:
  start <command> [--name <name>]   Start a background task
  run <command> [--name <name>]     Alias for start
  stop [task-id]                    Stop a task (default: latest)
  list [--json]                     List all tasks
  info <task-id> [--json]           Show details for a task
  logs [task-id] [-n <lines>]       View task logs
  restart <task-id>                 Restart a stopped or crashed task
  ask <task-id> "<question>"        Ask the agent about a task
  cleanup                           Remove expired finished tasks and logs
  tick save <name> <command>        Save a command as a named tick
  tick list                         List saved ticks
  tick rm <name>                    Remove a saved tick
  <tick-name>                       Run a saved tick as a task
`)
}

func cmdStart(manager *task.Manager, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("command is required")
	}
	name := ""
	commandArgs := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == "--name" {
			if i+1 >= len(args) {
				return fmt.Errorf("--name requires a value")
			}
			i++
			name = args[i]
			continue
		}
		commandArgs = append(commandArgs, args[i])
	}
	command := strings.Join(commandArgs, " ")
	if strings.TrimSpace(command) == "" {
		return fmt.Errorf("command is required")
	}
	if name == "" {
		name = truncate(command, 43)
	}
	taskID, err := manager.StartTask(name, command)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Started task %d: %s\n", taskID, name)
	return nil
}

func cmdStop(manager *task.Manager, args []string, stdout io.Writer) error {
	taskID, err := taskIDOrLatest(manager, args)
	if err != nil {
		return err
	}
	if err := manager.StopTask(taskID); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Stopped task %d\n", taskID)
	return nil
}

func cmdList(manager *task.Manager, args []string, stdout io.Writer) error {
	asJSON := false
	for _, a := range args {
		if a == "--json" {
			asJSON = true
		} else {
			return fmt.Errorf("unexpected argument %q", a)
		}
	}
	tasks, err := manager.ListTasks()
	if err != nil {
		return err
	}
	if asJSON {
		if tasks == nil {
			tasks = []*task.Task{}
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(tasksToInfo(tasks))
	}
	if len(tasks) == 0 {
		fmt.Fprintln(stdout, "No tasks")
		return nil
	}
	fmt.Fprintf(stdout, "%-4s %-10s %-30s %-8s %s\n", "ID", "STATUS", "NAME", "PID", "STARTED")
	fmt.Fprintln(stdout, strings.Repeat("-", 80))
	for _, task := range tasks {
		fmt.Fprintf(stdout, "%-4d %-10s %-30s %-8d %s\n",
			task.ID,
			task.Status,
			truncate(task.Name, 30),
			task.PID,
			task.StartTime.Format("2006-01-02 15:04:05"),
		)
	}
	return nil
}

// taskInfo is the machine- and human-readable representation of a task used by
// `watchy list --json` and `watchy info`.
type taskInfo struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Command  string `json:"command"`
	WorkDir  string `json:"work_dir"`
	Status   string `json:"status"`
	PID      int    `json:"pid"`
	Started  string `json:"started"`
	Ended    string `json:"ended,omitempty"`
	ExitCode *int   `json:"exit_code,omitempty"`
	LogPath  string `json:"log_path"`
}

func tasksToInfo(tasks []*task.Task) []taskInfo {
	out := make([]taskInfo, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, taskToInfo(t))
	}
	return out
}

func taskToInfo(t *task.Task) taskInfo {
	info := taskInfo{
		ID:      t.ID,
		Name:    t.Name,
		Command: t.Command,
		WorkDir: t.WorkDir,
		Status:  t.Status,
		PID:     t.PID,
		Started: t.StartTime.Format("2006-01-02 15:04:05"),
		LogPath: t.LogPath,
	}
	if t.EndTime != nil {
		info.Ended = t.EndTime.Format("2006-01-02 15:04:05")
	}
	if code, ok := task.ExitCode(t.LogPath); ok {
		info.ExitCode = &code
	}
	return info
}

func cmdInfo(manager *task.Manager, args []string, stdout io.Writer) error {
	asJSON := false
	var idArgs []string
	for _, a := range args {
		if a == "--json" {
			asJSON = true
		} else {
			idArgs = append(idArgs, a)
		}
	}
	taskID, err := taskIDOrLatest(manager, idArgs)
	if err != nil {
		return err
	}
	t, err := manager.GetTask(taskID)
	if err != nil {
		return err
	}
	info := taskToInfo(t)
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(info)
	}
	fmt.Fprintf(stdout, "ID:       %d\n", info.ID)
	fmt.Fprintf(stdout, "Name:     %s\n", info.Name)
	fmt.Fprintf(stdout, "Status:   %s\n", info.Status)
	fmt.Fprintf(stdout, "PID:      %d\n", info.PID)
	fmt.Fprintf(stdout, "Started:  %s\n", info.Started)
	if info.Ended != "" {
		fmt.Fprintf(stdout, "Ended:    %s\n", info.Ended)
	}
	if info.ExitCode != nil {
		fmt.Fprintf(stdout, "ExitCode: %d\n", *info.ExitCode)
	}
	fmt.Fprintf(stdout, "Dir:      %s\n", info.WorkDir)
	fmt.Fprintf(stdout, "Log:      %s\n", info.LogPath)
	fmt.Fprintf(stdout, "Command:  %s\n", info.Command)
	return nil
}

func cmdRestart(manager *task.Manager, args []string, stdout io.Writer) error {
	taskID, err := taskIDOrLatest(manager, args)
	if err != nil {
		return err
	}
	newID, err := manager.RestartTask(taskID)
	if err != nil {
		return err
	}
	t, err := manager.GetTask(int(newID))
	if err == nil {
		fmt.Fprintf(stdout, "Restarted task %d as task %d: %s\n", taskID, newID, t.Name)
		fmt.Fprintf(stdout, "View logs: watchy logs %d\n", newID)
	} else {
		fmt.Fprintf(stdout, "Restarted task %d as task %d\n", taskID, newID)
	}
	return nil
}

func cmdLogs(manager *task.Manager, args []string, stdout io.Writer) error {
	lines := 50
	var taskArgs []string
	for i := 0; i < len(args); i++ {
		if args[i] != "-n" {
			taskArgs = append(taskArgs, args[i])
			continue
		}
		if i+1 >= len(args) {
			return fmt.Errorf("-n requires a line count")
		}
		i++
		value, err := strconv.Atoi(args[i])
		if err != nil || value <= 0 {
			return fmt.Errorf("invalid line count %q", args[i])
		}
		lines = value
	}
	taskID, err := taskIDOrLatest(manager, taskArgs)
	if err != nil {
		return err
	}
	result, err := manager.TailLogs(taskID, lines)
	if err != nil {
		return err
	}
	for _, line := range result.Lines {
		fmt.Fprintln(stdout, line.Content)
	}
	return nil
}

func taskIDOrLatest(manager *task.Manager, args []string) (int, error) {
	if len(args) == 0 {
		task, err := manager.GetLatestTask()
		if err != nil {
			return 0, err
		}
		return task.ID, nil
	}
	if len(args) > 1 {
		return 0, fmt.Errorf("unexpected argument %q", args[1])
	}
	taskID, err := strconv.Atoi(args[0])
	if err != nil || taskID <= 0 {
		return 0, fmt.Errorf("invalid task ID %q", args[0])
	}
	return taskID, nil
}

func cmdAsk(manager *task.Manager, cfg *config.Config, host string, args []string, stdout io.Writer) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: watchy ask <task-id> \"<question>\"")
	}
	taskID, err := strconv.Atoi(args[0])
	if err != nil || taskID <= 0 {
		return fmt.Errorf("invalid task ID %q", args[0])
	}
	ollamaAgent, err := agent.NewAgentWithModel(manager, cfg.Model, host)
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, "Asking agent…")
	answer, err := ollamaAgent.Ask(taskID, strings.Join(args[1:], " "))
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, answer)
	return nil
}

func cmdTUI(
	manager *task.Manager,
	cfg *config.Config,
	host string,
	tickStore *tick.Store,
	historyStore *store.HistoryStore,
) error {
	ollamaAgent, err := agent.NewAgentWithModel(manager, cfg.Model, host)
	if err != nil {
		return fmt.Errorf("create agent: %w", err)
	}
	model := tui.New(manager, ollamaAgent, cfg, tickStore, historyStore)
	program := tea.NewProgram(&model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	model.SetProgram(program)
	if _, err := program.Run(); err != nil {
		return err
	}
	return nil
}

func cmdCleanup(manager *task.Manager, cfg *config.Config, stdout io.Writer) error {
	count, err := manager.Cleanup(cfg.RetentionDays)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Cleaned up %d old task(s)\n", count)
	return nil
}

func cmdTick(store *tick.Store, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: watchy tick <save|list|rm>")
	}
	switch args[0] {
	case "save":
		if len(args) < 3 {
			return fmt.Errorf("usage: watchy tick save <name> <command>")
		}
		name := args[1]
		command := strings.Join(args[2:], " ")
		if err := store.Save(name, command, ""); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "Saved tick %q: %s\n", name, command)
		return nil
	case "list":
		ticks := store.List()
		if len(ticks) == 0 {
			fmt.Fprintln(stdout, "No ticks saved")
			fmt.Fprintln(stdout, "Save one with: watchy tick save <name> <command>")
			return nil
		}
		fmt.Fprintf(stdout, "%-15s %s\n", "NAME", "COMMAND")
		fmt.Fprintln(stdout, strings.Repeat("-", 60))
		for _, namedTick := range ticks {
			fmt.Fprintf(stdout, "%-15s %s\n", namedTick.Name, namedTick.Tick.Command)
		}
		return nil
	case "rm":
		if len(args) != 2 {
			return fmt.Errorf("usage: watchy tick rm <name>")
		}
		if err := store.Remove(args[1]); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "Removed tick %q\n", args[1])
		return nil
	default:
		return fmt.Errorf("unknown tick subcommand %q", args[0])
	}
}

func cmdRunTick(manager *task.Manager, store *tick.Store, name string, stdout io.Writer) error {
	savedTick, err := store.Get(name)
	if err != nil {
		return err
	}
	taskID, err := manager.StartTask(name, savedTick.Command)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Started tick %q as task %d: %s\n", name, taskID, savedTick.Command)
	fmt.Fprintf(stdout, "View logs: watchy logs %d\n", taskID)
	return nil
}

func truncate(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}
