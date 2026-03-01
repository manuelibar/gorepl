package session

import (
	"testing"
)

func TestParseGoDocOutput_FuncAndType(t *testing.T) {
	output := `package foo // import "github.com/manuelibar/gorepl/example/foo"

func New() *Foo
type Foo struct{}
`
	syms := parseGoDocOutput(output, declSymbolRe)
	if len(syms) != 2 {
		t.Fatalf("expected 2 symbols, got %d: %+v", len(syms), syms)
	}

	// Check New is ValueSymbol.
	found := false
	for _, s := range syms {
		if s.Name == "New" {
			found = true
			if s.Kind != ValueSymbol {
				t.Errorf("New should be ValueSymbol, got %d", s.Kind)
			}
		}
	}
	if !found {
		t.Error("expected to find symbol New")
	}

	// Check Foo is TypeSymbol.
	found = false
	for _, s := range syms {
		if s.Name == "Foo" {
			found = true
			if s.Kind != TypeSymbol {
				t.Errorf("Foo should be TypeSymbol, got %d", s.Kind)
			}
		}
	}
	if !found {
		t.Error("expected to find symbol Foo")
	}
}

func TestParseGoDocOutput_VarAndConst(t *testing.T) {
	output := `package example

var ErrNotFound error
const MaxRetries int
func Helper() string
`
	syms := parseGoDocOutput(output, declSymbolRe)
	expect := map[string]SymbolKind{
		"ErrNotFound": ValueSymbol,
		"MaxRetries":  ValueSymbol,
		"Helper":      ValueSymbol,
	}
	if len(syms) != len(expect) {
		t.Fatalf("expected %d symbols, got %d: %+v", len(expect), len(syms), syms)
	}
	for _, s := range syms {
		want, ok := expect[s.Name]
		if !ok {
			t.Errorf("unexpected symbol %q", s.Name)
			continue
		}
		if s.Kind != want {
			t.Errorf("%s: kind = %d, want %d", s.Name, s.Kind, want)
		}
	}
}

func TestParseGoDocOutput_SkipsMethods(t *testing.T) {
	output := `package foo

type Foo struct{}
func New() *Foo
    func (f *Foo) Do()
    func (f *Foo) String() string
`
	syms := parseGoDocOutput(output, declSymbolRe)
	for _, s := range syms {
		if s.Name == "Do" || s.Name == "String" {
			t.Errorf("method %s should have been skipped", s.Name)
		}
	}
	if len(syms) != 2 {
		t.Errorf("expected 2 symbols (Foo, New), got %d: %+v", len(syms), syms)
	}
}

func TestParseGoDocOutput_Dedup(t *testing.T) {
	output := `package foo

type Foo struct{}
type Foo struct {
    X int
}
`
	syms := parseGoDocOutput(output, declSymbolRe)
	count := 0
	for _, s := range syms {
		if s.Name == "Foo" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected Foo once, got %d times", count)
	}
}

func TestClassifySymbols_Success(t *testing.T) {
	exports := []LoadedSymbol{
		{Name: "New", Kind: ValueSymbol},
		{Name: "Foo", Kind: TypeSymbol},
		{Name: "Bar", Kind: TypeSymbol},
	}
	result, err := ClassifySymbols([]string{"New", "Foo"}, exports)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 symbols, got %d", len(result))
	}
	if result[0].Name != "New" || result[0].Kind != ValueSymbol {
		t.Errorf("result[0] = %+v", result[0])
	}
	if result[1].Name != "Foo" || result[1].Kind != TypeSymbol {
		t.Errorf("result[1] = %+v", result[1])
	}
}

func TestClassifySymbols_Unknown(t *testing.T) {
	exports := []LoadedSymbol{
		{Name: "New", Kind: ValueSymbol},
	}
	_, err := ClassifySymbols([]string{"New", "Bogus", "Xyz"}, exports)
	if err == nil {
		t.Fatal("expected error for unknown symbols")
	}
	if got := err.Error(); got != "symbol(s) not found: Bogus, Xyz" {
		t.Errorf("error = %q", got)
	}
}

func TestParseGoDocOutput_AllSymbolsIncludesUnexported(t *testing.T) {
	output := `package foo

func New() *Foo
type Foo struct{}
func secretHelper()
type internalConfig struct{}
var debugMode bool
`
	syms := parseGoDocOutput(output, declAllSymbolRe)
	names := map[string]bool{}
	for _, s := range syms {
		names[s.Name] = true
	}
	for _, expected := range []string{"New", "Foo", "secretHelper", "internalConfig", "debugMode"} {
		if !names[expected] {
			t.Errorf("expected symbol %q in output, got %v", expected, names)
		}
	}
}

func TestListExports_ThisModule(t *testing.T) {
	syms, err := ListExports("github.com/manuelibar/gorepl/example/foo", "../..")
	if err != nil {
		t.Fatalf("ListExports: %v", err)
	}
	if len(syms) == 0 {
		t.Fatal("expected non-empty export list")
	}

	hasNew := false
	hasFoo := false
	for _, s := range syms {
		if s.Name == "New" && s.Kind == ValueSymbol {
			hasNew = true
		}
		if s.Name == "Foo" && s.Kind == TypeSymbol {
			hasFoo = true
		}
	}
	if !hasNew {
		t.Error("expected New as ValueSymbol")
	}
	if !hasFoo {
		t.Error("expected Foo as TypeSymbol")
	}
}
