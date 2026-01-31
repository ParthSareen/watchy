package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/ollama/ollama/api"
)

func newProps(props map[string]api.ToolProperty) *api.ToolPropertiesMap {
	m := api.NewToolPropertiesMap()
	for k, v := range props {
		m.Set(k, v)
	}
	return m
}

// GetTools returns tool definitions for Ollama
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
				Description: "Execute a read-only bash command. Allowed: grep, tail, head, awk, sed, wc, cat, sort, uniq, cut, ls, find, ps, lsof, netstat, ss, df, du, free, uptime, whoami, hostname, uname, env, printenv, which, file, stat, id, curl, dig, ping. Pipes are supported.",
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
				Name:        "get_task_info",
				Description: "Get metadata about a task including its ID, name, command, PID, status, start time, and log file path",
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
	case "start_task":
		command, ok := args.Get("command")
		if !ok {
			return "", fmt.Errorf("missing 'command' argument")
		}
		name, _ := args.Get("name")
		return a.startTask(command.(string), name)
	case "stop_task":
		taskID, ok := args.Get("task_id")
		if !ok {
			return "", fmt.Errorf("missing 'task_id' argument")
		}
		return a.stopTask(toInt(taskID))
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

func (a *Agent) startTask(command string, nameVal interface{}) (string, error) {
	name := ""
	if s, ok := nameVal.(string); ok && s != "" {
		name = s
	} else {
		name = command
		if len(name) > 40 {
			name = name[:40] + "..."
		}
	}

	taskID, err := a.taskManager.StartTask(name, command)
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

func (a *Agent) getTaskInfo(taskID int) (string, error) {
	task, err := a.taskManager.GetTask(taskID)
	if err != nil {
		return "", err
	}

	info := map[string]interface{}{
		"id":         task.ID,
		"name":       task.Name,
		"command":    task.Command,
		"pid":        task.PID,
		"status":     task.Status,
		"start_time": task.StartTime.Format("2006-01-02 15:04:05"),
		"log_path":   task.LogPath,
	}

	if task.EndTime != nil {
		info["end_time"] = task.EndTime.Format("2006-01-02 15:04:05")
	}

	result, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return "", err
	}

	return string(result), nil
}
