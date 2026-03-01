package repl

import (
	"context"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"

	"github.com/mibar/gorepl/internal/session"
)

func (r *REPL) handleCommand(ctx context.Context, line string, w io.Writer, errW io.Writer) (quit bool) {
	parts := strings.Fields(line)
	switch parts[0] {
	case ":quit", ":q":
		if r.onSave != nil {
			if err := r.onSave(); err != nil {
				fmt.Fprintf(errW, "error saving session: %v\n", err)
			}
		}
		return true
	case ":destroy":
		if r.onDestroy == nil {
			fmt.Fprintln(errW, "no persistent session to destroy")
			return false
		}
		if err := r.onDestroy(); err != nil {
			fmt.Fprintf(errW, "error destroying session: %v\n", err)
			return false
		}
		fmt.Fprintln(w, "Session destroyed.")
		return true
	case ":reset", ":r":
		r.session.Reset()
		fmt.Fprintln(w, "Session reset.")
	case ":cells", ":c":
		cells := r.session.Cells()
		if len(cells) == 0 {
			fmt.Fprintln(w, "No cells.")
			return false
		}
		for _, c := range cells {
			mark := "ok"
			switch c.Status {
			case "error":
				mark = "err"
			case "pending":
				mark = "..."
			}
			code := c.Code
			if i := strings.Index(code, "\n"); i >= 0 {
				code = code[:i] + " ..."
			}
			fmt.Fprintf(w, "  [%d] (%s) %s\n", c.ID, mark, code)
		}
	case ":remove", ":rm":
		if len(parts) < 2 {
			fmt.Fprintln(errW, "usage: :remove <cell_id>")
			return false
		}
		id, err := strconv.Atoi(parts[1])
		if err != nil {
			fmt.Fprintf(errW, "invalid cell id: %s\n", parts[1])
			return false
		}
		if err := r.session.RemoveCell(id); err != nil {
			fmt.Fprintf(errW, "%v\n", err)
			return false
		}
		fmt.Fprintf(w, "Removed cell %d.\n", id)
	case ":import", ":i":
		r.handleImport(parts[1:], w, errW)
	case ":env", ":e":
		r.handleEnv(parts[1:], w, errW)
	case ":dep", ":d":
		r.handleDep(ctx, parts[1:], w, errW)
	case ":cd":
		if len(parts) < 2 {
			fmt.Fprintln(errW, "usage: :cd <dir>")
			return false
		}
		if err := r.runner.SetWorkDir(parts[1]); err != nil {
			fmt.Fprintf(errW, "error: %v\n", err)
			return false
		}
		fmt.Fprintln(w, r.runner.WorkDir())
	case ":pwd":
		fmt.Fprintln(w, r.runner.WorkDir())
	case ":state":
		r.printState(w)
	case ":help", ":h":
		fmt.Fprint(w, `:quit      :q   exit (auto-saves persistent sessions)
:destroy        remove persistent session and exit
:reset     :r   clear all cells and imports
:cells     :c   list cells
:remove    :rm  remove cell by ID
:state          show current REPL state (cells, imports, mode, env)
:import    :i   import packages (see :import flags below)
:env       :e   show/set env vars (:env KEY=VALUE)
:dep       :d   add dependency (:dep github.com/pkg)
:cd             change working directory (:cd <dir>)
:pwd            print current working directory
:help      :h   this help

:import flags:
  :import                              list loaded packages
  :import <pkg>                        dot-import all exports
  :import <pkg> --symbols=Foo,New      selective dot-import
  :import <pkg> --ns                   qualified import (pkg.Symbol)
  :import <pkg> --ns --symbols=Foo     qualified selective
  :import <pkg> --deep                 package mode (unexported access)
  :import <pkg> --blank                side-effect import (import _ "pkg")
  :import <pkg> ?                      list exported symbols
  :import <pkg> ??                     list all symbols (incl. unexported)
`)
	default:
		fmt.Fprintf(errW, "unknown command: %s\n", parts[0])
	}
	return false
}

