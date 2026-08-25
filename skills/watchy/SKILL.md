---
name: watchy
description: Run and monitor persistent background processes with the locally installed Watchy CLI. Use when starting a development server, worker, build watcher, database, benchmark, eval, or other long-running command that must stay alive after the current tool call, especially when the user wants quick access to status and logs.
---

# Watchy

Use Watchy to keep long-running processes alive and make their logs easy to inspect. Do not use it for ordinary short commands.

## Start a process

1. Confirm `watchy` is installed with `command -v watchy`.
2. Run `watchy list` and check the requested port when applicable. Do not start an obvious duplicate. For parseable state in a tool call, use `watchy list --json` or `watchy info <id> --json` (fields include `command`, `work_dir`, `status`, `pid`, `log_path`, and `exit_code`).
3. Launch Watchy from the directory in which the child command must run. Set the execution tool's `workdir`; Watchy passes its working directory to the child.
4. Use a short, specific name:

```bash
watchy start 'npm run dev -- --port 8081' --name site-8081
```

5. Capture the task ID from `Started task <id>`.
6. Check startup immediately:

```bash
watchy list
watchy logs <id> -n 80
```

7. If the process serves a local endpoint, perform the smallest relevant health request after the logs indicate readiness.

Keep a successfully started process running unless the user asks to stop it or the task is explicitly temporary.

## Monitor and diagnose

Use bounded log reads by default:

```bash
watchy logs <id> -n 100
```

Poll `watchy list` and logs at useful intervals for longer startups. If a task crashes, report its status and the relevant log tail (and the exit code from `watchy info <id>` when useful). Do not restart it repeatedly without understanding the failure.

To restart a stopped or crashed task once the failure is understood:

```bash
watchy restart <id>
```

This starts a new task (new ID) with the same command and working directory; capture the new ID from the output.

Use `watchy ask <id> "<question>"` only when model-assisted log analysis is useful. `ask` is read-only (it cannot start, stop, or restart tasks) and times out after 30 seconds; it prints `Asking agent…` to stdout before the answer, so capture the answer from the lines that follow. Do not add `--online` or change the model unless the user requests that mode.

## Stop a process

Always provide the explicit task ID:

```bash
watchy stop <id>
```

Never use bare `watchy stop`; it stops the latest task, which might be unrelated.

## Safety

- Treat the command passed to `watchy start` as real shell execution. Watchy runs it through `bash -c`.
- Do not include secrets in command arguments. Commands and logs persist under `~/.watchy`.
- Do not run `watchy cleanup` without explicit user intent; it removes historical task records and logs.
- Do not create, replace, or remove ticks unless the user asks.
- Do not launch bare `watchy` from a non-interactive tool call; it opens the TUI.
- A warning that managed Ollama is not ready can precede normal task-management output. Judge the command by its exit status and resulting task output.

## Handoff

Report:

- task ID and name;
- running, stopped, or crashed status (and exit code for crashed tasks);
- working directory and command;
- local URL or port, when applicable;
- the task's log path under `~/.watchy/`;
- `watchy logs <id> -n 100` as the follow-up command.
