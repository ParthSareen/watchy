package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/ollama/ollama/api"
	"github.com/parth/watchy/internal/task"
)

func newProps(props map[string]api.ToolProperty) *api.ToolPropertiesMap {
	m := api.NewToolPropertiesMap()
	for k, v := range props {
		m.Set(k, v)
	}
	return m
}

// GetTools returns the full tool set available to the interactive chat agent.
func GetTools() []api.Tool {
	return []api.Tool{
		{
			Type: "function",
			Function: api.ToolFunction{
				Name:        "read_file",
				Description: "Read the contents of a file given its absolute path. Use this to read log files or any other files on the system.",
				Parameters: api.ToolFunctionParameters{
					Type:     "object",
					Required: []string{"path"},
					Properties: newProps(map[string]api.ToolProperty{
						"path": {
							Type:        api.PropertyType{"string"},
							Description: "The absolute path to the file to read",
						},
					}),
				},
			},
		},
		{
			Type: "function",
			Function: api.ToolFunction{
				Name:        "bash_command",
				Description: "Execute a read-only bash command. Allowed: grep, tail, head, awk, sed, wc, cat, sort, uniq, cut, ls, find, ps, lsof, netstat, ss, df, du, free, uptime, whoami, hostname, uname, env, printenv, which, file, stat, id, curl, dig, ping, psql, cd, pkill. Pipes are supported.",
				Parameters: api.ToolFunctionParameters{
					Type:     "object",
					Required: []string{"command"},
					Properties: newProps(map[string]api.ToolProperty{
						"command": {
							Type:        api.PropertyType{"string"},
							Description: "The bash command to execute (e.g., 'grep ERROR /path/to/log', 'tail -n 20 /path/to/log')",
						},
					}),
				},
			},
		},
		{
			Type: "function",
			Function: api.ToolFunction{
				Name:        "list_tasks",
				Description: "List all current background tasks with their ID, name, command, status, PID, start time, and log file path. Use this to refresh your view of task state after starting or stopping tasks.",
				Parameters: api.ToolFunctionParameters{
					Type:       "object",
					Required:   []string{},
					Properties: api.NewToolPropertiesMap(),
				},
			},
		},
		{
			Type: "function",
			Function: api.ToolFunction{
				Name:        "start_task",
				Description: "Start a new background task. The command will run in the background and its output will be logged.",
				Parameters: api.ToolFunctionParameters{
					Type:     "object",
					Required: []string{"command"},
					Properties: newProps(map[string]api.ToolProperty{
						"command": {
							Type:        api.PropertyType{"string"},
							Description: "The shell command to run as a background task",
						},
						"name": {
							Type:        api.PropertyType{"string"},
							Description: "A short human-readable name for the task (optional, defaults to the command)",
						},
						"workdir": {
							Type:        api.PropertyType{"string"},
							Description: "Working directory to run the command in (optional, defaults to the current directory)",
						},
					}),
				},
			},
		},
		{
			Type: "function",
			Function: api.ToolFunction{
				Name:        "stop_task",
				Description: "Stop a running background task by its ID",
				Parameters: api.ToolFunctionParameters{
					Type:     "object",
					Required: []string{"task_id"},
					Properties: newProps(map[string]api.ToolProperty{
						"task_id": {
							Type:        api.PropertyType{"integer"},
							Description: "The ID of the task to stop",
						},
					}),
				},
			},
		},
		{
			Type: "function",
			Function: api.ToolFunction{
				Name:        "restart_task",
				Description: "Restart a stopped or crashed task with the same command. A running task is stopped first. Returns the new task ID and log path.",
				Parameters: api.ToolFunctionParameters{
					Type:     "object",
					Required: []string{"task_id"},
					Properties: newProps(map[string]api.ToolProperty{
						"task_id": {
							Type:        api.PropertyType{"integer"},
							Description: "The ID of the task to restart",
						},
					}),
				},
			},
		},
		{
			Type: "function",
			Function: api.ToolFunction{
				Name:        "get_task_info",
				Description: "Get metadata about a task including its ID, name, command, working directory, PID, status, start time, end time, exit code, and log file path.",
				Parameters: api.ToolFunctionParameters{
					Type:     "object",
					Required: []string{"task_id"},
					Properties: newProps(map[string]api.ToolProperty{
						"task_id": {
							Type:        api.PropertyType{"integer"},
							Description: "The ID of the task",
						},
					}),
				},
			},
		},
	}
}

// readOnlyTools are the tools safe for one-shot analysis (watchy ask): they can
// inspect state but not mutate it.
var readOnlyTools = map[string]bool{
	"read_file":     true,
	"bash_command":  true,
	"list_tasks":    true,
	"get_task_info": true,
}

// GetReadOnlyTools returns the read-only subset of the tool set, used by
// `watchy ask` so a one-shot analysis can't silently start or stop tasks.
func GetReadOnlyTools() []api.Tool {
	var out []api.Tool
	for _, t := range GetTools() {
		if readOnlyTools[t.Function.Name] {
			out = append(out, t)
		}
	}
	return out
}

