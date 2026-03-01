package session

import (
	"strings"
	"testing"
)

func TestGenerateEntryFile_SingleCell(t *testing.T) {
	cells := []*Cell{{ID: 1, Code: `fmt.Println(secretHelper())`}}
	stdlib := map[string]string{"fmt": "fmt"}
	src := GenerateEntryFile("foo", cells, stdlib)

	if !strings.Contains(src, "package foo") {
		t.Error("expected package foo declaration")
	}
	if !strings.Contains(src, `"fmt"`) {
		t.Error("expected fmt import")
	}
	if !strings.Contains(src, "func GoreplEntry()") {
		t.Error("expected GoreplEntry function")
	}
	if !strings.Contains(src, "fmt.Println(secretHelper())") {
		t.Error("expected cell code in output")
	}
	if !strings.Contains(src, "// --- Cell 1 ---") {
		t.Error("expected cell comment marker")
	}
	// Single cell should NOT have a delimiter.
	if strings.Contains(src, CellDelimiter) {
		t.Error("single cell should not have delimiter")
	}
}

func TestGenerateEntryFile_MultiCell(t *testing.T) {
	cells := []*Cell{
		{ID: 1, Code: `x := 1`},
		{ID: 2, Code: `fmt.Println(x)`},
	}
	stdlib := map[string]string{"fmt": "fmt"}
	src := GenerateEntryFile("foo", cells, stdlib)

	if !strings.Contains(src, "__GOREPL__") {
		t.Error("expected cell delimiter between cells")
	}
	if !strings.Contains(src, `"os"`) {
		t.Error("expected os import for multi-cell delimiter")
	}
}

func TestGenerateEntryMain(t *testing.T) {
	src := GenerateEntryMain("github.com/example/foo", "foo")

	if !strings.Contains(src, "package main") {
		t.Error("expected package main")
	}
	if !strings.Contains(src, `foo "github.com/example/foo"`) {
		t.Error("expected import of target package")
	}
	if !strings.Contains(src, "foo.GoreplEntry()") {
		t.Error("expected call to GoreplEntry")
	}
}

func TestGenerateEntryFile_NoImports(t *testing.T) {
	cells := []*Cell{{ID: 1, Code: `x := 42`}}
	src := GenerateEntryFile("foo", cells, nil)

	if !strings.Contains(src, "package foo") {
		t.Error("expected package foo")
	}
	if strings.Contains(src, "import") {
		t.Error("expected no import block when no stdlib refs")
	}
}
