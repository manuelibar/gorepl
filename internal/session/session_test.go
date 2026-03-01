package session

import (
	"testing"
)

func TestSession_ImportDotImport(t *testing.T) {
	s := New()
	if err := s.Import("github.com/example/foo", "foo"); err != nil {
		t.Fatalf("Import: %v", err)
	}
	pkgs := s.LoadedPackages()
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package, got %d", len(pkgs))
	}
	if pkgs[0].Mode != ImportModeDot {
		t.Errorf("expected ImportModeDot, got %q", pkgs[0].Mode)
	}
	if pkgs[0].FullPath != "github.com/example/foo" {
		t.Errorf("FullPath = %q", pkgs[0].FullPath)
	}
}

func TestSession_ImportDuplicate(t *testing.T) {
	s := New()
	_ = s.Import("github.com/example/foo", "foo")
	err := s.Import("github.com/example/foo", "foo")
	if err == nil {
		t.Error("expected error for duplicate dot-import")
	}
}

func TestSession_ImportSymbols(t *testing.T) {
	s := New()
	syms := []LoadedSymbol{
		{Name: "New", Kind: ValueSymbol},
		{Name: "Foo", Kind: TypeSymbol},
	}
	if err := s.ImportSymbols("github.com/example/foo", "foo", syms); err != nil {
		t.Fatalf("ImportSymbols: %v", err)
	}
	pkgs := s.LoadedPackages()
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package, got %d", len(pkgs))
	}
	if pkgs[0].Mode != ImportModeSelective {
		t.Errorf("expected ImportModeSelective, got %q", pkgs[0].Mode)
	}
	if len(pkgs[0].Symbols) != 2 {
		t.Errorf("expected 2 symbols, got %d", len(pkgs[0].Symbols))
	}
}

func TestSession_ImportSymbolsCollision(t *testing.T) {
	s := New()
	_ = s.ImportSymbols("github.com/example/foo", "foo", []LoadedSymbol{
		{Name: "New", Kind: ValueSymbol},
	})
	err := s.ImportSymbols("github.com/example/bar", "bar", []LoadedSymbol{
		{Name: "New", Kind: ValueSymbol},
	})
	if err == nil {
		t.Error("expected collision error")
	}
}

func TestSession_ImportSymbolsSamePackageNoCollision(t *testing.T) {
	s := New()
	_ = s.ImportSymbols("github.com/example/foo", "foo", []LoadedSymbol{
		{Name: "New", Kind: ValueSymbol},
	})
	// Updating symbols for the same package should succeed.
	err := s.ImportSymbols("github.com/example/foo", "foo", []LoadedSymbol{
		{Name: "New", Kind: ValueSymbol},
		{Name: "Foo", Kind: TypeSymbol},
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	pkgs := s.LoadedPackages()
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package, got %d", len(pkgs))
	}
	if len(pkgs[0].Symbols) != 2 {
		t.Errorf("expected 2 symbols, got %d", len(pkgs[0].Symbols))
	}
}

func TestSession_UpgradeSelectiveToDotImport(t *testing.T) {
	s := New()
	_ = s.ImportSymbols("github.com/example/foo", "foo", []LoadedSymbol{
		{Name: "New", Kind: ValueSymbol},
	})
	if err := s.Import("github.com/example/foo", "foo"); err != nil {
		t.Fatalf("upgrade to dot-import: %v", err)
	}
	pkgs := s.LoadedPackages()
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package, got %d", len(pkgs))
	}
	if pkgs[0].Mode != ImportModeDot {
		t.Errorf("expected ImportModeDot after upgrade, got %q", pkgs[0].Mode)
	}
	if len(pkgs[0].Symbols) != 0 {
		t.Error("symbols should be cleared after upgrade to dot-import")
	}
}

func TestSession_ResetClearsImports(t *testing.T) {
	s := New()
	_ = s.Import("github.com/example/foo", "foo")
	s.AddCell("x := 1")
	s.Reset()
	if len(s.LoadedPackages()) != 0 {
		t.Error("expected imports cleared after reset")
	}
	if len(s.Cells()) != 0 {
		t.Error("expected cells cleared after reset")
	}
}

func TestSession_LoadedSymbolNames(t *testing.T) {
	s := New()
	_ = s.ImportSymbols("github.com/example/foo", "foo", []LoadedSymbol{
		{Name: "New", Kind: ValueSymbol},
		{Name: "Foo", Kind: TypeSymbol},
	})
	_ = s.Import("github.com/example/bar", "bar")

	names := s.LoadedSymbolNames()
	// Only selective symbols are tracked, not dot-import.
	if len(names) != 2 {
		t.Fatalf("expected 2 symbol names, got %d: %v", len(names), names)
	}
	if names["New"] != "github.com/example/foo" {
		t.Errorf("New owner = %q", names["New"])
	}
	if names["Foo"] != "github.com/example/foo" {
		t.Errorf("Foo owner = %q", names["Foo"])
	}
}

