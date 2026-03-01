package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndLoadSnapshot(t *testing.T) {
	dir := t.TempDir()

	original := &SessionSnapshot{
		Metadata: Metadata{
			ID:        "test-session",
			CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Module:    "github.com/example/mod",
			ModuleDir: "/tmp/mod",
		},
		Cells: []*Cell{
			{ID: 1, Code: "x := 1", Status: CellSuccess, Output: "1\n"},
			{ID: 2, Code: "y := x + 1", Status: CellSuccess, Output: "2\n"},
		},
		NextID: 3,
		LoadedPkgs: []LoadedPackage{
			{FullPath: "github.com/example/foo", ShortName: "foo", Mode: ImportModeDot},
			{FullPath: "github.com/example/bar", ShortName: "bar", Mode: ImportModeSelective, Symbols: []LoadedSymbol{
				{Name: "New", Kind: ValueSymbol},
				{Name: "Bar", Kind: TypeSymbol},
			}},
		},
		EnvVars: map[string]string{"FOO": "bar", "DEBUG": "1"},
	}

	if err := original.Save(dir); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify file exists.
	if _, err := os.Stat(filepath.Join(dir, SessionFile)); err != nil {
		t.Fatalf("session.json not created: %v", err)
	}

	loaded, err := LoadSnapshot(dir)
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}

	// Verify metadata.
	if loaded.Metadata.ID != original.Metadata.ID {
		t.Errorf("ID = %q, want %q", loaded.Metadata.ID, original.Metadata.ID)
	}
	if !loaded.Metadata.CreatedAt.Equal(original.Metadata.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", loaded.Metadata.CreatedAt, original.Metadata.CreatedAt)
	}
	if loaded.Metadata.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set after Save")
	}
	if loaded.Metadata.Module != original.Metadata.Module {
		t.Errorf("Module = %q, want %q", loaded.Metadata.Module, original.Metadata.Module)
	}

	// Verify cells.
	if len(loaded.Cells) != 2 {
		t.Fatalf("len(Cells) = %d, want 2", len(loaded.Cells))
	}
	if loaded.Cells[0].Code != "x := 1" {
		t.Errorf("Cells[0].Code = %q", loaded.Cells[0].Code)
	}
	if loaded.NextID != 3 {
		t.Errorf("NextID = %d, want 3", loaded.NextID)
	}

	// Verify loaded packages.
	if len(loaded.LoadedPkgs) != 2 {
		t.Fatalf("len(LoadedPkgs) = %d, want 2", len(loaded.LoadedPkgs))
	}
	if loaded.LoadedPkgs[0].Mode != ImportModeDot {
		t.Errorf("LoadedPkgs[0].Mode = %q, want %q", loaded.LoadedPkgs[0].Mode, ImportModeDot)
	}
	if len(loaded.LoadedPkgs[1].Symbols) != 2 {
		t.Errorf("LoadedPkgs[1].Symbols = %d, want 2", len(loaded.LoadedPkgs[1].Symbols))
	}

	// Verify env vars.
	if loaded.EnvVars["FOO"] != "bar" {
		t.Errorf("EnvVars[FOO] = %q, want %q", loaded.EnvVars["FOO"], "bar")
	}
}

func TestLoadSnapshot_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadSnapshot(dir)
	if err == nil {
		t.Fatal("expected error for missing session.json")
	}
}

func TestSnapshotRestoreIdentity(t *testing.T) {
	sess := New()
	sess.AddCell("x := 1")
	sess.AddCell("y := 2")
	sess.UpdateCell(1, CellSuccess, "1\n", "")
	_ = sess.Import("github.com/example/foo", "foo")

	cells, nextID, pkgs := sess.Snapshot()

	// Restore into a fresh session.
	sess2 := New()
	sess2.Restore(cells, nextID, pkgs)

	// Verify identity.
	cells2, nextID2, pkgs2 := sess2.Snapshot()

	if len(cells2) != 2 {
		t.Fatalf("len(cells) = %d, want 2", len(cells2))
	}
	if cells2[0].Code != "x := 1" {
		t.Errorf("cells[0].Code = %q", cells2[0].Code)
	}
	if cells2[0].Status != CellSuccess {
		t.Errorf("cells[0].Status = %q, want %q", cells2[0].Status, CellSuccess)
	}
	if nextID2 != 3 {
		t.Errorf("nextID = %d, want 3", nextID2)
	}
	if len(pkgs2) != 1 {
		t.Fatalf("len(pkgs) = %d, want 1", len(pkgs2))
	}
	if pkgs2[0].FullPath != "github.com/example/foo" {
		t.Errorf("pkgs[0].FullPath = %q", pkgs2[0].FullPath)
	}
}

func TestSnapshotMutationIsolation(t *testing.T) {
	sess := New()
	sess.AddCell("x := 1")

	cells, _, _ := sess.Snapshot()

	// Mutate the snapshot — should not affect the session.
	cells[0].Code = "MUTATED"

	origCells := sess.Cells()
	if origCells[0].Code != "x := 1" {
		t.Errorf("session was mutated via snapshot: Code = %q", origCells[0].Code)
	}
}
