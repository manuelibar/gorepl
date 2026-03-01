package session

import (
	"fmt"
	"regexp"
	"runtime"
	"strings"
)

// CellDelimiter is written to stdout between each cell's output so the REPL
// can extract only the latest cell's output without TrimPrefix heuristics.
const CellDelimiter = "\x00__GOREPL__\x00"

var pkgRefRe = regexp.MustCompile(`\b([a-z][a-z0-9]*)\.[A-Z]`)

// Generate produces a complete main.go source from accumulated cells.
// It auto-detects stdlib imports from package references and emits
// imports and symbol aliases for loaded packages based on their ImportMode.
// A unique delimiter is emitted between cells so the REPL can split output.
func Generate(cells []*Cell, stdlibMap map[string]string, loadedPkgs []LoadedPackage) string {
	var body strings.Builder
	for i, cell := range cells {
		if i > 0 {
			body.WriteString(fmt.Sprintf("\tos.Stdout.WriteString(%q)\n", CellDelimiter))
		}
		body.WriteString(fmt.Sprintf("\t// --- Cell %d ---\n", cell.ID))
		for _, line := range strings.Split(cell.Code, "\n") {
			if line == "" {
				body.WriteString("\n")
			} else {
				body.WriteString(fmt.Sprintf("\t%s\n", line))
			}
		}
		body.WriteString("\n")
	}

	// Build set of package paths claimed by loaded packages so auto-detect
	// and magic imports don't duplicate them.
	loadedPaths := make(map[string]bool)
	for _, pkg := range loadedPkgs {
		loadedPaths[pkg.FullPath] = true
	}

	imports := resolveImports(cells, stdlibMap, loadedPaths)

	// Include "os" for the cell delimiter (only needed with multiple cells).
	if len(cells) > 1 {
		hasOS := false
		for _, imp := range imports {
			if imp == "os" {
				hasOS = true
				break
			}
		}
		if !hasOS {
			imports = append([]string{"os"}, imports...)
		}
	}

	// Classify loaded packages by import mode.
	var dotImports []string
	var selectivePkgs []LoadedPackage
	var qualifiedPkgs []LoadedPackage
	var blankImports []string
	for _, pkg := range loadedPkgs {
		switch pkg.Mode {
		case ImportModeDot:
			dotImports = append(dotImports, pkg.FullPath)
		case ImportModeSelective:
			if len(pkg.Symbols) > 0 {
				selectivePkgs = append(selectivePkgs, pkg)
			}
		case ImportModeQualified:
			qualifiedPkgs = append(qualifiedPkgs, pkg)
		case ImportModeBlank:
			blankImports = append(blankImports, pkg.FullPath)
		}
	}

	// --- Build source ---
	var src strings.Builder
	src.WriteString("package main\n\n")

	// Import block.
	hasImports := len(imports) > 0 || len(dotImports) > 0 || len(selectivePkgs) > 0 ||
		len(qualifiedPkgs) > 0 || len(blankImports) > 0
	if hasImports {
		src.WriteString("import (\n")
		for _, imp := range imports {
			if strings.Contains(imp, " ") {
				src.WriteString(fmt.Sprintf("\t%s\n", imp))
			} else {
				src.WriteString(fmt.Sprintf("\t%q\n", imp))
			}
		}
		for _, dp := range dotImports {
			src.WriteString(fmt.Sprintf("\t. %q\n", dp))
		}
		for _, sp := range selectivePkgs {
			src.WriteString(fmt.Sprintf("\t%q\n", sp.FullPath))
		}
		for _, qp := range qualifiedPkgs {
			src.WriteString(fmt.Sprintf("\t%s %q\n", qp.ShortName, qp.FullPath))
		}
		for _, bp := range blankImports {
			src.WriteString(fmt.Sprintf("\t_ %q\n", bp))
		}
		src.WriteString(")\n\n")
	}

	// Selective symbol aliases (must be outside main for type aliases).
	for _, sp := range selectivePkgs {
		for _, sym := range sp.Symbols {
			switch sym.Kind {
			case TypeSymbol:
				src.WriteString(fmt.Sprintf("type %s = %s.%s\n", sym.Name, sp.ShortName, sym.Name))
			case ValueSymbol:
				src.WriteString(fmt.Sprintf("var %s = %s.%s\n", sym.Name, sp.ShortName, sym.Name))
			}
		}
		src.WriteString("\n")
	}

	src.WriteString("func main() {\n")
	src.WriteString(body.String())
	src.WriteString("}\n")

	return src.String()
}

// GenerateGoMod produces a go.mod for the temp session directory.
// If modulePath is empty, it generates a standalone module.
// goVersion should be a bare version like "1.25" — if empty, it is
// detected from the current runtime.
func GenerateGoMod(modulePath, moduleDir, goVersion string) string {
	if goVersion == "" {
		goVersion = detectGoVersion()
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("module gorepl_session\n\ngo %s\n\n", goVersion))
	if modulePath != "" {
		b.WriteString(fmt.Sprintf("require %s v0.0.0\n\n", modulePath))
		b.WriteString(fmt.Sprintf("replace %s v0.0.0 => %s\n", modulePath, moduleDir))
	}
	return b.String()
}

// ReadGoVersion parses the `go X.Y` directive from a go.mod file's contents.
func ReadGoVersion(goModContents string) string {
	for _, line := range strings.Split(goModContents, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "go ") {
			v := strings.TrimSpace(strings.TrimPrefix(line, "go"))
			// Strip patch version if present: "1.25.5" → "1.25"
			parts := strings.SplitN(v, ".", 3)
			if len(parts) >= 2 {
				return parts[0] + "." + parts[1]
			}
			return v
		}
	}
	return ""
}

// detectGoVersion extracts "X.Y" from runtime.Version() (e.g. "go1.25.5" → "1.25").
func detectGoVersion() string {
	v := runtime.Version()
	v = strings.TrimPrefix(v, "go")
	parts := strings.SplitN(v, ".", 3)
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}
	return "1.21" // safe fallback
}

func resolveImports(cells []*Cell, stdlibMap map[string]string, loadedPaths map[string]bool) []string {
	seen := map[string]bool{}
	var imports []string
	add := func(imp string) {
		key := strings.TrimSpace(imp)
		if seen[key] || loadedPaths[key] {
			return
		}
		seen[key] = true
		imports = append(imports, key)
	}

	// Auto-detect from pkg.Identifier patterns in code.
	var allCode strings.Builder
	for _, cell := range cells {
		allCode.WriteString(cell.Code)
		allCode.WriteString("\n")
	}
	for _, m := range pkgRefRe.FindAllStringSubmatch(allCode.String(), -1) {
		if path, ok := stdlibMap[m[1]]; ok {
			add(path)
		}
	}

	return imports
}
