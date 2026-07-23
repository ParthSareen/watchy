package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAppendFailureDoesNotMutateMemory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")
	store, err := NewHistoryStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := store.Append("user", "hello"); err == nil {
		t.Fatal("expected append to fail")
	}
	if recent := store.Recent(1); len(recent) != 0 {
		t.Fatalf("failed append left %d in-memory messages", len(recent))
	}
}