// ExecuteTool executes a tool call and returns the result
func (a *Agent) ExecuteTool(toolCall api.ToolCall) (string, error) {
	args := &toolCall.Function.Arguments
	switch toolCall.Function.Name {
	case "read_file":
		path, ok := args.Get("path")
		if !ok {
			return "", fmt.Errorf("missing 'path' argument")
		}
		return a.readFile(path.(string))
	case "bash_command":
		command, ok := args.Get("command")
		if !ok {
			return "", fmt.Errorf("missing 'command' argument")
		}
		return a.bashCommand(command.(string))
	case "list_tasks":
		return a.listTasks()
	case "start_task":
		command, ok := args.Get("command")
		if !ok {
			return "", fmt.Errorf("missing 'command' argument")
		}
		name, _ := args.Get("name")
		workdir, _ := args.Get("workdir")
		return a.startTask(command.(string), name, workdir)
	case "stop_task":
		taskID, ok := args.Get("task_id")
		if !ok {
			return "", fmt.Errorf("missing 'task_id' argument")
		}
		return a.stopTask(toInt(taskID))
	case "restart_task":
		taskID, ok := args.Get("task_id")
		if !ok {
			return "", fmt.Errorf("missing 'task_id' argument")
		}
		return a.restartTask(toInt(taskID))
	case "get_task_info":
		taskID, ok := args.Get("task_id")
		if !ok {
			return "", fmt.Errorf("missing 'task_id' argument")
		}
		return a.getTaskInfo(toInt(taskID))
	default:
		return "", fmt.Errorf("unknown tool: %s", toolCall.Function.Name)
	}
}

func (a *Agent) readFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	// Limit to 10KB to prevent overwhelming the model
	if len(content) > 10240 {
		content = content[len(content)-10240:]
		return fmt.Sprintf("[... truncated to last 10KB ...]\n%s", string(content)), nil
	}

	return string(content), nil
}

func (a *Agent) bashCommand(command string) (string, error) {
	// Validate command is safe (whitelist approach)
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return "", fmt.Errorf("empty command")
	}

	safeCommands := map[string]bool{
		"grep": true, "tail": true, "head": true, "awk": true,
		"sed": true, "wc": true, "cat": true, "sort": true,
		"uniq": true, "cut": true, "ls": true, "find": true,
		"ps": true, "lsof": true, "netstat": true, "ss": true,
		"df": true, "du": true, "free": true, "uptime": true,
		"whoami": true, "hostname": true, "uname": true,
		"env": true, "printenv": true, "which": true,
		"file": true, "stat": true, "id": true,
		"curl": true, "dig": true, "ping": true,
		"psql": true, "cd": true, "pkill": true,
	}

	if !safeCommands[parts[0]] {
		return "", fmt.Errorf("command '%s' is not allowed. Only read-only commands are permitted", parts[0])
	}

	cmd := exec.Command("bash", "-c", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("Command failed: %s\nOutput: %s", err, string(output)), nil
	}

	if len(output) > 10240 {
		output = output[:10240]
		return fmt.Sprintf("%s\n[... truncated ...]", string(output)), nil
	}

	return string(output), nil
}

func toInt(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}

func (a *Agent) listTasks() (string, error) {
	tasks, err := a.taskManager.ListTasks()
	if err != nil {
		return "", fmt.Errorf("failed to list tasks: %w", err)
	}
	if len(tasks) == 0 {
		return "No tasks", nil
	}

	type taskSummary struct {
		ID      int    `json:"id"`
		Name    string `json:"name"`
		Command string `json:"command"`
		Status  string `json:"status"`
		PID     int    `json:"pid"`
		Started string `json:"started"`
		LogPath string `json:"log_path"`
	}

	summary := make([]taskSummary, 0, len(tasks))
	for _, t := range tasks {
		summary = append(summary, taskSummary{
			ID:      t.ID,
			Name:    t.Name,
			Command: t.Command,
			Status:  t.Status,
			PID:     t.PID,
			Started: t.StartTime.Format("2006-01-02 15:04:05"),
			LogPath: t.LogPath,
		})
	}

	result, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return "", err
	}
	return string(result), nil
}

func (a *Agent) startTask(command string, nameVal, workdirVal interface{}) (string, error) {
	name := ""
	if s, ok := nameVal.(string); ok && s != "" {
		name = s
	} else {
		name = command
		runes := []rune(name)
		if len(runes) > 40 {
			name = string(runes[:40]) + "..."
		}
	}

	workdir := ""
	if s, ok := workdirVal.(string); ok {
		workdir = s
	}

	var taskID int64
	var err error
	if workdir != "" {
		taskID, err = a.taskManager.StartTaskInDir(name, command, workdir)
	} else {
		taskID, err = a.taskManager.StartTask(name, command)
	}
	if err != nil {
		return "", fmt.Errorf("failed to start task: %w", err)
	}

	return fmt.Sprintf("Started task %d: %s", taskID, name), nil
}

func (a *Agent) stopTask(id int) (string, error) {
	if err := a.taskManager.StopTask(id); err != nil {
		return "", fmt.Errorf("failed to stop task: %w", err)
	}
	return fmt.Sprintf("Stopped task %d", id), nil
}

func (a *Agent) restartTask(id int) (string, error) {
	newID, err := a.taskManager.RestartTask(id)
	if err != nil {
		return "", fmt.Errorf("failed to restart task: %w", err)
	}
	t, err := a.taskManager.GetTask(int(newID))
	logPath := ""
	if err == nil {
		logPath = t.LogPath
	}
	return fmt.Sprintf("Restarted task %d as new task %d (log: %s)", id, newID, logPath), nil
}

func (a *Agent) getTaskInfo(taskID int) (string, error) {
	t, err := a.taskManager.GetTask(taskID)
	if err != nil {
		return "", err
	}

	info := map[string]interface{}{
		"id":         t.ID,
		"name":       t.Name,
		"command":    t.Command,
		"work_dir":   t.WorkDir,
		"pid":        t.PID,
		"status":     t.Status,
		"start_time": t.StartTime.Format("2006-01-02 15:04:05"),
		"log_path":   t.LogPath,
	}

	if t.EndTime != nil {
		info["end_time"] = t.EndTime.Format("2006-01-02 15:04:05")
	}
	if code, ok := task.ExitCode(t.LogPath); ok {
		info["exit_code"] = code
	}

	result, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return "", err
	}

	return string(result), nil
}