// importFlags holds the parsed flags from an :import command.
type importFlags struct {
	ns      bool
	symbols []string
	deep    bool
	blank   bool
	query   string // "?" or "??"
}

// parseImportArgs separates the package name from flags in any order.
func parseImportArgs(args []string) (pkg string, flags importFlags, err error) {
	for _, arg := range args {
		switch {
		case arg == "--ns":
			flags.ns = true
		case strings.HasPrefix(arg, "--symbols="):
			raw := strings.TrimPrefix(arg, "--symbols=")
			for s := range strings.SplitSeq(raw, ",") {
				s = strings.TrimSpace(s)
				if s != "" {
					flags.symbols = append(flags.symbols, s)
				}
			}
		case arg == "--deep":
			flags.deep = true
		case arg == "--blank":
			flags.blank = true
		case arg == "?" || arg == "??":
			flags.query = arg
		case !strings.HasPrefix(arg, "-"):
			if pkg != "" {
				return "", importFlags{}, fmt.Errorf("unexpected argument: %s", arg)
			}
			pkg = arg
		default:
			return "", importFlags{}, fmt.Errorf("unknown flag: %s", arg)
		}
	}

	// Validate mutual exclusivity.
	exclusive := 0
	if flags.ns {
		exclusive++
	}
	if flags.deep {
		exclusive++
	}
	if flags.blank {
		exclusive++
	}
	if exclusive > 1 {
		return "", importFlags{}, fmt.Errorf("--ns, --deep, and --blank are mutually exclusive")
	}
	if len(flags.symbols) > 0 && (flags.deep || flags.blank) {
		return "", importFlags{}, fmt.Errorf("--symbols cannot be combined with --deep or --blank")
	}
	return pkg, flags, nil
}

func (r *REPL) handleImport(args []string, w io.Writer, errW io.Writer) {
	// No args: list loaded packages.
	if len(args) == 0 {
		r.listLoadedPackages(w)
		return
	}

	pkg, flags, err := parseImportArgs(args)
	if err != nil {
		fmt.Fprintf(errW, "error: %v\n", err)
		return
	}

	// Bare :import with no package (shouldn't happen normally, but handle gracefully).
	if pkg == "" {
		r.listLoadedPackages(w)
		return
	}

	// Guard against circular dependency.
	if pkg == "gorepl_session" {
		fmt.Fprintln(errW, "cannot import the session module (circular dependency)")
		return
	}

	// Resolve package name to full path.
	fullPath, shortName, err := r.resolvePackage(pkg)
	if err != nil {
		fmt.Fprintf(errW, "%v\n", err)
		return
	}

	// :import pkg ? — list exported symbols.
	if flags.query == "?" {
		exports, err := session.ListExports(fullPath, r.moduleDir)
		if err != nil {
			fmt.Fprintf(errW, "error: %v\n", err)
			return
		}
		fmt.Fprintf(w, "Exports from %s:\n", fullPath)
		printSymbolHints(w, exports, "")
		return
	}

	// :import pkg ?? — list all symbols including unexported.
	if flags.query == "??" {
		syms, err := session.ListAllSymbols(fullPath, r.moduleDir)
		if err != nil {
			fmt.Fprintf(errW, "error: %v\n", err)
			return
		}
		fmt.Fprintf(w, "All symbols from %s:\n", fullPath)
		printSymbolHints(w, syms, "")
		return
	}

	// :import pkg --blank — side-effect import.
	if flags.blank {
		if err := r.session.ImportBlank(fullPath); err != nil {
			fmt.Fprintf(errW, "%v\n", err)
			return
		}
		fmt.Fprintf(w, "Imported _ %q (side-effect)\n", fullPath)
		return
	}

	// :import pkg --deep — overlay mode (unexported access).
	if flags.deep {
		r.handleDeepImport(fullPath, w, errW)
		return
	}

	// :import pkg --ns — qualified import.
	if flags.ns {
		var classified []session.LoadedSymbol
		if len(flags.symbols) > 0 {
			exports, err := session.ListExports(fullPath, r.moduleDir)
			if err != nil {
				fmt.Fprintf(errW, "error: %v\n", err)
				return
			}
			classified, err = session.ClassifySymbols(flags.symbols, exports)
			if err != nil {
				fmt.Fprintf(errW, "error: %v\n", err)
				return
			}
		}
		if err := r.session.ImportQualified(fullPath, shortName, classified); err != nil {
			fmt.Fprintf(errW, "%v\n", err)
			return
		}
		fmt.Fprintf(w, "Imported %s (qualified: %s.Symbol)\n", fullPath, shortName)
		return
	}

	// :import pkg --symbols=Foo,Bar — selective dot-import.
	if len(flags.symbols) > 0 {
		exports, err := session.ListExports(fullPath, r.moduleDir)
		if err != nil {
			fmt.Fprintf(errW, "error: %v\n", err)
			return
		}
		classified, err := session.ClassifySymbols(flags.symbols, exports)
		if err != nil {
			fmt.Fprintf(errW, "error: %v\n", err)
			return
		}
		if err := r.session.ImportSymbols(fullPath, shortName, classified); err != nil {
			fmt.Fprintf(errW, "%v\n", err)
			return
		}
		fmt.Fprintf(w, "Imported from %s (selective)\n", fullPath)
		return
	}

	// :import pkg — dot-import all exports.
	if err := r.session.Import(fullPath, shortName); err != nil {
		fmt.Fprintf(errW, "%v\n", err)
		return
	}
	fmt.Fprintf(w, "Imported %s (dot-import, all exports)\n", fullPath)
}

