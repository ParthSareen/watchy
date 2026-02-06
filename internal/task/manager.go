package task

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

type Manager struct {
	storage *Storage
	logsDir string
}

// NewManager creates a new task manager
func NewManager(storage *Storage, logsDir string) *Manager {
	return &Manager{
		storage: storage,
		logsDir: logsDir,
	}
}

// StartTask starts a new background task
func (m *Manager) StartTask(name, command string) (int64, error) {
	if command == "" {
		return 0, fmt.Errorf("empty command")
	}

	// Create log file
	timestamp := time.Now().Format("20060102-150405")
	logPath := filepath.Join(m.logsDir, fmt.Sprintf("task-%s.log", timestamp))
	logFile, err := os.Create(logPath)
	if err != nil {
		return 0, fmt.Errorf("failed to create log file: %w", err)
	}

	// Always run through bash -c to handle complex commands
	cmd := exec.Command("bash", "-c", command)

	// Set process group to detach from parent
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	// Redirect stdout and stderr to log file
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	// Start the process
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return 0, fmt.Errorf("failed to start process: %w", err)
	}

	pid := cmd.Process.Pid

	// Save task to database
	taskID, err := m.storage.CreateTask(name, command, pid, logPath)
	if err != nil {
		// Try to kill the process if database save fails
		syscall.Kill(-pid, syscall.SIGTERM)
		logFile.Close()
		return 0, fmt.Errorf("failed to save task: %w", err)
	}

	// Close log file handle (process keeps it open)
	logFile.Close()

	// Start goroutine to wait for process completion
	go m.watchProcess(int(taskID), cmd)

	return taskID, nil
}

// watchProcess waits for a process to complete and updates status
func (m *Manager) watchProcess(taskID int, cmd *exec.Cmd) {
	err := cmd.Wait()

	status := "stopped"
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() != 0 {
			status = "crashed"
		}
	}

	m.storage.UpdateTaskStatus(taskID, status)
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

	// Kill the process group (negative PID)
	if err := syscall.Kill(-task.PID, syscall.SIGTERM); err != nil {
		// If SIGTERM fails, try SIGKILL
		if err := syscall.Kill(-task.PID, syscall.SIGKILL); err != nil {
			return fmt.Errorf("failed to kill process: %w", err)
		}
	}

	// Update status
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

// TailLogs reads the last N lines from a task's log file
func (m *Manager) TailLogs(id int, lines int) ([]string, error) {
	task, err := m.storage.GetTask(id)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(task.LogPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	// Read all lines
	var allLines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read log file: %w", err)
	}

	// Return last N lines
	if len(allLines) <= lines {
		return allLines, nil
	}
	return allLines[len(allLines)-lines:], nil
}

// CheckPID checks if a PID is still running
func (m *Manager) CheckPID(pid int) bool {
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
	for _, t := range tasks {
		os.Remove(t.LogPath)
		if err := m.storage.DeleteTask(t.ID); err != nil {
			continue
		}
		count++
	}

	return count, nil
}

// SyncTaskStatus synchronizes task status with actual process state
func (m *Manager) SyncTaskStatus() error {
	tasks, err := m.storage.ListTasks()
	if err != nil {
		return err
	}

	for _, task := range tasks {
		if task.Status == "running" {
			if !m.CheckPID(task.PID) {
				m.storage.UpdateTaskStatus(task.ID, "crashed")
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
	return m.StartTask(task.Name, task.Command)
}
