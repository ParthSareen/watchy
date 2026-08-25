package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/parth/watchy/internal/config"
	"github.com/parth/watchy/internal/task"
)

func TestParseOptionsSeparatesGlobalFlags(t *testing.T) {
	opts, err := parseOptions([]string{"--online", "--model=llama3.2", "logs", "12", "-n", "5"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.online || !opts.modelSet || opts.model != "llama3.2" {
		t.Fatalf("options = %+v", opts)
	}
	if opts.command != "logs" || strings.Join(opts.args, " ") != "12 -n 5" {
		t.Fatalf("command = %q, args = %v", opts.command, opts.args)
	}
}

func TestParseOptionsRejectsMissingModel(t *testing.T) {
	if _, err := parseOptions([]string{"--model"}); err == nil {
		t.Fatal("expected missing model error")
	}
}

func TestParseOptionsStartAndRunAreEquivalent(t *testing.T) {
	var baseline options
	for i, spelling := range []string{"start", "run"} {
		opts, err := parseOptions([]string{spelling, "printf 'ok'", "--name", "demo"})
		if err != nil {
			t.Fatalf("parse %s: %v", spelling, err)
		}
		if i == 0 {
			baseline = opts
			continue
		}
		if opts.command != baseline.command || strings.Join(opts.args, "\x00") != strings.Join(baseline.args, "\x00") {
			t.Fatalf("%s options = %+v, want %+v", spelling, opts, baseline)
		}
	}
}

func TestRunStartAndRunShareBehavior(t *testing.T) {
	for _, spelling := range []string{"start", "run"} {
		t.Run(spelling, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			var stdout, stderr bytes.Buffer
			if err := run([]string{spelling, "true", "--name", "demo"}, &stdout, &stderr); err != nil {
				t.Fatal(err)
			}
			if stdout.String() != "Started task 1: demo\n" {
				t.Fatalf("stdout = %q", stdout.String())
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q", stderr.String())
			}
		})
	}
}

func TestRunStartAndRunShareValidation(t *testing.T) {
	for _, spelling := range []string{"start", "run"} {
		t.Run(spelling, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			var stdout, stderr bytes.Buffer
			err := run([]string{spelling, "true", "--name"}, &stdout, &stderr)
			if err == nil || err.Error() != "--name requires a value" {
				t.Fatalf("error = %v, want --name requires a value", err)
			}
			if stdout.Len() != 0 || stderr.Len() != 0 {
				t.Fatalf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
			}
		})
	}
}

func TestRunStartAndRunShareHelp(t *testing.T) {
	outputs := make(map[string]string)
	for _, spelling := range []string{"start", "run"} {
		var stdout, stderr bytes.Buffer
		if err := run([]string{spelling, "--help"}, &stdout, &stderr); err != nil {
			t.Fatalf("%s --help: %v", spelling, err)
		}
		if stderr.Len() != 0 {
			t.Fatalf("%s stderr = %q", spelling, stderr.String())
		}
		outputs[spelling] = stdout.String()
	}
	if outputs["run"] != outputs["start"] {
		t.Fatalf("run help differs from start help\nrun:\n%s\nstart:\n%s", outputs["run"], outputs["start"])
	}
}

