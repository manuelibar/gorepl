package session

import (
	"strings"
	"testing"
)

func TestGenerate_SingleCell(t *testing.T) {
	cells := []*Cell{{ID: 1, Code: `fmt.Println("hello")`}}
	stdlib := map[string]string{"fmt": "fmt"}
	src := Generate(cells, stdlib, nil)

	if !strings.Contains(src, `"fmt"`) {
		t.Error("expected fmt import")
	}
	if !strings.Contains(src, `fmt.Println("hello")`) {
		t.Error("expected cell code in output")
	}
	if !strings.Contains(src, "// --- Cell 1 ---") {
		t.Error("expected cell comment marker")
	}
	// Single cell should NOT have a delimiter before it.
	if strings.Contains(src, CellDelimiter) {
		t.Error("single cell should not have delimiter")
	}
}

func TestGenerate_MultiCell(t *testing.T) {
	cells := []*Cell{
		{ID: 1, Code: `x := 1`},
		{ID: 2, Code: `fmt.Println(x)`},
	}
	stdlib := map[string]string{"fmt": "fmt"}
	src := Generate(cells, stdlib, nil)

	if !strings.Contains(src, "// --- Cell 1 ---") {
		t.Error("expected cell 1 marker")
	}
	if !strings.Contains(src, "// --- Cell 2 ---") {
		t.Error("expected cell 2 marker")
	}
	// Multi-cell should have delimiter call between cells.
	if !strings.Contains(src, "__GOREPL__") {
		t.Error("expected cell delimiter between cells")
	}
}

func TestGenerate_AutoImport(t *testing.T) {
	cells := []*Cell{{ID: 1, Code: `fmt.Println(time.Now())`}}
	stdlib := map[string]string{"fmt": "fmt", "time": "time"}
	src := Generate(cells, stdlib, nil)

	if !strings.Contains(src, `"fmt"`) {
		t.Error("expected fmt import")
	}
	if !strings.Contains(src, `"time"`) {
		t.Error("expected time import")
	}
}

func TestGenerate_MagicImportRemoved(t *testing.T) {
	// Magic //import comments are no longer supported — they remain as regular code.
	cells := []*Cell{{ID: 1, Code: "//import \"github.com/google/uuid\"\nx := uuid.New()"}}
	stdlib := map[string]string{}
	src := Generate(cells, stdlib, nil)

	// The import block should NOT contain the magic import path.
	// Extract just the import block (between "import (" and ")").
	importStart := strings.Index(src, "import (")
	importEnd := -1
	if importStart >= 0 {
		importEnd = strings.Index(src[importStart:], ")")
		if importEnd >= 0 {
			importBlock := src[importStart : importStart+importEnd+1]
			if strings.Contains(importBlock, "github.com/google/uuid") {
				t.Error("magic import comments should no longer generate imports in the import block")
			}
		}
	}
	// The comment should remain in the code body (treated as plain code).
	if !strings.Contains(src, "//import") {
		t.Error("magic import comment should remain as plain code")
	}
}

func TestGenerate_ImportsOSForMultiCell(t *testing.T) {
	// Single cell should NOT force-import os.
	single := Generate([]*Cell{{ID: 1, Code: `x := 1`}}, map[string]string{}, nil)
	if strings.Contains(single, `"os"`) {
		t.Error("single cell should not force-import os")
	}

	// Multi-cell should import os for the delimiter.
	multi := Generate([]*Cell{
		{ID: 1, Code: `x := 1`},
		{ID: 2, Code: `y := 2`},
	}, map[string]string{}, nil)
	if !strings.Contains(multi, `"os"`) {
		t.Error("multi-cell should import os for delimiter")
	}
}

func TestGenerateGoMod_Standalone(t *testing.T) {
	mod := GenerateGoMod("", "", "1.25")
	if !strings.Contains(mod, "go 1.25") {
		t.Error("expected go version directive")
	}
	if strings.Contains(mod, "require") {
		t.Error("standalone should not have require")
	}
}

func TestGenerateGoMod_WithModule(t *testing.T) {
	mod := GenerateGoMod("github.com/example/pkg", "/path/to/pkg", "1.22")
	if !strings.Contains(mod, "go 1.22") {
		t.Error("expected go version")
	}
	if !strings.Contains(mod, "require github.com/example/pkg v0.0.0") {
		t.Error("expected require directive")
	}
	if !strings.Contains(mod, "replace github.com/example/pkg v0.0.0 => /path/to/pkg") {
		t.Error("expected replace directive")
	}
}

