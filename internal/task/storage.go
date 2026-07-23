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
	WorkDir   string
	PID       int
	Status    string // "running", "stopped", "crashed"
	StartTime time.Time
	EndTime   *time.Time
	LogPath   string
	CreatedAt time.Time
}

const taskColumns = "id, name, command, work_dir, pid, status, start_time, end_time, log_path, created_at"

type rowScanner interface {
	Scan(dest ...any) error
}

// NewStorage creates a new Storage instance and initializes the database
func NewStorage(dbPath string) (*Storage, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	// Watchy has concurrent readers and a background completion watcher, but a
	// single local SQLite writer. Serializing access here prevents transient
	// SQLITE_BUSY failures without spreading retries through every call site.
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	s := &Storage{db: db}
	if err := s.initSchema(); err != nil {
		db.Close()
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
			work_dir TEXT NOT NULL DEFAULT '',
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
	if err := s.ensureColumn("work_dir", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}

	return nil
}

func (s *Storage) ensureColumn(name, definition string) error {
	rows, err := s.db.Query(`PRAGMA table_info(tasks)`)
	if err != nil {
		return fmt.Errorf("inspect tasks schema: %w", err)
	}

	for rows.Next() {
		var cid int
		var columnName, columnType string
		var notNull, primaryKey int
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &columnName, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return fmt.Errorf("scan tasks schema: %w", err)
		}
		if columnName == name {
			if err := rows.Close(); err != nil {
				return fmt.Errorf("close tasks schema query: %w", err)
			}
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("read tasks schema: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close tasks schema query: %w", err)
	}
	if _, err := s.db.Exec(`ALTER TABLE tasks ADD COLUMN ` + name + ` ` + definition); err != nil {
		return fmt.Errorf("add tasks.%s: %w", name, err)
	}
	return nil
}

// CreateTask inserts a new task into the database
func (s *Storage) CreateTask(name, command, workDir string, pid int, logPath string) (int64, error) {
	now := time.Now().Unix()
	result, err := s.db.Exec(
		`INSERT INTO tasks (name, command, work_dir, pid, status, start_time, log_path, created_at)
		 VALUES (?, ?, ?, ?, 'running', ?, ?, ?)`,
		name, command, workDir, pid, now, logPath, now,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to create task: %w", err)
	}

	return result.LastInsertId()
}

// GetTask retrieves a task by ID
func (s *Storage) GetTask(id int) (*Task, error) {
	t, err := scanTask(s.db.QueryRow(`SELECT `+taskColumns+` FROM tasks WHERE id = ?`, id))

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("task %d not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	return t, nil
}

// ListTasks retrieves all tasks
func (s *Storage) ListTasks() ([]*Task, error) {
	rows, err := s.db.Query(
		`SELECT ` + taskColumns + ` FROM tasks ORDER BY created_at DESC, id DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan task: %w", err)
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tasks: %w", err)
	}
	return tasks, nil
}

func (s *Storage) GetLatestTask() (*Task, error) {
	t, err := scanTask(s.db.QueryRow(`SELECT ` + taskColumns + ` FROM tasks ORDER BY created_at DESC, id DESC LIMIT 1`))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("no tasks found")
		}
		return nil, fmt.Errorf("failed to get latest task: %w", err)
	}

	return t, nil
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

// ListTasksOlderThan returns completed/crashed tasks older than N days
func (s *Storage) ListTasksOlderThan(days int) ([]*Task, error) {
	cutoff := time.Now().AddDate(0, 0, -days).Unix()
	rows, err := s.db.Query(
		`SELECT `+taskColumns+` FROM tasks
		 WHERE end_time IS NOT NULL AND end_time < ? ORDER BY created_at DESC, id DESC`, cutoff,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list old tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan task: %w", err)
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate old tasks: %w", err)
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

func scanTask(row rowScanner) (*Task, error) {
	var task Task
	var startTime, createdAt int64
	var endTime sql.NullInt64
	if err := row.Scan(
		&task.ID,
		&task.Name,
		&task.Command,
		&task.WorkDir,
		&task.PID,
		&task.Status,
		&startTime,
		&endTime,
		&task.LogPath,
		&createdAt,
	); err != nil {
		return nil, err
	}
	task.StartTime = time.Unix(startTime, 0)
	task.CreatedAt = time.Unix(createdAt, 0)
	if endTime.Valid {
		endTimeValue := time.Unix(endTime.Int64, 0)
		task.EndTime = &endTimeValue
	}
	return &task, nil
}