func TestVersionDoesNotInitializeApplicationState(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--version"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(stdout.String()) != version {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func newCommandTestManager(t *testing.T) (*task.Manager, *task.Storage, string) {
	t.Helper()
	dir := t.TempDir()
	storage, err := task.NewStorage(filepath.Join(dir, "watchy.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { storage.Close() })
	logsDir := filepath.Join(dir, "logs")
	if err := os.Mkdir(logsDir, 0755); err != nil {
		t.Fatal(err)
	}
	return task.NewManager(storage, logsDir), storage, dir
}

func TestCmdShowPrintsExactTaskDetailsForMissingLog(t *testing.T) {
	manager, storage, dir := newCommandTestManager(t)
	logPath := filepath.Join(dir, "logs", "missing.log")
	taskID, err := storage.CreateTask("a name that must never be truncated", "go test ./...", dir, 1234, logPath)
	if err != nil {
		t.Fatal(err)
	}
	created, err := storage.GetTask(int(taskID))
	if err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := cmdShow(manager, []string{fmt.Sprint(taskID)}, true, &output); err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("Task %d: a name that must never be truncated\n\n"+
		"Status:                   running\n"+
		"Command:                  go test ./...\n"+
		"Working directory:        %s\n"+
		"Process group leader PID: 1234\n"+
		"Started:                  %s\n"+
		"Ended:                    running\n"+
		"Log path:                 %s\n"+
		"Log size:                 missing\n"+
		"Last log activity:        missing\n", taskID, dir, created.StartTime.Format(time.RFC3339), logPath)
	if output.String() != want {
		t.Fatalf("show output = %q, want %q", output.String(), want)
	}

	output.Reset()
	if err := cmdShow(manager, []string{fmt.Sprint(taskID), "--json"}, true, &output); err != nil {
		t.Fatal(err)
	}
	wantJSON := fmt.Sprintf(`{"status_fresh":true,"task":{"id":%d,"name":"a name that must never be truncated","status":"running","command":"go test ./...","work_dir":%q,"process_group_leader_pid":1234,"start_time":%q,"end_time":null,"log_path":%q,"log_exists":false,"log_size_bytes":null,"last_log_activity":null}}`+"\n",
		taskID, dir, created.StartTime.Format(time.RFC3339), logPath)
	if output.String() != wantJSON {
		t.Fatalf("show JSON = %q, want %q", output.String(), wantJSON)
	}
}

func TestCmdListWideAndJSONPreserveLongNames(t *testing.T) {
	manager, storage, dir := newCommandTestManager(t)
	name := "this task name is intentionally longer than the normal list column"
	logPath := filepath.Join(dir, "logs", "present.log")
	if err := os.WriteFile(logPath, []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}
	taskID, err := storage.CreateTask(name, "printf hello", dir, 4321, logPath)
	if err != nil {
		t.Fatal(err)
	}
	created, err := storage.GetTask(int(taskID))
	if err != nil {
		t.Fatal(err)
	}

	var wide bytes.Buffer
	if err := cmdList(manager, []string{"--wide"}, true, &wide); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(wide.String(), name) {
		t.Fatalf("wide list truncated name: %q", wide.String())
	}

	var jsonOutput bytes.Buffer
	if err := cmdList(manager, []string{"--json"}, true, &jsonOutput); err != nil {
		t.Fatal(err)
	}
	metadata, err := os.Stat(logPath)
	if err != nil {
		t.Fatal(err)
	}
	size := metadata.Size()
	activity := metadata.ModTime()
	want := fmt.Sprintf(`{"status_fresh":true,"tasks":[{"id":%d,"name":%q,"status":"running","command":"printf hello","work_dir":%q,"process_group_leader_pid":4321,"start_time":%q,"end_time":null,"log_path":%q,"log_exists":true,"log_size_bytes":%d,"last_log_activity":%q}]}`+"\n",
		taskID, name, dir, created.StartTime.Format(time.RFC3339), logPath, size, activity.Format(time.RFC3339Nano))
	if jsonOutput.String() != want {
		t.Fatalf("list JSON = %q, want %q", jsonOutput.String(), want)
	}
}

func TestRunListReadOnlyDatabaseWarnsAndReturnsPersistedTasks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := config.New(); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(home, ".watchy", "watchy.db")
	storage, err := task.NewStorage(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := storage.CreateTask("readonly", "sleep 30", home, -1, filepath.Join(home, ".watchy", "logs", "missing.log")); err != nil {
		storage.Close()
		t.Fatal(err)
	}
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dbPath, 0444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dbPath, 0644) })

	var stdout, stderr bytes.Buffer
	if err := run([]string{"list", "--json"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "task status may be stale") {
		t.Fatalf("stderr = %q, want stale-status warning", stderr.String())
	}
	if !strings.Contains(stdout.String(), `"status_fresh":false`) || !strings.Contains(stdout.String(), `"name":"readonly"`) {
		t.Fatalf("stdout = %q, want persisted task with stale status", stdout.String())
	}
}
