package repl

import (
	"bytes"
	"context"
	"testing"

	"github.com/manuelibar/gorepl/internal/session"
)

// stubRunner satisfies the fields needed by REPL without actually running Go code.
// We only test command handling here, not evaluation.

func newTestREPL(pkgIndex map[string]string, moduleDir string) *REPL {
	sess := session.New()
	return &REPL{
		session:   sess,
		stdlibMap: map[string]string{"fmt": "fmt"},
		pkgIndex:  pkgIndex,
		moduleDir: moduleDir,
	}
}

func TestHandleImport_ListEmpty(t *testing.T) {
	r := newTestREPL(nil, "")
	var out, errOut bytes.Buffer
	r.handleImport(nil, &out, &errOut)

	if out.String() != "No imported packages.\n" {
		t.Errorf("out = %q", out.String())
	}
}

func TestHandleImport_NoModule(t *testing.T) {
	r := newTestREPL(nil, "")
	var out, errOut bytes.Buffer
	r.handleImport([]string{"foo"}, &out, &errOut)

	if errOut.String() != "no module loaded (use --entrypoint)\n" {
		t.Errorf("errOut = %q", errOut.String())
	}
}

func TestHandleImport_NotFound(t *testing.T) {
	r := newTestREPL(map[string]string{"bar": "github.com/example/bar"}, "")
	var out, errOut bytes.Buffer
	r.handleImport([]string{"baz"}, &out, &errOut)

	if errOut.String() != "package \"baz\" not found in module\n" {
		t.Errorf("errOut = %q", errOut.String())
	}
}

func TestHandleImport_Ambiguous(t *testing.T) {
	ambig := "github.com/a/util" + session.AmbiguousMarkerForTest() + "github.com/b/util"
	r := newTestREPL(map[string]string{"util": ambig}, "")
	var out, errOut bytes.Buffer
	r.handleImport([]string{"util"}, &out, &errOut)

	if errOut.String() == "" {
		t.Error("expected ambiguity error")
	}
}

func TestHandleImport_FullPath(t *testing.T) {
	r := newTestREPL(nil, "")
	var out, errOut bytes.Buffer
	r.handleImport([]string{"github.com/example/foo"}, &out, &errOut)

	if errOut.String() != "" {
		t.Errorf("unexpected error: %s", errOut.String())
	}
	pkgs := r.session.LoadedPackages()
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package, got %d", len(pkgs))
	}
	if pkgs[0].FullPath != "github.com/example/foo" {
		t.Errorf("FullPath = %q", pkgs[0].FullPath)
	}
	if pkgs[0].Mode != session.ImportModeDot {
		t.Errorf("expected ImportModeDot, got %q", pkgs[0].Mode)
	}
}

func TestHandleImport_ShortName(t *testing.T) {
	r := newTestREPL(map[string]string{"foo": "github.com/example/foo"}, "")
	var out, errOut bytes.Buffer
	r.handleImport([]string{"foo"}, &out, &errOut)

	if errOut.String() != "" {
		t.Errorf("unexpected error: %s", errOut.String())
	}
	pkgs := r.session.LoadedPackages()
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package, got %d", len(pkgs))
	}
	if pkgs[0].FullPath != "github.com/example/foo" {
		t.Errorf("FullPath = %q", pkgs[0].FullPath)
	}
}

func TestHandleImport_ListLoaded(t *testing.T) {
	r := newTestREPL(nil, "")
	_ = r.session.Import("github.com/example/foo", "foo")
	_ = r.session.ImportSymbols("github.com/example/bar", "bar", []session.LoadedSymbol{
		{Name: "New", Kind: session.ValueSymbol},
	})

	var out, errOut bytes.Buffer
	r.handleImport(nil, &out, &errOut)

	output := out.String()
	if !strContains(output, "github.com/example/foo") {
		t.Error("expected foo in listing")
	}
	if !strContains(output, "github.com/example/bar") {
		t.Error("expected bar in listing")
	}
	if !strContains(output, "(all exports)") {
		t.Error("expected (all exports) for dot-import")
	}
	if !strContains(output, "New") {
		t.Error("expected symbol name in listing")
	}
}

func TestHandleImport_RejectsSessionModule(t *testing.T) {
	r := newTestREPL(nil, "")
	var out, errOut bytes.Buffer
	r.handleImport([]string{"gorepl_session"}, &out, &errOut)

	if errOut.String() == "" {
		t.Error("expected error for circular dependency")
	}
	if !strContains(errOut.String(), "circular") {
		t.Errorf("expected circular dependency message, got: %s", errOut.String())
	}
	if len(r.session.LoadedPackages()) != 0 {
		t.Error("session module should not be loadable")
	}
}

