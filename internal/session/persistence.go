package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SessionFile is the conventional name for the session state file inside a
// session directory.
const SessionFile = "session.json"

// Metadata holds bookkeeping info for a persisted session.
type Metadata struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Module    string    `json:"module,omitempty"`
	ModuleDir string    `json:"module_dir,omitempty"`
}

// SessionSnapshot is the on-disk representation of a session.
type SessionSnapshot struct {
	Metadata   Metadata          `json:"metadata"`
	Cells      []*Cell           `json:"cells"`
	NextID     int               `json:"next_id"`
	LoadedPkgs []LoadedPackage   `json:"loaded_packages,omitempty"`
	EnvVars    map[string]string `json:"env_vars,omitempty"`
}

// Save writes the snapshot as session.json inside dir.
func (snap *SessionSnapshot) Save(dir string) error {
	snap.Metadata.UpdatedAt = time.Now().UTC()

	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling session snapshot: %w", err)
	}

	path := filepath.Join(dir, SessionFile)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// LoadSnapshot reads a session.json from dir.
func LoadSnapshot(dir string) (*SessionSnapshot, error) {
	path := filepath.Join(dir, SessionFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var snap SessionSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &snap, nil
}
