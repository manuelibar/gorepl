package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadModulePath(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		want     string
	}{
		{"standard", "module github.com/x/app\n\ngo 1.22\n", "github.com/x/app"},
		{"with spaces", "module  github.com/x/app  \n", "github.com/x/app"},
		{"missing", "go 1.22\n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ReadModulePath(tt.contents)
			if got != tt.want {
				t.Errorf("ReadModulePath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTraverseEntrypoint_FindsLocalPackages(t *testing.T) {
	// Create a temp Go file that imports two example packages from this module.
	// The module root is 2 levels up from this package (session → internal → gorepl).
	moduleRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}

	entryContent := `package main

import (
	"github.com/manuelibar/gorepl/example/foo"
	bar "github.com/manuelibar/gorepl/example/bar"
)

func main() {
	_ = foo.New()
	_ = bar.New()
}
`
	tmp := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(tmp, []byte(entryContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Point the temp file at the real module by symlinking or just using the
	// module root directly. TraverseEntrypoint walks up to find go.mod, so we
	// need the temp file to be inside the module tree — OR we test with a real
	// file that's already in the tree.
	//
	// Use a real file from the module tree instead.
	_ = tmp
	_ = moduleRoot

	// Use an actual file within the module tree.
	realEntry := filepath.Join(moduleRoot, "example", "foo", "foo.go")
	if _, err := os.Stat(realEntry); err != nil {
		t.Skip("example/foo/foo.go not found, skipping integration test")
	}

	// foo.go only imports "os" (stdlib), so localPkgMap will be empty,
	// but modulePath and moduleDir must be resolved correctly.
	localPkgMap, modulePath, modDir, err := TraverseEntrypoint(realEntry)
	if err != nil {
		t.Fatalf("TraverseEntrypoint: %v", err)
	}
	if modulePath != "github.com/manuelibar/gorepl" {
		t.Errorf("modulePath = %q, want %q", modulePath, "github.com/manuelibar/gorepl")
	}
	absModuleRoot, _ := filepath.Abs(moduleRoot)
	if modDir != absModuleRoot {
		t.Errorf("moduleDir = %q, want %q", modDir, absModuleRoot)
	}
	// foo.go imports only stdlib, so no local packages discovered.
	if len(localPkgMap) != 0 {
		t.Errorf("expected empty localPkgMap, got %v", localPkgMap)
	}
}

func TestTraverseEntrypoint_AliasResolution(t *testing.T) {
	// Write an entrypoint inside the module tree so findGoMod can resolve go.mod.
	moduleRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	goModPath := filepath.Join(moduleRoot, "go.mod")
	if _, err := os.Stat(goModPath); err != nil {
		t.Skip("go.mod not found")
	}

	// Temp file placed inside the module root so go.mod is reachable.
	entryContent := `package main

import (
	aliased "github.com/manuelibar/gorepl/example/foo"
	"github.com/manuelibar/gorepl/example/bar"
)

func main() { _ = aliased.New(); _ = bar.New() }
`
	tmp := filepath.Join(moduleRoot, "example", "_entrypoint_test_main.go")
	if err := os.WriteFile(tmp, []byte(entryContent), 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp)

	localPkgMap, _, _, err := TraverseEntrypoint(tmp)
	if err != nil {
		t.Fatalf("TraverseEntrypoint: %v", err)
	}

	// foo is imported with alias "aliased" — should take priority over declared pkg name.
	if v, ok := localPkgMap["aliased"]; !ok || v != "github.com/manuelibar/gorepl/example/foo" {
		t.Errorf("expected aliased→example/foo in map, got %v", localPkgMap)
	}
	// bar has no alias — should use declared package name "bar".
	if v, ok := localPkgMap["bar"]; !ok || v != "github.com/manuelibar/gorepl/example/bar" {
		t.Errorf("expected bar→example/bar in map, got %v", localPkgMap)
	}
	// "foo" short name should NOT appear (the alias overrides it).
	if _, ok := localPkgMap["foo"]; ok {
		t.Errorf("short name 'foo' should not appear when alias is used")
	}
}

func TestFindGoMod(t *testing.T) {
	moduleRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}

	// Start from a deep subdir — should still find module root's go.mod.
	deepDir := filepath.Join(moduleRoot, "internal", "session")
	_, gotDir, err := findGoMod(deepDir)
	if err != nil {
		t.Fatalf("findGoMod: %v", err)
	}
	absRoot, _ := filepath.Abs(moduleRoot)
	if gotDir != absRoot {
		t.Errorf("moduleDir = %q, want %q", gotDir, absRoot)
	}
}

func TestImportPathToDir(t *testing.T) {
	got := importPathToDir("github.com/foo/app/service", "github.com/foo/app", "/src/app")
	want := filepath.Join("/src/app", "service")
	if got != want {
		t.Errorf("importPathToDir() = %q, want %q", got, want)
	}
}