// listLoadedPackages prints the currently loaded packages in the session.
func (r *REPL) listLoadedPackages(w io.Writer) {
	pkgs := r.session.LoadedPackages()
	if len(pkgs) == 0 {
		fmt.Fprintln(w, "No imported packages.")
		return
	}
	for _, pkg := range pkgs {
		switch pkg.Mode {
		case session.ImportModeDot:
			fmt.Fprintf(w, "  . %q  (all exports)\n", pkg.FullPath)
		case session.ImportModeSelective:
			names := make([]string, len(pkg.Symbols))
			for i, s := range pkg.Symbols {
				names[i] = s.Name
			}
			fmt.Fprintf(w, "    %q → %s  (selective)\n", pkg.FullPath, strings.Join(names, ", "))
		case session.ImportModeQualified:
			fmt.Fprintf(w, "    %s %q  (qualified)\n", pkg.ShortName, pkg.FullPath)
		case session.ImportModeBlank:
			fmt.Fprintf(w, "  _ %q  (side-effect)\n", pkg.FullPath)
		}
	}
}

// printSymbolHints prints a list of symbols with kind and optional namespace prefix.
func printSymbolHints(w io.Writer, symbols []session.LoadedSymbol, nsPrefix string) {
	for _, sym := range symbols {
		kind := "func"
		if sym.Kind == session.TypeSymbol {
			kind = "type"
		}
		exported := len(sym.Name) > 0 && sym.Name[0] >= 'A' && sym.Name[0] <= 'Z'
		suffix := ""
		if !exported {
			suffix = "   [unexported]"
		}
		prefix := ""
		if nsPrefix != "" {
			prefix = nsPrefix + "."
		}
		fmt.Fprintf(w, "  %-5s %s%s%s\n", kind, prefix, sym.Name, suffix)
	}
}

// handleDeepImport enters overlay/package mode for the given package.
// Cells run inside the package, with access to unexported symbols.
func (r *REPL) handleDeepImport(fullPath string, w io.Writer, errW io.Writer) {
	if r.moduleDir == "" {
		fmt.Fprintln(errW, "--deep requires a module (use --entrypoint)")
		return
	}

	// Resolve the package's on-disk directory and declared name.
	pkgName, pkgDir, err := session.ResolvePackageDir(fullPath, r.moduleDir)
	if err != nil {
		fmt.Fprintf(errW, "error resolving package: %v\n", err)
		return
	}

	// Enter package mode — clears existing session state.
	r.session.EnterPackage(&session.PackageMode{
		FullPath: fullPath,
		Name:     pkgName,
		Dir:      pkgDir,
	})

	fmt.Fprintf(w, "Entered %s (package mode — unexported access)\n", fullPath)
	fmt.Fprintln(w, "Session cleared. Use :reset to exit package mode.")
}

