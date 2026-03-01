package session

import (
	"testing"
)

func TestBuildPackageIndex_ThisModule(t *testing.T) {
	// Run against the gorepl module root (two levels up from internal/session).
	idx, err := BuildPackageIndex("../..")
	if err != nil {
		t.Fatalf("BuildPackageIndex: %v", err)
	}
	if len(idx) == 0 {
		t.Fatal("expected non-empty package index")
	}

	// Should contain the example packages.
	if _, ok := idx["foo"]; !ok {
		t.Error("expected 'foo' in index")
	}
	if _, ok := idx["bar"]; !ok {
		t.Error("expected 'bar' in index")
	}
}

func TestBuildPackageIndex_InvalidDir(t *testing.T) {
	_, err := BuildPackageIndex("/nonexistent/path")
	if err == nil {
		t.Error("expected error for nonexistent directory")
	}
}

func TestIsAmbiguous(t *testing.T) {
	if IsAmbiguous("github.com/foo/bar") {
		t.Error("single path should not be ambiguous")
	}
	ambig := "github.com/a/util" + ambiguousMarker + "github.com/b/util"
	if !IsAmbiguous(ambig) {
		t.Error("joined paths should be ambiguous")
	}
}

func TestAmbiguousPaths(t *testing.T) {
	entry := "github.com/a/util" + ambiguousMarker + "github.com/b/util"
	paths := AmbiguousPaths(entry)
	if len(paths) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(paths))
	}
	if paths[0] != "github.com/a/util" {
		t.Errorf("paths[0] = %q", paths[0])
	}
	if paths[1] != "github.com/b/util" {
		t.Errorf("paths[1] = %q", paths[1])
	}
}
