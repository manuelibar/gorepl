package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestListSessions_Empty(t *testing.T) {
	// Use a temp dir as the session root to avoid side effects.
	root := t.TempDir()
	sessions, err := listSessionsIn(root)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestListSessions_WithSessions(t *testing.T) {
	root := t.TempDir()

	// Create two sessions.
	for _, name := range []string{"alpha", "beta"} {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		snap := &SessionSnapshot{
			Metadata: Metadata{
				ID:        name,
				CreatedAt: time.Now().UTC(),
			},
			Cells:  []*Cell{{ID: 1, Code: "x := 1", Status: CellSuccess}},
			NextID: 2,
		}
		if err := snap.Save(dir); err != nil {
			t.Fatal(err)
		}
	}

	// Create a dir without session.json — should be skipped.
	if err := os.MkdirAll(filepath.Join(root, "invalid"), 0o755); err != nil {
		t.Fatal(err)
	}

	sessions, err := listSessionsIn(root)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}

	names := map[string]bool{}
	for _, s := range sessions {
		names[s.Name] = true
		if s.CellCount != 1 {
			t.Errorf("session %s: CellCount = %d, want 1", s.Name, s.CellCount)
		}
	}
	if !names["alpha"] || !names["beta"] {
		t.Errorf("expected alpha and beta, got %v", names)
	}
}

func TestDestroySession(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "doomed")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	// Write a file so the dir is non-empty.
	if err := os.WriteFile(filepath.Join(sub, "file.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := DestroySession(sub); err != nil {
		t.Fatalf("DestroySession: %v", err)
	}
	if _, err := os.Stat(sub); !os.IsNotExist(err) {
		t.Error("expected directory to be removed")
	}
}

// listSessionsIn is a test helper that scans a given root instead of the
// default ~/.gorepl/sessions/.
func listSessionsIn(root string) ([]SessionInfo, error) {
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var sessions []SessionInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		snap, err := LoadSnapshot(dir)
		if err != nil {
			continue
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
