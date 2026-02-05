package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"time"

	"github.com/ollama/ollama/api"
	"github.com/parth/watchy/internal/task"
)

type Agent struct {
	client      *api.Client
	model       string
	taskManager *task.Manager
}

// NewAgent creates a new Ollama agent with the given Ollama host URL
func NewAgent(taskManager *task.Manager, ollamaHost string) (*Agent, error) {
	client, err := createClient(ollamaHost)
	if err != nil {
		return nil, err
	}

	return &Agent{
		client:      client,
		model:       "glm-4.7:cloud",
		taskManager: taskManager,
	}, nil
}

// NewAgentWithModel creates a new Ollama agent with a specific model and host
func NewAgentWithModel(taskManager *task.Manager, model string, ollamaHost string) (*Agent, error) {
	agent, err := NewAgent(taskManager, ollamaHost)
	if err != nil {
		return nil, err
	}
	if model != "" {
		agent.model = model
	}
	return agent, nil
}

// createClient creates an Ollama API client for the given host URL.
// If ollamaHost is empty, falls back to the environment-based client.
func createClient(ollamaHost string) (*api.Client, error) {
	if ollamaHost == "" {
		return api.ClientFromEnvironment()
	}

	baseURL, err := url.Parse(ollamaHost)
	if err != nil {
		return nil, fmt.Errorf("invalid ollama host URL: %w", err)
	}

	return api.NewClient(baseURL, http.DefaultClient), nil
}

// ToolStartEvent is emitted before a tool executes
type ToolStartEvent struct {
	Tool string
	Args string
}

// ToolResultEvent is emitted after a tool executes
type ToolResultEvent struct {
	Tool   string
	Result string
}

// SetModel changes the model used for inference
func (a *Agent) SetModel(model string) {
	a.model = model
}

// Model returns the current model name
func (a *Agent) Model() string {
	return a.model
}

// Conversation holds persistent chat state
type Conversation struct {
	agent    *Agent
	messages []api.Message
}

// NewConversation creates a new conversation with system prompt containing all tasks
func (a *Agent) NewConversation() *Conversation {
	c := &Conversation{agent: a}
	c.buildSystemPrompt()
	return c
}

func (c *Conversation) buildSystemPrompt() {
	allTasks, err := c.agent.taskManager.ListTasks()
	if err != nil {
		c.messages = []api.Message{{
			Role:    "system",
			Content: "You are a helpful assistant analyzing logs for background tasks. (Failed to load task list.)",
		}}
		return
	}

	var tasksContext string
	for _, t := range allTasks {
		tasksContext += fmt.Sprintf("  - [%d] %s | cmd: %s | status: %s | pid: %d | log: %s\n",
			t.ID, t.Name, t.Command, t.Status, t.PID, t.LogPath)
	}

	cwd, _ := os.Getwd()
	hostname, _ := os.Hostname()

	systemPrompt := fmt.Sprintf(`You are a helpful assistant managing and analyzing background tasks.
You have access to tools to read files, execute bash commands, get task info, start tasks, and stop tasks.

Environment:
  hostname: %s
  os: %s/%s
  cwd: %s
  shell: %s

All tasks:
%s
You are an operator. When the user asks you to do something, don't just answer -- do it.

Approach:
1. Figure out what's needed: read files, check running processes, inspect logs, look at the environment.
2. Do the work: start services, run setup scripts, install dependencies, configure things.
3. Verify it worked: check health endpoints, read logs for errors, confirm processes are running.
4. If something fails: read the logs, diagnose the issue, fix it, and retry. Keep going until it works or you've exhausted your options.

Don't ask the user what to do -- investigate and act. Use bash_command to explore the system, read_file to check configs and logs, start_task to run things in the background, and stop_task to kill broken processes.

Be concise. Show what you did and what happened, not what you could do.`, hostname, runtime.GOOS, runtime.GOARCH, cwd, os.Getenv("SHELL"), tasksContext)

	if len(c.messages) > 0 {
		c.messages[0] = api.Message{Role: "system", Content: systemPrompt}
	} else {
		c.messages = []api.Message{{Role: "system", Content: systemPrompt}}
	}
}

// RefreshSystemPrompt rebuilds the system prompt with current task state
func (c *Conversation) RefreshSystemPrompt() {
	c.buildSystemPrompt()
}