func TestSession_ImportQualified(t *testing.T) {
	s := New()
	syms := []LoadedSymbol{{Name: "New", Kind: ValueSymbol}}
	if err := s.ImportQualified("github.com/example/foo", "foo", syms); err != nil {
		t.Fatalf("ImportQualified: %v", err)
	}
	pkgs := s.LoadedPackages()
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package, got %d", len(pkgs))
	}
	if pkgs[0].Mode != ImportModeQualified {
		t.Errorf("expected ImportModeQualified, got %q", pkgs[0].Mode)
	}
	if len(pkgs[0].Symbols) != 1 {
		t.Errorf("expected 1 symbol, got %d", len(pkgs[0].Symbols))
	}
}

func TestSession_ImportQualifiedNoSymbols(t *testing.T) {
	s := New()
	if err := s.ImportQualified("github.com/example/foo", "foo", nil); err != nil {
		t.Fatalf("ImportQualified: %v", err)
	}
	pkgs := s.LoadedPackages()
	if pkgs[0].Mode != ImportModeQualified {
		t.Errorf("expected ImportModeQualified, got %q", pkgs[0].Mode)
	}
}

func TestSession_ImportBlank(t *testing.T) {
	s := New()
	if err := s.ImportBlank("github.com/lib/pq"); err != nil {
		t.Fatalf("ImportBlank: %v", err)
	}
	pkgs := s.LoadedPackages()
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package, got %d", len(pkgs))
	}
	if pkgs[0].Mode != ImportModeBlank {
		t.Errorf("expected ImportModeBlank, got %q", pkgs[0].Mode)
	}
}

func TestSession_ImportBlankDuplicate(t *testing.T) {
	s := New()
	_ = s.ImportBlank("github.com/lib/pq")
	err := s.ImportBlank("github.com/lib/pq")
	if err == nil {
		t.Error("expected error for duplicate blank import")
	}
}

func TestSession_LoadedSymbolNamesExcludesBlank(t *testing.T) {
	s := New()
	_ = s.ImportBlank("github.com/lib/pq")
	_ = s.ImportQualified("github.com/example/foo", "foo", []LoadedSymbol{
		{Name: "New", Kind: ValueSymbol},
	})
	names := s.LoadedSymbolNames()
	if len(names) != 1 {
		t.Fatalf("expected 1 symbol name, got %d: %v", len(names), names)
	}
	if names["New"] != "github.com/example/foo" {
		t.Errorf("New owner = %q", names["New"])
	}
}

func TestSession_EnterPackage(t *testing.T) {
	s := New()
	// Add some state first.
	s.AddCell("x := 1")
	_ = s.Import("github.com/example/foo", "foo")

	pm := &PackageMode{
		FullPath: "github.com/example/bar",
		Name:     "bar",
		Dir:      "/tmp/bar",
	}
	s.EnterPackage(pm)

	// Should clear cells and imports.
	if len(s.Cells()) != 0 {
		t.Error("expected cells cleared after EnterPackage")
	}
	if len(s.LoadedPackages()) != 0 {
		t.Error("expected imports cleared after EnterPackage")
	}
	if s.NextID() != 1 {
		t.Errorf("expected nextID reset to 1, got %d", s.NextID())
	}

	// Should be in package mode.
	got := s.GetPackageMode()
	if got == nil {
		t.Fatal("expected package mode to be set")
	}
	if got.FullPath != "github.com/example/bar" {
		t.Errorf("FullPath = %q", got.FullPath)
	}
	if got.Name != "bar" {
		t.Errorf("Name = %q", got.Name)
	}
	if got.Dir != "/tmp/bar" {
		t.Errorf("Dir = %q", got.Dir)
	}
}

func TestSession_GetPackageModeNilByDefault(t *testing.T) {
	s := New()
	if s.GetPackageMode() != nil {
		t.Error("expected nil package mode for new session")
	}
}

func TestSession_ResetClearsPackageMode(t *testing.T) {
	s := New()
	s.EnterPackage(&PackageMode{
		FullPath: "github.com/example/foo",
		Name:     "foo",
		Dir:      "/tmp/foo",
	})
	s.AddCell("x := 1")

	s.Reset()

	if s.GetPackageMode() != nil {
		t.Error("expected package mode cleared after Reset")
	}
	if len(s.Cells()) != 0 {
		t.Error("expected cells cleared after Reset")
	}
}

func TestSession_AddCellInPackageMode(t *testing.T) {
	s := New()
	s.EnterPackage(&PackageMode{
		FullPath: "github.com/example/foo",
		Name:     "foo",
		Dir:      "/tmp/foo",
	})

	c := s.AddCell("secretFunc()")
	if c.ID != 1 {
		t.Errorf("expected cell ID 1, got %d", c.ID)
	}
	c2 := s.AddCell("anotherFunc()")
	if c2.ID != 2 {
		t.Errorf("expected cell ID 2, got %d", c2.ID)
	}
	if len(s.Cells()) != 2 {
		t.Errorf("expected 2 cells, got %d", len(s.Cells()))
	}
}
