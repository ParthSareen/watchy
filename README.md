# watchy 👀


Background task manager with a TUI and an LLM agent that can read logs, run diagnostics, and start/stop tasks on your behalf.

Uses Ollama for inference. The agent sees all your tasks and their log files, and has tools to read files, run shell commands, and manage tasks.

![TUI](TUI.png)

## Install

```
go install github.com/parth/watchy/cmd/watchy@latest
```

Requires [Ollama](https://ollama.com). Recommended models:

- `glm-4.7:cloud` -- default, runs via Ollama cloud
- `glm-4.7-flash` -- runs fully local

## Usage

Run `watchy` to open the TUI. Left pane shows tasks, right pane shows logs or chat.

```
watchy                              # launch TUI
watchy --online                     # launch TUI using ollama.com cloud
watchy --model llama3.1:8b          # launch TUI with a different model
watchy --version                    # print version
watchy start 'make serve'           # start a background task
watchy start 'npm test' --name ci   # start with a custom name
watchy stop 3                       # stop task 3
watchy list                         # list all tasks
watchy logs 3 -n 100                # last 100 lines of task 3
watchy ask 3 "any errors?"          # ask the agent about task 3
watchy cleanup                      # remove old finished tasks
```

The `--model` flag works with any command.

## TUI keybindings

### Navigation
```
j/k or ↑/↓   navigate task list / scroll logs / move cursor
g            go to top of logs
G            go to bottom of logs
tab          cycle: sidebar → logs → chat → sidebar
h            hide/show sidebar
enter        open logs for selected task / maximize logs pane
```

### Logs
```
l            show logs for selected task
/            search in logs
n            next search match
N            previous search match
v            toggle visual mode (select lines with cursor)
y            copy to clipboard (selection in visual mode, or all logs)
s            split view (logs + chat side-by-side)
```

### Tasks
```
x            stop selected task
r            restart stopped/crashed task
```

### Chat
```
c            open chat (focuses input immediately)
ctrl+left    switch from chat to logs
esc          cancel in-flight agent request
```

### Display
```
t            cycle theme (green/blue/purple/orange/pink/cyan/red/white)
m            toggle light/dark mode
```

### General
```
q            quit
ctrl+c       quit (works even in chat input)
```

## Chat commands

```
/model              show current model
/model llama3.1:8b  switch model mid-session
/save <name>        save last agent-started task as a tick
/save <name> <cmd>  save a command as a named tick
/new                clear chat history
```

## Agent tools

The agent has access to:

- `read_file` -- read any file by path
- `bash_command` -- run read-only shell commands (grep, tail, head, awk, sed, wc, cat, sort, uniq, cut)
- `get_task_info` -- get task metadata
- `start_task` -- start a new background task
- `stop_task` -- stop a running task

Ask it things like "start a web server on port 8080", "are there any errors in task 2?", or "grep for panics across all logs".

## Config

Optional config at `~/.watchy/config.yaml`:

```yaml
retention_days: 1
model: "glm-4.7:cloud"
theme: "green"
```

Available themes: `green`, `blue`, `purple`, `orange`, `pink`, `cyan`, `red`, `white`. Use `t` in the TUI to cycle through them, or `m` to toggle light/dark mode for your terminal background.

You can also set the model per-session with `--model` or `/model` in chat. Config file values are used as defaults.

Data lives in `~/.watchy/` (SQLite db + log files).