func TestHandleCommand_ResetClearsImports(t *testing.T) {
	r := newTestREPL(nil, "")
	_ = r.session.Import("github.com/example/foo", "foo")

	var out, errOut bytes.Buffer
	r.handleCommand(context.Background(), ":reset", &out, &errOut)

	if len(r.session.LoadedPackages()) != 0 {
		t.Error("expected imports cleared after :reset")
	}
}

func TestHandleCommand_HelpIncludesImport(t *testing.T) {
	r := newTestREPL(nil, "")
	var out, errOut bytes.Buffer
	r.handleCommand(context.Background(), ":help", &out, &errOut)

	if !strContains(out.String(), ":import") {
		t.Error("expected :import in help output")
	}
}

func TestHandleCommand_DestroyNoPersistence(t *testing.T) {
	r := newTestREPL(nil, "")
	var out, errOut bytes.Buffer
	quit := r.handleCommand(context.Background(), ":destroy", &out, &errOut)

	if quit {
		t.Error("expected :destroy to NOT quit without persistence")
	}
	if !strContains(errOut.String(), "no persistent session") {
		t.Errorf("errOut = %q, expected no persistent session message", errOut.String())
	}
}

func TestHandleCommand_DestroyWithPersistence(t *testing.T) {
	r := newTestREPL(nil, "")
	destroyed := false
	r.onDestroy = func() error { destroyed = true; return nil }

	var out, errOut bytes.Buffer
	quit := r.handleCommand(context.Background(), ":destroy", &out, &errOut)

	if !quit {
		t.Error("expected :destroy to quit with persistence")
	}
	if !destroyed {
		t.Error("expected onDestroy callback to be called")
	}
	if !strContains(out.String(), "Session destroyed") {
		t.Errorf("out = %q, expected destroyed message", out.String())
	}
}

func TestHandleCommand_QuitCallsOnSave(t *testing.T) {
	r := newTestREPL(nil, "")
	saved := false
	r.onSave = func() error { saved = true; return nil }

	var out, errOut bytes.Buffer
	quit := r.handleCommand(context.Background(), ":quit", &out, &errOut)

	if !quit {
		t.Error("expected :quit to quit")
	}
	if !saved {
		t.Error("expected onSave callback to be called on :quit")
	}
}

func TestHandleCommand_HelpIncludesDestroy(t *testing.T) {
	r := newTestREPL(nil, "")
	var out, errOut bytes.Buffer
	r.handleCommand(context.Background(), ":help", &out, &errOut)

	if !strContains(out.String(), ":destroy") {
		t.Error("expected :destroy in help output")
	}
}

func TestParseImportArgs_DotImport(t *testing.T) {
	pkg, flags, err := parseImportArgs([]string{"foo"})
	if err != nil {
		t.Fatal(err)
	}
	if pkg != "foo" {
		t.Errorf("pkg = %q", pkg)
	}
	if flags.ns || flags.deep || flags.blank || flags.query != "" || len(flags.symbols) > 0 {
		t.Errorf("unexpected flags: %+v", flags)
	}
}

func TestParseImportArgs_Qualified(t *testing.T) {
	pkg, flags, err := parseImportArgs([]string{"foo", "--ns"})
	if err != nil {
		t.Fatal(err)
	}
	if pkg != "foo" || !flags.ns {
		t.Errorf("pkg = %q, flags = %+v", pkg, flags)
	}
}

func TestParseImportArgs_QualifiedWithSymbols(t *testing.T) {
	pkg, flags, err := parseImportArgs([]string{"foo", "--ns", "--symbols=Foo,New"})
	if err != nil {
		t.Fatal(err)
	}
	if pkg != "foo" || !flags.ns {
		t.Errorf("pkg = %q, ns = %v", pkg, flags.ns)
	}
	if len(flags.symbols) != 2 || flags.symbols[0] != "Foo" || flags.symbols[1] != "New" {
		t.Errorf("symbols = %v", flags.symbols)
	}
}

func TestParseImportArgs_Blank(t *testing.T) {
	pkg, flags, err := parseImportArgs([]string{"github.com/lib/pq", "--blank"})
	if err != nil {
		t.Fatal(err)
	}
	if pkg != "github.com/lib/pq" || !flags.blank {
		t.Errorf("pkg = %q, flags = %+v", pkg, flags)
	}
}

func TestParseImportArgs_Deep(t *testing.T) {
	pkg, flags, err := parseImportArgs([]string{"foo", "--deep"})
	if err != nil {
		t.Fatal(err)
	}
	if pkg != "foo" || !flags.deep {
		t.Errorf("pkg = %q, flags = %+v", pkg, flags)
	}
}

func TestParseImportArgs_Query(t *testing.T) {
	pkg, flags, err := parseImportArgs([]string{"foo", "?"})
	if err != nil {
		t.Fatal(err)
	}
	if pkg != "foo" || flags.query != "?" {
		t.Errorf("pkg = %q, query = %q", pkg, flags.query)
	}
}

