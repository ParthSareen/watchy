package task

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Manager struct {
	storage         *Storage
	logsDir         string
	mu              sync.Mutex
	intentionalStop map[int]bool
	logCursors      map[int]*logCursor
}

type logCursor struct {
	path           string
	offset         int64
	totalCompleted int
	lines          []LogLine
}

const taskRunnerScript = `bash -c "$1"
status=$?
tmp="$2.tmp.$$"
printf '%d\n' "$status" > "$tmp"
mv "$tmp" "$2"
exit "$status"`

// NewManager creates a new task manager
func NewManager(storage *Storage, logsDir string) *Manager {
	return &Manager{
		storage:         storage,
		logsDir:         logsDir,
		intentionalStop: make(map[int]bool),
		logCursors:      make(map[int]*logCursor),
	}
}

// StartTask starts a new background task
func (m *Manager) StartTask(name, command string) (int64, error) {
	workDir, err := os.Getwd()
	if err != nil {
		return 0, fmt.Errorf("get task working directory: %w", err)
	}
	return m.startTaskInDir(name, command, workDir)
}

// StartTaskInDir starts a background task with an explicit working directory.
func (m *Manager) StartTaskInDir(name, command, workDir string) (int64, error) {
	return m.startTaskInDir(name, command, workDir)
}

// ExitCode returns the recorded exit status for a task's log path. The second
// value is false when no exit status is available (e.g. the task is still running).
func ExitCode(logPath string) (int, bool) {
	code, err := readExitCode(taskExitPath(logPath))
	if err != nil {
		return 0, false
	}
	return code, true
}

func (m *Manager) startTaskInDir(name, command, workDir string) (int64, error) {
	if strings.TrimSpace(command) == "" {
		return 0, fmt.Errorf("empty command")
	}
	if workDir == "" {
		return 0, fmt.Errorf("empty working directory")
	}

	logFile, err := os.CreateTemp(m.logsDir, "task-*.log")
	if err != nil {
		return 0, fmt.Errorf("failed to create log file: %w", err)
	}
	logPath := logFile.Name()
	exitPath := taskExitPath(logPath)

	cmd := exec.Command("bash", "-c", taskRunnerScript, "watchy-task", command, exitPath)
	cmd.Dir = workDir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		logFile.Close()
		os.Remove(logPath)
		return 0, fmt.Errorf("failed to start process: %w", err)
	}

	pid := cmd.Process.Pid
	taskID, err := m.storage.CreateTask(name, command, workDir, pid, logPath)
	if err != nil {
		syscall.Kill(-pid, syscall.SIGTERM)
		logFile.Close()
		os.Remove(logPath)
		return 0, fmt.Errorf("failed to save task: %w", err)
	}

	logFile.Close()
	go m.watchProcess(int(taskID), cmd, exitPath)

	return taskID, nil
}

func (m *Manager) watchProcess(taskID int, cmd *exec.Cmd, exitPath string) {
	err := cmd.Wait()

	m.mu.Lock()
	intentional := m.intentionalStop[taskID]
	delete(m.intentionalStop, taskID)
	m.mu.Unlock()

	task, getErr := m.storage.GetTask(taskID)
	if getErr != nil || task.Status != "running" {
		return
	}

	status := "crashed"
	if intentional {
		status = "stopped"
	} else if exitCode, readErr := readExitCode(exitPath); readErr == nil && exitCode == 0 {
		status = "stopped"
	} else if err == nil {
		status = "stopped"
	}

	_ = m.storage.UpdateTaskStatus(taskID, status)
}

// StopTask stops a running task
func (m *Manager) StopTask(id int) error {
	task, err := m.storage.GetTask(id)
	if err != nil {
		return err
	}

	if task.Status != "running" {
		return fmt.Errorf("task %d is not running (status: %s)", id, task.Status)
	}
	if task.PID <= 0 {
		return fmt.Errorf("invalid PID %d for task %d", task.PID, id)
	}

	m.mu.Lock()
	m.intentionalStop[id] = true
	m.mu.Unlock()

	if err := syscall.Kill(-task.PID, syscall.SIGTERM); err != nil {
		if !errors.Is(err, syscall.ESRCH) {
			m.mu.Lock()
			delete(m.intentionalStop, id)
			m.mu.Unlock()
			return fmt.Errorf("stop task process group: %w", err)
		}
	}

	deadline := time.Now().Add(2 * time.Second)
	for processGroupRunning(task.PID) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if processGroupRunning(task.PID) {
		if err := syscall.Kill(-task.PID, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			return fmt.Errorf("kill task process group: %w", err)
		}
	}
	return m.storage.UpdateTaskStatus(id, "stopped")
}

// ListTasks lists all tasks
func (m *Manager) ListTasks() ([]*Task, error) {
	return m.storage.ListTasks()
}

// GetTask gets a task by ID
func (m *Manager) GetTask(id int) (*Task, error) {
	return m.storage.GetTask(id)
}

func (m *Manager) GetLatestTask() (*Task, error) {
	return m.storage.GetLatestTask()
}

// TailLogs reads the last N lines from a task's log file.
// If lines is 0, all lines are returned.
// LogLine represents a single line from a log file with its original line number
type LogLine struct {
	LineNum int    // Original 1-indexed line number in the file
	Content string // Line content
}

// TailLogsResult contains the result of tailing logs with line number information
type TailLogsResult struct {
	Lines []LogLine
}

