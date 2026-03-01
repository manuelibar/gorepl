package session

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// DefaultSessionRoot is the directory name under $HOME where named sessions
// are stored.
const DefaultSessionRoot = ".gorepl/sessions"

// SessionInfo is the summary returned by ListSessions.
type SessionInfo struct {
	Name      string
	Dir       string
	CreatedAt time.Time
	UpdatedAt time.Time
	CellCount int
}

// SessionRoot returns the absolute path to ~/.gorepl/sessions/.
func SessionRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	return filepath.Join(home, DefaultSessionRoot), nil
}

// ResolveNamedSession returns the directory for a named session under the
// session root. It creates the directory if it doesn't exist.
func ResolveNamedSession(name string) (string, error) {
	root, err := SessionRoot()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating session dir: %w", err)
	}
	return dir, nil
}

// ListSessions scans the session root and returns info for each session
// that has a valid session.json.
func ListSessions() ([]SessionInfo, error) {
	root, err := SessionRoot()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", root, err)
	}

	var sessions []SessionInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		snap, err := LoadSnapshot(dir)
		if err != nil {
			continue // skip dirs without valid session.json
		}
		sessions = append(sessions, SessionInfo{
			Name:      e.Name(),
			Dir:       dir,
			CreatedAt: snap.Metadata.CreatedAt,
			UpdatedAt: snap.Metadata.UpdatedAt,
			CellCount: len(snap.Cells),
		})
	}
	return sessions, nil
}

// DestroySession removes an entire session directory.
func DestroySession(dir string) error {
	return os.RemoveAll(dir)
}
