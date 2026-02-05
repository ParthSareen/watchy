package tick

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"
)

// Tick represents a saved command shortcut
type Tick struct {
	Command     string    `json:"command"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// NamedTick pairs a tick name with its data
type NamedTick struct {
	Name string
	Tick Tick
}

// Store manages the collection of ticks
type Store struct {
	path  string
	ticks map[string]Tick
}

var reservedNames = map[string]bool{
	"start": true, "stop": true, "list": true, "logs": true,
	"ask": true, "cleanup": true, "tick": true,
}

// NewStore creates a Store for the given JSON file path, loading existing ticks if the file exists.
func NewStore(path string) (*Store, error) {
	s := &Store{
		path:  path,
		ticks: make(map[string]Tick),
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(data, &s.ticks)
}

func (s *Store) save() error {
	data, err := json.MarshalIndent(s.ticks, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0644)
}

// Save saves a new tick. Returns error if name is reserved or already exists.
func (s *Store) Save(name, command, description string) error {
	if !isValidName(name) {
		return fmt.Errorf("invalid tick name %q (use alphanumeric, dash, or underscore)", name)
	}
	if reservedNames[name] {
		return fmt.Errorf("%q is a reserved command name", name)
	}
	if _, exists := s.ticks[name]; exists {
		return fmt.Errorf("tick %q already exists (use rm first to replace)", name)
	}
	s.ticks[name] = Tick{
		Command:     command,
		Description: description,
		CreatedAt:   time.Now(),
	}
	return s.save()
}

// Get returns a tick by name, or an error if not found.
func (s *Store) Get(name string) (Tick, error) {
	t, ok := s.ticks[name]
	if !ok {
		return Tick{}, fmt.Errorf("tick %q not found", name)
	}
	return t, nil
}

// Remove deletes a tick by name. Returns error if not found.
func (s *Store) Remove(name string) error {
	if _, ok := s.ticks[name]; !ok {
		return fmt.Errorf("tick %q not found", name)
	}
	delete(s.ticks, name)
	return s.save()
}

// List returns all ticks sorted by name.
func (s *Store) List() []NamedTick {
	result := make([]NamedTick, 0, len(s.ticks))
	for name, t := range s.ticks {
		result = append(result, NamedTick{Name: name, Tick: t})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// Has returns true if a tick with the given name exists.
func (s *Store) Has(name string) bool {
	_, ok := s.ticks[name]
	return ok
}

func isValidName(name string) bool {
	if name == "" {
		return false
	}
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_') {
			return false
		}
	}
	return true
}