func TestParseImportArgs_DoubleQuery(t *testing.T) {
	pkg, flags, err := parseImportArgs([]string{"foo", "??"})
	if err != nil {
		t.Fatal(err)
	}
	if pkg != "foo" || flags.query != "??" {
		t.Errorf("pkg = %q, query = %q", pkg, flags.query)
	}
}

func TestParseImportArgs_MutuallyExclusive(t *testing.T) {
	_, _, err := parseImportArgs([]string{"foo", "--ns", "--deep"})
	if err == nil {
		t.Error("expected mutual exclusivity error")
	}
	_, _, err = parseImportArgs([]string{"foo", "--ns", "--blank"})
	if err == nil {
		t.Error("expected mutual exclusivity error")
	}
	_, _, err = parseImportArgs([]string{"foo", "--deep", "--blank"})
	if err == nil {
		t.Error("expected mutual exclusivity error")
	}
}

func TestParseImportArgs_SymbolsWithDeepError(t *testing.T) {
	_, _, err := parseImportArgs([]string{"foo", "--deep", "--symbols=Foo"})
	if err == nil {
		t.Error("expected error for --symbols with --deep")
	}
}

func TestParseImportArgs_UnknownFlag(t *testing.T) {
	_, _, err := parseImportArgs([]string{"foo", "--bogus"})
	if err == nil {
		t.Error("expected error for unknown flag")
	}
}

func TestParseImportArgs_FlagsAnyOrder(t *testing.T) {
	pkg, flags, err := parseImportArgs([]string{"--ns", "foo", "--symbols=Bar"})
	if err != nil {
		t.Fatal(err)
	}
	if pkg != "foo" || !flags.ns || len(flags.symbols) != 1 || flags.symbols[0] != "Bar" {
		t.Errorf("pkg = %q, flags = %+v", pkg, flags)
	}
}

func TestHandleImport_QualifiedMode(t *testing.T) {
	r := newTestREPL(nil, "")
	var out, errOut bytes.Buffer
	r.handleImport([]string{"github.com/example/foo", "--ns"}, &out, &errOut)

	pkgs := r.session.LoadedPackages()
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package, got %d", len(pkgs))
	}
	if pkgs[0].Mode != session.ImportModeQualified {
		t.Errorf("expected ImportModeQualified, got %q", pkgs[0].Mode)
	}
}

func TestHandleImport_BlankMode(t *testing.T) {
	r := newTestREPL(nil, "")
	var out, errOut bytes.Buffer
	r.handleImport([]string{"github.com/lib/pq", "--blank"}, &out, &errOut)

	if errOut.String() != "" {
		t.Errorf("unexpected error: %s", errOut.String())
	}
	pkgs := r.session.LoadedPackages()
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package, got %d", len(pkgs))
	}
	if pkgs[0].Mode != session.ImportModeBlank {
		t.Errorf("expected ImportModeBlank, got %q", pkgs[0].Mode)
	}
}

func TestHandleImport_ListLoadedAllModes(t *testing.T) {
	r := newTestREPL(nil, "")
	_ = r.session.Import("github.com/example/foo", "foo")
	_ = r.session.ImportQualified("github.com/example/bar", "bar", nil)
	_ = r.session.ImportBlank("github.com/lib/pq")
	_ = r.session.ImportSymbols("github.com/example/baz", "baz", []session.LoadedSymbol{
		{Name: "New", Kind: session.ValueSymbol},
	})

	var out, errOut bytes.Buffer
	r.handleImport(nil, &out, &errOut)

	output := out.String()
	if !strContains(output, "(all exports)") {
		t.Error("expected (all exports)")
	}
	if !strContains(output, "(qualified)") {
		t.Error("expected (qualified)")
	}
	if !strContains(output, "(side-effect)") {
		t.Error("expected (side-effect)")
	}
	if !strContains(output, "(selective)") {
		t.Error("expected (selective)")
	}
}

func TestHandleImport_DeepRequiresModule(t *testing.T) {
	r := newTestREPL(nil, "")
	var out, errOut bytes.Buffer
	r.handleImport([]string{"github.com/example/foo", "--deep"}, &out, &errOut)

	if errOut.String() == "" {
		t.Error("expected error when using --deep without module")
	}
	if !strContains(errOut.String(), "--deep requires") {
		t.Errorf("expected --deep requires message, got: %s", errOut.String())
	}
}

func TestHandleCommand_ResetExitsPackageMode(t *testing.T) {
	r := newTestREPL(nil, "")
	// Manually enter package mode.
	r.session.EnterPackage(&session.PackageMode{
		FullPath: "github.com/example/foo",
		Name:     "foo",
		Dir:      "/tmp/foo",
	})

	var out, errOut bytes.Buffer
	r.handleCommand(context.Background(), ":reset", &out, &errOut)

	if r.session.GetPackageMode() != nil {
		t.Error("expected package mode cleared after :reset")
	}
}

func strContains(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}
