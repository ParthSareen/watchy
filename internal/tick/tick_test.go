package tick

import (
	"path/filepath"
	"testing"
)

func TestSaveFailureDoesNotMutateMemory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "ticks.json")
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save("serve", "go run .", ""); err == nil {
		t.Fatal("expected save to fail")
	}
	if store.Has("serve") {
		t.Fatal("failed save mutated in-memory ticks")
	}
}