// SendWithEvents sends a message and streams tool call events back via the callback.
// The callback is called for each tool call. The final text response is returned.
// Pass a cancellable context to support aborting mid-request.
func (c *Conversation) SendWithEvents(ctx context.Context, message string, onToolStart func(ToolStartEvent), onToolResult func(ToolResultEvent)) (string, error) {
	c.messages = append(c.messages, api.Message{
		Role:    "user",
		Content: message,
	})

	c.trimContext()

	tools := GetTools()
	maxIterations := 10

	for i := 0; i < maxIterations; i++ {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}

		stream := false
		req := &api.ChatRequest{
			Model:    c.agent.model,
			Messages: c.messages,
			Tools:    tools,
			Stream:   &stream,
		}

		var lastMsg api.Message
		err := c.agent.client.Chat(ctx, req, func(resp api.ChatResponse) error {
			lastMsg = resp.Message
			return nil
		})
		if err != nil {
			return "", fmt.Errorf("chat request failed: %w", err)
		}

		c.messages = append(c.messages, lastMsg)

		if len(lastMsg.ToolCalls) == 0 {
			return lastMsg.Content, nil
		}

		for _, toolCall := range lastMsg.ToolCalls {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}

			argsBytes, _ := json.Marshal(toolCall.Function.Arguments)
			argsStr := string(argsBytes)

			if onToolStart != nil {
				onToolStart(ToolStartEvent{
					Tool: toolCall.Function.Name,
					Args: argsStr,
				})
			}

			result, err := c.agent.ExecuteTool(toolCall)
			if err != nil {
				result = fmt.Sprintf("Error executing tool: %s", err)
			}

			if onToolResult != nil {
				onToolResult(ToolResultEvent{
					Tool:   toolCall.Function.Name,
					Result: result,
				})
			}

			c.messages = append(c.messages, api.Message{
				Role:    "tool",
				Content: result,
			})
		}
	}

	return "", fmt.Errorf("agent exceeded maximum iterations")
}

// Send is a simple wrapper without events (used by CLI ask)
func (c *Conversation) Send(message string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return c.SendWithEvents(ctx, message, nil, nil)
}

// trimContext drops middle messages if estimated tokens exceed 16K
func (c *Conversation) trimContext() {
	const maxTokens = 16000
	const charsPerToken = 4

	totalChars := 0
	for _, m := range c.messages {
		totalChars += len(m.Content)
	}

	if totalChars/charsPerToken <= maxTokens {
		return
	}

	if len(c.messages) <= 21 {
		return
	}

	keep := make([]api.Message, 0, 21)
	keep = append(keep, c.messages[0])
	keep = append(keep, c.messages[1:5]...)
	keep = append(keep, c.messages[len(c.messages)-16:]...)
	c.messages = keep
}

// Ask is a convenience method for single-shot questions (used by CLI)
func (a *Agent) Ask(taskID int, question string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	focusedTask, err := a.taskManager.GetTask(taskID)
	if err != nil {
		return "", fmt.Errorf("failed to get task: %w", err)
	}

	allTasks, err := a.taskManager.ListTasks()
	if err != nil {
		return "", fmt.Errorf("failed to list tasks: %w", err)
	}

	var tasksContext string
	for _, t := range allTasks {
		marker := ""
		if t.ID == taskID {
			marker = " <-- FOCUSED"
		}
		tasksContext += fmt.Sprintf("  - [%d] %s | cmd: %s | status: %s | pid: %d | log: %s%s\n",
			t.ID, t.Name, t.Command, t.Status, t.PID, t.LogPath, marker)
	}

	systemPrompt := fmt.Sprintf(`You are a helpful assistant analyzing logs for background tasks.
You have access to tools to read files, execute bash commands, and get task information.

All tasks:
%s
The user is asking about task %d (%s), but you can reference any task above.

When the user asks questions, use your tools to investigate the logs and provide accurate answers.
You can use the read_file tool to read log files directly, or bash_command to run grep/tail/etc.

Be concise and helpful in your responses.`,
		tasksContext, focusedTask.ID, focusedTask.Name)

	messages := []api.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: question},
	}

	tools := GetTools()
	maxIterations := 10

	for i := 0; i < maxIterations; i++ {
		stream := false
		req := &api.ChatRequest{
			Model:    a.model,
			Messages: messages,
			Tools:    tools,
			Stream:   &stream,
		}

		var lastMsg api.Message
		err := a.client.Chat(ctx, req, func(resp api.ChatResponse) error {
			lastMsg = resp.Message
			return nil
		})
		if err != nil {
			return "", fmt.Errorf("chat request failed: %w", err)
		}

		messages = append(messages, lastMsg)

		if len(lastMsg.ToolCalls) == 0 {
			return lastMsg.Content, nil
		}

		for _, toolCall := range lastMsg.ToolCalls {
			result, err := a.ExecuteTool(toolCall)
			if err != nil {
				result = fmt.Sprintf("Error executing tool: %s", err)
			}
			messages = append(messages, api.Message{
				Role:    "tool",
				Content: result,
			})
		}
	}

	return "", fmt.Errorf("agent exceeded maximum iterations")
}