// resolvePackage converts a package argument (short name or full path) to
// (fullPath, shortName). Returns an error if resolution fails.
func (r *REPL) resolvePackage(pkgArg string) (fullPath, shortName string, err error) {
	// Full path provided directly.
	if strings.Contains(pkgArg, "/") {
		return pkgArg, path.Base(pkgArg), nil
	}

	// Short name requires a package index.
	if r.pkgIndex == nil {
		return "", "", fmt.Errorf("no module loaded (use --entrypoint)")
	}

	entry, ok := r.pkgIndex[pkgArg]
	if !ok {
		return "", "", fmt.Errorf("package %q not found in module", pkgArg)
	}
	if session.IsAmbiguous(entry) {
		paths := session.AmbiguousPaths(entry)
		return "", "", fmt.Errorf("ambiguous package %q — matches:\n  %s\nuse full path instead",
			pkgArg, strings.Join(paths, "\n  "))
	}

	return entry, pkgArg, nil
}

func (r *REPL) handleEnv(args []string, w io.Writer, errW io.Writer) {
	if len(args) == 0 {
		env := r.runner.Env()
		if len(env) == 0 {
			fmt.Fprintln(w, "No custom env vars.")
			return
		}
		for k, v := range env {
			fmt.Fprintf(w, "  %s=%s\n", k, v)
		}
		return
	}

	arg := args[0]
	if k, v, ok := strings.Cut(arg, "="); ok {
		r.runner.SetEnv(k, v)
		fmt.Fprintf(w, "  %s=%s\n", k, v)
	} else {
		env := r.runner.Env()
		if v, ok := env[arg]; ok {
			fmt.Fprintf(w, "  %s=%s\n", arg, v)
		} else {
			fmt.Fprintf(errW, "env var %s not set\n", arg)
		}
	}
}

// printState writes a structured summary of the current REPL state.
// Useful for agents and tooling that need to introspect the session.
func (r *REPL) printState(w io.Writer) {
	cells := r.session.Cells()
	pkgs := r.session.LoadedPackages()
	pm := r.session.GetPackageMode()

	fmt.Fprintf(w, "cells:       %d  (next id: %d)\n", len(cells), r.session.NextID())

	if pm != nil {
		fmt.Fprintf(w, "mode:        package  name=%s  path=%s\n", pm.Name, pm.FullPath)
	} else {
		fmt.Fprintf(w, "mode:        main\n")
	}

	if len(pkgs) == 0 {
		fmt.Fprintf(w, "imports:     none\n")
	} else {
		fmt.Fprintf(w, "imports:     %d\n", len(pkgs))
		for _, pkg := range pkgs {
			fmt.Fprintf(w, "  [%s] %s\n", pkg.Mode, pkg.FullPath)
		}
	}

	if r.moduleDir != "" {
		fmt.Fprintf(w, "module:      %s\n", r.moduleDir)
	} else {
		fmt.Fprintf(w, "module:      none\n")
	}

	if r.runner != nil {
		fmt.Fprintf(w, "workdir:     %s\n", r.runner.WorkDir())
		if env := r.runner.Env(); len(env) > 0 {
			fmt.Fprintf(w, "env:\n")
			for k, v := range env {
				fmt.Fprintf(w, "  %s=%s\n", k, v)
			}
		}
	}

	fmt.Fprintf(w, "persistent:  %v\n", r.onSave != nil)
}

func (r *REPL) handleDep(ctx context.Context, args []string, w io.Writer, errW io.Writer) {
	if len(args) == 0 {
		fmt.Fprintln(errW, "usage: :dep <package>")
		return
	}
	pkg := args[0]
	fmt.Fprintf(w, "Adding %s...\n", pkg)
	if err := r.runner.AddDep(ctx, pkg); err != nil {
		fmt.Fprintf(errW, "error: %v\n", err)
		return
	}
	fmt.Fprintf(w, "Added %s\n", pkg)
}
