package task

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestNewStorageMigratesExistingTasksWithWorkingDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watchy.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		CREATE TABLE tasks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			command TEXT NOT NULL,
			pid INTEGER,
			status TEXT CHECK(status IN ('running', 'stopped', 'crashed')) NOT NULL,
			start_time INTEGER NOT NULL,
			end_time INTEGER,
			log_path TEXT NOT NULL,
			created_at INTEGER NOT NULL
		);
		INSERT INTO tasks (name, command, pid, status, start_time, log_path, created_at)
		VALUES ('old', 'true', 1, 'stopped', ?, '/tmp/old.log', ?);
	`, time.Now().Unix(), time.Now().Unix())
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	db.Close()

	storage, err := NewStorage(path)
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	task, err := storage.GetLatestTask()
	if err != nil {
		t.Fatal(err)
	}
	if task.WorkDir != "" {
		t.Fatalf("migrated WorkDir = %q, want empty fallback", task.WorkDir)
	}
}

func TestNewStorageReadsExistingReadOnlyDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watchy.db")
	storage, err := NewStorage(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := storage.CreateTask("existing", "true", "/tmp", 1, "/tmp/existing.log"); err != nil {
		storage.Close()
		t.Fatal(err)
	}
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}

	readonly, err := NewStorage("file:" + path + "?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer readonly.Close()
	value, err := readonly.GetLatestTask()
	if err != nil {
		t.Fatal(err)
	}
	if value.Name != "existing" {
		t.Fatalf("task name = %q, want existing", value.Name)
	}
}