func (m *Manager) TailLogs(id int, lines int) (*TailLogsResult, error) {
	task, err := m.storage.GetTask(id)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(task.LogPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	var allLines []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read log file: %w", err)
	}

	totalLines := len(allLines)

	if lines <= 0 || totalLines <= lines {
		// Return all lines
		result := make([]LogLine, totalLines)
		for i, content := range allLines {
			result[i] = LogLine{LineNum: i + 1, Content: content}
		}
		return &TailLogsResult{
			Lines: result,
		}, nil
	}

	// Return last N lines
	startIdx := totalLines - lines
	result := make([]LogLine, lines)
	for i := 0; i < lines; i++ {
		result[i] = LogLine{LineNum: startIdx + i + 1, Content: allLines[startIdx+i]}
	}
	return &TailLogsResult{
		Lines: result,
	}, nil
}

// FollowLogs returns a bounded snapshot and only reads bytes appended since the
// previous call for the same task.
func (m *Manager) FollowLogs(id, maxLines int) (*TailLogsResult, error) {
	if maxLines <= 0 {
		return nil, fmt.Errorf("max lines must be positive")
	}
	task, err := m.storage.GetTask(id)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(task.LogPath)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat log file: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	cursor := m.logCursors[id]
	if cursor == nil || cursor.path != task.LogPath || info.Size() < cursor.offset {
		cursor = &logCursor{path: task.LogPath}
		m.logCursors[id] = cursor
	}
	if _, err := file.Seek(cursor.offset, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek log file: %w", err)
	}

	reader := bufio.NewReader(file)
	partial := ""
	for {
		line, readErr := reader.ReadString('\n')
		if readErr == nil {
			cursor.offset += int64(len(line))
			cursor.totalCompleted++
			cursor.lines = append(cursor.lines, LogLine{
				LineNum: cursor.totalCompleted,
				Content: strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"),
			})
			if len(cursor.lines) > maxLines {
				cursor.lines = cursor.lines[len(cursor.lines)-maxLines:]
			}
			continue
		}
		if errors.Is(readErr, io.EOF) {
			partial = strings.TrimSuffix(line, "\r")
			break
		}
		return nil, fmt.Errorf("read log file: %w", readErr)
	}

	if len(cursor.lines) > maxLines {
		cursor.lines = append([]LogLine(nil), cursor.lines[len(cursor.lines)-maxLines:]...)
	}

	result := append([]LogLine(nil), cursor.lines...)
	totalLines := cursor.totalCompleted
	if partial != "" {
		totalLines++
		result = append(result, LogLine{LineNum: totalLines, Content: partial})
		if len(result) > maxLines {
			result = result[len(result)-maxLines:]
		}
	}
	return &TailLogsResult{Lines: result}, nil
}

// CheckPID checks if a PID is still running
func (m *Manager) CheckPID(pid int) bool {
	if pid <= 0 {
		return false
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// Send signal 0 to check if process exists
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// Cleanup removes old completed/crashed tasks and their log files
func (m *Manager) Cleanup(retentionDays int) (int, error) {
	tasks, err := m.storage.ListTasksOlderThan(retentionDays)
	if err != nil {
		return 0, err
	}

	count := 0
	var cleanupErrors []error
	for _, t := range tasks {
		if err := m.storage.DeleteTask(t.ID); err != nil {
			cleanupErrors = append(cleanupErrors, err)
			continue
		}
		count++
		m.mu.Lock()
		delete(m.logCursors, t.ID)
		m.mu.Unlock()
		for _, path := range []string{t.LogPath, taskExitPath(t.LogPath)} {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("remove %s: %w", filepath.Base(path), err))
			}
		}
	}

	return count, errors.Join(cleanupErrors...)
}

// SyncTaskStatus synchronizes task status with actual process state
func (m *Manager) SyncTaskStatus() error {
	tasks, err := m.storage.ListTasks()
	if err != nil {
		return err
	}

	for _, task := range tasks {
		if task.Status == "running" {
			if exitCode, err := readExitCode(taskExitPath(task.LogPath)); err == nil {
				status := "crashed"
				if exitCode == 0 {
					status = "stopped"
				}
				if err := m.storage.UpdateTaskStatus(task.ID, status); err != nil {
					return err
				}
				continue
			}
			if !m.CheckPID(task.PID) {
				if err := m.storage.UpdateTaskStatus(task.ID, "crashed"); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// RestartTask restarts a stopped or crashed task with the same command
func (m *Manager) RestartTask(id int) (int64, error) {
	task, err := m.GetTask(id)
	if err != nil {
		return 0, err
	}

	// If task is running, stop it first
	if task.Status == "running" {
		if err := m.StopTask(id); err != nil {
			return 0, fmt.Errorf("failed to stop running task: %w", err)
		}
	}

	// Start a new task with the same name and command
	workDir := task.WorkDir
	if workDir == "" {
		var err error
		workDir, err = os.Getwd()
		if err != nil {
			return 0, fmt.Errorf("get fallback working directory: %w", err)
		}
	}
	return m.startTaskInDir(task.Name, task.Command, workDir)
}

func taskExitPath(logPath string) string {
	return logPath + ".exit"
}

func readExitCode(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	exitCode, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("parse exit status: %w", err)
	}
	return exitCode, nil
}

func processGroupRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(-pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
