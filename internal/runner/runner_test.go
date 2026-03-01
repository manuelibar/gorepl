package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAutoFix_UnusedVarOldFormat(t *testing.T) {
	source := "package main\n\nfunc main() {\n\tx := 1\n}\n"
	stderr := "main.go:4:2: x declared and not used"
	fixed := autoFix(source, stderr)
	if !strings.Contains(fixed, "_ = x") {
		t.Errorf("expected blank assignment, got:\n%s", fixed)
	}
}

func TestAutoFix_UnusedVarNewFormat(t *testing.T) {
	source := "package main\n\nfunc main() {\n\tx := 1\n}\n"
	stderr := "main.go:4:2: declared and not used: x"
	fixed := autoFix(source, stderr)
	if !strings.Contains(fixed, "_ = x") {
		t.Errorf("expected blank assignment, got:\n%s", fixed)
	}
}

func TestAutoFix_UnusedImport(t *testing.T) {
	source := "package main\n\nimport (\n\t\"fmt\"\n)\n\nfunc main() {\n}\n"
	stderr := `main.go:4:2: "fmt" imported and not used`
	fixed := autoFix(source, stderr)
	if strings.Contains(fixed, `"fmt"`) {
		t.Errorf("expected import removed, got:\n%s", fixed)
	}
}

func TestAutoFix_Combined(t *testing.T) {
	source := "package main\n\nimport (\n\t\"fmt\"\n)\n\nfunc main() {\n\tx := 1\n}\n"
	stderr := "main.go:4:2: \"fmt\" imported and not used\nmain.go:8:2: x declared and not used"
	fixed := autoFix(source, stderr)
	if strings.Contains(fixed, `"fmt"`) {
		t.Error("expected import removed")
	}
	if !strings.Contains(fixed, "_ = x") {
		t.Error("expected blank assignment")
	}
}

func TestAutoFix_NoFixNeeded(t *testing.T) {
	source := "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(1)\n}\n"
	stderr := "main.go:6:15: undefined: foo"
	fixed := autoFix(source, stderr)
	if fixed != source {
		t.Error("source should be unchanged when no fix applies")
	}
}

func TestSetEnv(t *testing.T) {
	r := &Runner{extraEnv: map[string]string{}}
	r.SetEnv("FOO", "bar")
	env := r.Env()
	if env["FOO"] != "bar" {
		t.Errorf("expected FOO=bar, got %q", env["FOO"])
	}
}

func TestEnv_ReturnsCopy(t *testing.T) {
	r := &Runner{extraEnv: map[string]string{"A": "1"}}
	env := r.Env()
	env["B"] = "2"
	if _, ok := r.extraEnv["B"]; ok {
		t.Error("Env() should return a copy, not a reference")
	}
}

func TestNewWithDir_Fresh(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "fresh-session")
	goMod := "module gorepl_session\n\ngo 1.25\n"

	r, err := NewWithDir(dir, goMod, "")
	if err != nil {
		t.Fatalf("NewWithDir: %v", err)
	}
	if r.TempDir() != dir {
		t.Errorf("TempDir() = %q, want %q", r.TempDir(), dir)
	}

	// go.mod should exist.
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatalf("reading go.mod: %v", err)
	}
	if string(data) != goMod {
		t.Errorf("go.mod = %q, want %q", string(data), goMod)
	}
}

func TestNewWithDir_Resume(t *testing.T) {
	dir := t.TempDir()
	goMod := "module gorepl_session\n\ngo 1.25\n"

	// Write an existing go.mod with different content.
	existing := "module gorepl_session\n\ngo 1.24\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	r, err := NewWithDir(dir, goMod, "")
	if err != nil {
		t.Fatalf("NewWithDir: %v", err)
	}
	_ = r

	// go.mod should NOT be overwritten on resume.
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != existing {
		t.Errorf("go.mod was overwritten on resume: %q", string(data))
	}
}

func TestRestoreEnv(t *testing.T) {
	r := &Runner{extraEnv: map[string]string{"A": "1"}}
	r.RestoreEnv(map[string]string{"B": "2", "C": "3"})
	env := r.Env()
	if env["A"] != "1" || env["B"] != "2" || env["C"] != "3" {
		t.Errorf("env = %v", env)
	}
}
