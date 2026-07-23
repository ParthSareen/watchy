package task

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestManager(t *testing.T) (*Manager, *Storage, string) {
	t.Helper()
	dir := t.TempDir()
	storage, err := NewStorage(filepath.Join(dir, "watchy.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { storage.Close() })
	logsDir := filepath.Join(dir, "logs")
	if err := os.Mkdir(logsDir, 0755); err != nil {
		t.Fatal(err)
	}
	return NewManager(storage, logsDir), storage, dir
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}

func TestTaskCompletionPreservesDirectoryAndExitStatus(t *testing.T) {
	manager, storage, dir := newTestManager(t)
	taskID, err := manager.startTaskInDir("pwd", "pwd", dir)
	if err != nil {
		t.Fatal(err)
	}
	started, err := storage.GetTask(int(taskID))
	if err != nil {
		t.Fatal(err)
	}
	if started.WorkDir != dir {
		t.Fatalf("WorkDir = %q, want %q", started.WorkDir, dir)
	}

	waitForFile(t, taskExitPath(started.LogPath))
	if err := manager.SyncTaskStatus(); err != nil {
		t.Fatal(err)
	}
	finished, err := storage.GetTask(int(taskID))
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != "stopped" {
		t.Fatalf("Status = %q, want stopped", finished.Status)
	}
	logData, err := os.ReadFile(finished.LogPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(logData)) != dir {
		t.Fatalf("log = %q, want working directory", logData)
	}
}

func TestTaskLogPathsAreUnique(t *testing.T) {
	manager, storage, dir := newTestManager(t)
	firstID, err := manager.startTaskInDir("first", "true", dir)
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := manager.startTaskInDir("second", "true", dir)
	if err != nil {
		t.Fatal(err)
	}
	first, _ := storage.GetTask(int(firstID))
	second, _ := storage.GetTask(int(secondID))
	if first.LogPath == second.LogPath {
		t.Fatalf("tasks share log path %q", first.LogPath)
	}
	waitForFile(t, taskExitPath(first.LogPath))
	waitForFile(t, taskExitPath(second.LogPath))
}

func TestStopTaskCannotBeOverwrittenAsCrashed(t *testing.T) {
	manager, storage, dir := newTestManager(t)
	taskID, err := manager.startTaskInDir("sleep", "sleep 30", dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.StopTask(int(taskID)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	task, err := storage.GetTask(int(taskID))
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != "stopped" {
		t.Fatalf("Status = %q, want stopped", task.Status)
	}
}

func TestFollowLogsIsBoundedAndReadsAppends(t *testing.T) {
	manager, storage, dir := newTestManager(t)
	logPath := filepath.Join(dir, "logs", "manual.log")
	if err := os.WriteFile(logPath, []byte("one\ntwo\nthree\n"), 0644); err != nil {
		t.Fatal(err)
	}
	taskID, err := storage.CreateTask("manual", "true", dir, os.Getpid(), logPath)
	if err != nil {
		t.Fatal(err)
	}

	first, err := manager.FollowLogs(int(taskID), 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := logContents(first.Lines); got != "two\nthree" {
		t.Fatalf("first snapshot = %q", got)
	}
	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("four\n"); err != nil {
		file.Close()
		t.Fatal(err)
	}
	file.Close()

	second, err := manager.FollowLogs(int(taskID), 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := logContents(second.Lines); got != "three\nfour" {
		t.Fatalf("second snapshot = %q", got)
	}
	if second.Lines[0].LineNum != 3 || second.Lines[1].LineNum != 4 {
		t.Fatalf("line numbers = %v", second.Lines)
	}

	shrunk, err := manager.FollowLogs(int(taskID), 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := logContents(shrunk.Lines); got != "four" {
		t.Fatalf("shrunk snapshot = %q", got)
	}
}

func TestTailLogsAcceptsLargeLines(t *testing.T) {
	manager, storage, dir := newTestManager(t)
	logPath := filepath.Join(dir, "logs", "large.log")
	largeLine := strings.Repeat("x", 128*1024)
	if err := os.WriteFile(logPath, []byte(largeLine+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	taskID, err := storage.CreateTask("large", "true", dir, os.Getpid(), logPath)
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.TailLogs(int(taskID), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Lines) != 1 || result.Lines[0].Content != largeLine {
		t.Fatalf("large line was not preserved")
	}
}

func TestStopTaskRejectsInvalidPID(t *testing.T) {
	manager, storage, dir := newTestManager(t)
	logPath := filepath.Join(dir, "logs", "invalid-pid.log")
	if err := os.WriteFile(logPath, nil, 0644); err != nil {
		t.Fatal(err)
	}
	taskID, err := storage.CreateTask("invalid", "true", dir, 0, logPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.StopTask(int(taskID)); err == nil || !strings.Contains(err.Error(), "invalid PID") {
		t.Fatalf("StopTask error = %v, want invalid PID", err)
	}
}

func logContents(lines []LogLine) string {
	content := make([]string, len(lines))
	for i, line := range lines {
		content[i] = line.Content
	}
	return strings.Join(content, "\n")
}