func TestGenerateGoMod_DetectsVersion(t *testing.T) {
	// When goVersion is empty, it should detect from runtime.
	mod := GenerateGoMod("", "", "")
	if !strings.Contains(mod, "go ") {
		t.Error("expected auto-detected go version")
	}
	// Should not contain the fallback "1.21".
	if strings.Contains(mod, "go 1.21") {
		t.Log("warning: got fallback version — runtime detection may have failed")
	}
}

func TestReadGoVersion(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		want     string
	}{
		{"standard", "module foo\n\ngo 1.22\n", "1.22"},
		{"with patch", "module foo\n\ngo 1.25.5\n", "1.25"},
		{"no directive", "module foo\n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ReadGoVersion(tt.contents)
			if got != tt.want {
				t.Errorf("ReadGoVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveImports_Dedup(t *testing.T) {
	cells := []*Cell{
		{ID: 1, Code: `fmt.Println("a")`},
		{ID: 2, Code: `fmt.Println("b")`},
	}
	stdlib := map[string]string{"fmt": "fmt"}
	src := Generate(cells, stdlib, nil)

	count := strings.Count(src, `"fmt"`)
	if count != 1 {
		t.Errorf("expected fmt imported once, got %d times", count)
	}
}

func TestGenerate_DotImport(t *testing.T) {
	cells := []*Cell{{ID: 1, Code: `f := New()`}}
	pkgs := []LoadedPackage{{
		FullPath:  "github.com/example/foo",
		ShortName: "foo",
		Mode:      ImportModeDot,
	}}
	src := Generate(cells, nil, pkgs)

	if !strings.Contains(src, `. "github.com/example/foo"`) {
		t.Error("expected dot-import in output")
	}
}

func TestGenerate_SelectiveImport_TypeAndValue(t *testing.T) {
	cells := []*Cell{{ID: 1, Code: `f := New()`}}
	pkgs := []LoadedPackage{{
		FullPath:  "github.com/example/foo",
		ShortName: "foo",
		Mode:      ImportModeSelective,
		Symbols: []LoadedSymbol{
			{Name: "New", Kind: ValueSymbol},
			{Name: "Foo", Kind: TypeSymbol},
		},
	}}
	src := Generate(cells, nil, pkgs)

	// Should have a regular import, not dot-import.
	if strings.Contains(src, `. "github.com/example/foo"`) {
		t.Error("selective import should not be dot-import")
	}
	if !strings.Contains(src, `"github.com/example/foo"`) {
		t.Error("expected regular import for selective package")
	}

	// Should have aliases outside main().
	if !strings.Contains(src, "var New = foo.New") {
		t.Errorf("expected var alias for New\nsrc:\n%s", src)
	}
	if !strings.Contains(src, "type Foo = foo.Foo") {
		t.Errorf("expected type alias for Foo\nsrc:\n%s", src)
	}
}

func TestGenerate_DotImportDedupsAutoDetect(t *testing.T) {
	// If "foo" is dot-imported and code references foo.Something,
	// the auto-detect should NOT emit a second regular import for "foo".
	cells := []*Cell{{ID: 1, Code: `foo.Bar()`}}
	stdlib := map[string]string{"foo": "foo"}
	pkgs := []LoadedPackage{{
		FullPath:  "foo",
		ShortName: "foo",
		Mode:      ImportModeDot,
	}}
	src := Generate(cells, stdlib, pkgs)

	// "foo" should appear only once (as dot-import), not also as regular import.
	dotCount := strings.Count(src, `. "foo"`)
	regCount := strings.Count(src, `"foo"`)
	// regCount includes the dot-import line, so it should be exactly 1.
	if dotCount != 1 {
		t.Errorf("expected 1 dot-import of foo, got %d", dotCount)
	}
	if regCount != 1 {
		t.Errorf("expected foo imported exactly once (as dot), got %d references", regCount)
	}
}

func TestGenerate_SelectiveDedupsAutoDetect(t *testing.T) {
	// If "bar" is selectively imported and code references bar.Something,
	// auto-detect should NOT emit a duplicate regular import.
	cells := []*Cell{{ID: 1, Code: `bar.Xyz()`}}
	stdlib := map[string]string{"bar": "bar"}
	pkgs := []LoadedPackage{{
		FullPath:  "bar",
		ShortName: "bar",
		Mode:      ImportModeSelective,
		Symbols: []LoadedSymbol{
			{Name: "Xyz", Kind: ValueSymbol},
		},
	}}
	src := Generate(cells, stdlib, pkgs)

	count := strings.Count(src, `"bar"`)
	if count != 1 {
		t.Errorf("expected bar imported once, got %d\nsrc:\n%s", count, src)
	}
}

func TestGenerate_MixedDotAndSelective(t *testing.T) {
	cells := []*Cell{{ID: 1, Code: `f := New(); b := MakeBar()`}}
	pkgs := []LoadedPackage{
		{
			FullPath:  "github.com/example/foo",
			ShortName: "foo",
			Mode:      ImportModeDot,
		},
		{
			FullPath:  "github.com/example/bar",
			ShortName: "bar",
			Mode:      ImportModeSelective,
			Symbols: []LoadedSymbol{
				{Name: "MakeBar", Kind: ValueSymbol},
				{Name: "Bar", Kind: TypeSymbol},
			},
		},
	}
	src := Generate(cells, nil, pkgs)

	if !strings.Contains(src, `. "github.com/example/foo"`) {
		t.Error("expected dot-import for foo")
	}
	if !strings.Contains(src, `"github.com/example/bar"`) {
		t.Error("expected regular import for bar")
	}
	if !strings.Contains(src, "var MakeBar = bar.MakeBar") {
		t.Error("expected var alias for MakeBar")
	}
	if !strings.Contains(src, "type Bar = bar.Bar") {
		t.Error("expected type alias for Bar")
	}
}

func TestGenerate_QualifiedImport(t *testing.T) {
	cells := []*Cell{{ID: 1, Code: `_ = foo.New()`}}
	pkgs := []LoadedPackage{{
		FullPath:  "github.com/example/foo",
		ShortName: "foo",
		Mode:      ImportModeQualified,
	}}
	src := Generate(cells, nil, pkgs)

	// Should emit aliased import: foo "github.com/example/foo"
	if !strings.Contains(src, `foo "github.com/example/foo"`) {
		t.Errorf("expected qualified aliased import\nsrc:\n%s", src)
	}
	// Should NOT have dot-import.
	if strings.Contains(src, `. "github.com/example/foo"`) {
		t.Error("qualified import should not be dot-import")
	}
}

func TestGenerate_BlankImport(t *testing.T) {
	cells := []*Cell{{ID: 1, Code: `x := 1`}}
	pkgs := []LoadedPackage{{
		FullPath: "github.com/lib/pq",
		Mode:     ImportModeBlank,
	}}
	src := Generate(cells, nil, pkgs)

	if !strings.Contains(src, `_ "github.com/lib/pq"`) {
		t.Errorf("expected blank import\nsrc:\n%s", src)
	}
}

func TestGenerate_AllModesCombined(t *testing.T) {
	cells := []*Cell{{ID: 1, Code: `_ = foo.New(); _ = MakeBar()`}}
	pkgs := []LoadedPackage{
		{FullPath: "github.com/example/foo", ShortName: "foo", Mode: ImportModeQualified},
		{FullPath: "github.com/example/bar", ShortName: "bar", Mode: ImportModeDot},
		{FullPath: "github.com/example/baz", ShortName: "baz", Mode: ImportModeSelective,
			Symbols: []LoadedSymbol{{Name: "MakeBar", Kind: ValueSymbol}}},
		{FullPath: "github.com/lib/pq", Mode: ImportModeBlank},
	}
	src := Generate(cells, nil, pkgs)

	if !strings.Contains(src, `. "github.com/example/bar"`) {
		t.Error("expected dot-import for bar")
	}
	if !strings.Contains(src, `"github.com/example/baz"`) {
		t.Error("expected regular import for baz")
	}
	if !strings.Contains(src, `foo "github.com/example/foo"`) {
		t.Error("expected qualified import for foo")
	}
	if !strings.Contains(src, `_ "github.com/lib/pq"`) {
		t.Error("expected blank import for pq")
	}
	if !strings.Contains(src, "var MakeBar = baz.MakeBar") {
		t.Error("expected var alias for MakeBar")
	}
}
