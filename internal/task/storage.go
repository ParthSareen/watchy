package task

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type Storage struct {
	db *sql.DB
}

type Task struct {
	ID        int
	Name      string
	Command   string
	PID       int
	Status    string // "running", "stopped", "crashed"
	StartTime time.Time
	EndTime   *time.Time
	LogPath   string
	CreatedAt time.Time
}

// NewStorage creates a new Storage instance and initializes the database
func NewStorage(dbPath string) (*Storage, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	s := &Storage{db: db}
	if err := s.initSchema(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *Storage) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS tasks (
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
	`

	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	return nil
}

// CreateTask inserts a new task into the database
func (s *Storage) CreateTask(name, command string, pid int, logPath string) (int64, error) {
	now := time.Now().Unix()
	result, err := s.db.Exec(
		`INSERT INTO tasks (name, command, pid, status, start_time, log_path, created_at)
		 VALUES (?, ?, ?, 'running', ?, ?, ?)`,
		name, command, pid, now, logPath, now,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to create task: %w", err)
	}

	return result.LastInsertId()
}

// GetTask retrieves a task by ID
func (s *Storage) GetTask(id int) (*Task, error) {
	var t Task
	var startTime, createdAt int64
	var endTime sql.NullInt64

	err := s.db.QueryRow(
		`SELECT id, name, command, pid, status, start_time, end_time, log_path, created_at
		 FROM tasks WHERE id = ?`, id,
	).Scan(&t.ID, &t.Name, &t.Command, &t.PID, &t.Status, &startTime, &endTime, &t.LogPath, &createdAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("task %d not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	t.StartTime = time.Unix(startTime, 0)
	t.CreatedAt = time.Unix(createdAt, 0)
	if endTime.Valid {
		et := time.Unix(endTime.Int64, 0)
		t.EndTime = &et
	}

	return &t, nil
}

// ListTasks retrieves all tasks
func (s *Storage) ListTasks() ([]*Task, error) {
	rows, err := s.db.Query(
		`SELECT id, name, command, pid, status, start_time, end_time, log_path, created_at
		 FROM tasks ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*Task
	for rows.Next() {
		var t Task
		var startTime, createdAt int64
		var endTime sql.NullInt64

		err := rows.Scan(&t.ID, &t.Name, &t.Command, &t.PID, &t.Status, &startTime, &endTime, &t.LogPath, &createdAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan task: %w", err)
		}

		t.StartTime = time.Unix(startTime, 0)
		t.CreatedAt = time.Unix(createdAt, 0)
		if endTime.Valid {
			et := time.Unix(endTime.Int64, 0)
			t.EndTime = &et
		}

		tasks = append(tasks, &t)
	}

	return tasks, nil
}

// UpdateTaskStatus updates a task's status and optionally end time
func (s *Storage) UpdateTaskStatus(id int, status string) error {
	var err error
	if status == "stopped" || status == "crashed" {
		now := time.Now().Unix()
		_, err = s.db.Exec(
			`UPDATE tasks SET status = ?, end_time = ? WHERE id = ?`,
			status, now, id,
		)
	} else {
		_, err = s.db.Exec(
			`UPDATE tasks SET status = ? WHERE id = ?`,
			status, id,
		)
	}

	if err != nil {
		return fmt.Errorf("failed to update task status: %w", err)
	}

	return nil
}

// UpdateTaskPID updates a task's PID
func (s *Storage) UpdateTaskPID(id, pid int) error {
	_, err := s.db.Exec(`UPDATE tasks SET pid = ? WHERE id = ?`, pid, id)
	if err != nil {
		return fmt.Errorf("failed to update task PID: %w", err)
	}
	return nil
}

// ListTasksOlderThan returns completed/crashed tasks older than N days
func (s *Storage) ListTasksOlderThan(days int) ([]*Task, error) {
	cutoff := time.Now().AddDate(0, 0, -days).Unix()
	rows, err := s.db.Query(
		`SELECT id, name, command, pid, status, start_time, end_time, log_path, created_at
		 FROM tasks WHERE end_time IS NOT NULL AND end_time < ? ORDER BY created_at DESC`, cutoff,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list old tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*Task
	for rows.Next() {
		var t Task
		var startTime, createdAt int64
		var endTime sql.NullInt64

		err := rows.Scan(&t.ID, &t.Name, &t.Command, &t.PID, &t.Status, &startTime, &endTime, &t.LogPath, &createdAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan task: %w", err)
		}

		t.StartTime = time.Unix(startTime, 0)
		t.CreatedAt = time.Unix(createdAt, 0)
		if endTime.Valid {
			et := time.Unix(endTime.Int64, 0)
			t.EndTime = &et
		}

		tasks = append(tasks, &t)
	}

	return tasks, nil
}

// DeleteTask deletes a task by ID
func (s *Storage) DeleteTask(id int) error {
	_, err := s.db.Exec(`DELETE FROM tasks WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete task: %w", err)
	}
	return nil
}

// Close closes the database connection
func (s *Storage) Close() error {
	return s.db.Close()
}
