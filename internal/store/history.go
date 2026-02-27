package store

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ChatMessage represents a single chat message for persistence
type ChatMessage struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// HistoryStore manages chat history with JSONL persistence
type HistoryStore struct {
	path        string
	maxMessages int
	messages    []ChatMessage // in-memory cache of recent messages
}

// NewHistoryStore creates a new history store with the given file path
func NewHistoryStore(path string) (*HistoryStore, error) {
	s := &HistoryStore{
		path:        path,
		maxMessages: 60,
		messages:    make([]ChatMessage, 0, 60),
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create history directory: %w", err)
	}

	// Load existing messages
	if err := s.load(); err != nil {
		return nil, err
	}

	return s, nil
}

// load reads all messages from the JSONL file
func (s *HistoryStore) load() error {
	file, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to open history file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var msg ChatMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue // skip invalid lines
		}
		s.messages = append(s.messages, msg)
	}

	// Trim to max if needed
	if len(s.messages) > s.maxMessages {
		s.messages = s.messages[len(s.messages)-s.maxMessages:]
	}

	return scanner.Err()
}

// Append adds a message to history and persists it
func (s *HistoryStore) Append(role, content string) error {
	msg := ChatMessage{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
	}

	// Add to memory
	s.messages = append(s.messages, msg)
	if len(s.messages) > s.maxMessages {
		s.messages = s.messages[len(s.messages)-s.maxMessages:]
	}

	// Append to file
	file, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open history file: %w", err)
	}
	defer file.Close()

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	if _, err := file.Write(data); err != nil {
		return err
	}
	if _, err := file.WriteString("\n"); err != nil {
		return err
	}

	return nil
}

// Recent returns the last n messages from history
func (s *HistoryStore) Recent(n int) []ChatMessage {
	if n >= len(s.messages) {
		result := make([]ChatMessage, len(s.messages))
		copy(result, s.messages)
		return result
	}
	result := make([]ChatMessage, n)
	copy(result, s.messages[len(s.messages)-n:])
	return result
}

// All returns all messages in memory
func (s *HistoryStore) All() []ChatMessage {
	result := make([]ChatMessage, len(s.messages))
	copy(result, s.messages)
	return result
}

// Close is a no-op for JSONL (data is already persisted)
func (s *HistoryStore) Close() error {
	return nil
}
