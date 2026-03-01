package session

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// SymbolKind classifies an exported symbol for codegen alias emission.
type SymbolKind int

const (
	ValueSymbol SymbolKind = iota // func, var, const → var X = pkg.X
	TypeSymbol                    // type → type X = pkg.X
)

// LoadedSymbol represents an exported symbol with its classification.
type LoadedSymbol struct {
	Name string     `json:"name"`
	Kind SymbolKind `json:"kind"`
}

// declSymbolRe matches `go doc` output lines declaring exported symbols:
//
//	func New() *Foo         (top-level or indented constructor)
//	type Foo struct
//	var ErrSomething = ...
//	const MaxSize = ...
//
// It captures the keyword and the symbol name. Leading whitespace is allowed
// because `go doc` indents constructors under their types.
var declSymbolRe = regexp.MustCompile(`^\s*(func|type|var|const)\s+([A-Z]\w*)`)

// declAllSymbolRe matches both exported and unexported symbols in `go doc -u` output.
var declAllSymbolRe = regexp.MustCompile(`^\s*(func|type|var|const)\s+([a-zA-Z]\w*)`)

// methodRe matches method declarations: `func (receiver) Name(...)`.
// These are skipped — methods are accessible through the type, not standalone.
var methodRe = regexp.MustCompile(`^\s*func\s+\(`)

// ListExports runs `go doc <pkgPath>` in moduleDir and returns all exported
// symbols with their kinds. Only top-level declarations are returned; methods
// on types are excluded (they are accessible through the type itself).
func ListExports(pkgPath, moduleDir string) ([]LoadedSymbol, error) {
	cmd := exec.Command("go", "doc", pkgPath)
	cmd.Dir = moduleDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("go doc %s: %s", pkgPath, strings.TrimSpace(string(out)))
	}

	return parseGoDocOutput(string(out), declSymbolRe), nil
}

// ListAllSymbols runs `go doc -u -all <pkgPath>` in moduleDir and returns all
// symbols including unexported ones. Methods are excluded.
func ListAllSymbols(pkgPath, moduleDir string) ([]LoadedSymbol, error) {
	cmd := exec.Command("go", "doc", "-u", "-all", pkgPath)
	cmd.Dir = moduleDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("go doc -u -all %s: %s", pkgPath, strings.TrimSpace(string(out)))
	}

	return parseGoDocOutput(string(out), declAllSymbolRe), nil
}

// parseGoDocOutput extracts symbols from `go doc` output using the given regex.
func parseGoDocOutput(output string, re *regexp.Regexp) []LoadedSymbol {
	var symbols []LoadedSymbol
	seen := map[string]bool{}
	for _, line := range strings.Split(output, "\n") {
		// Skip method lines (func with receiver).
		if methodRe.MatchString(line) {
			continue
		}
		m := re.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		keyword, name := m[1], m[2]
		if seen[name] {
			continue
		}
		seen[name] = true

		kind := ValueSymbol
		if keyword == "type" {
			kind = TypeSymbol
		}
		symbols = append(symbols, LoadedSymbol{Name: name, Kind: kind})
	}
	return symbols
}

// ResolvePackageDir runs `go list -json <pkgPath>` in moduleDir and returns
// the declared package name and the on-disk directory.
func ResolvePackageDir(pkgPath, moduleDir string) (pkgName, dir string, err error) {
	cmd := exec.Command("go", "list", "-json", pkgPath)
	cmd.Dir = moduleDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("go list -json %s: %s", pkgPath, strings.TrimSpace(string(out)))
	}

	// Quick parse — we only need Name and Dir from the JSON.
	var info struct {
		Name string `json:"Name"`
		Dir  string `json:"Dir"`
	}
	if jsonErr := parseJSON(out, &info); jsonErr != nil {
		return "", "", fmt.Errorf("parsing go list output: %w", jsonErr)
	}
	if info.Dir == "" {
		return "", "", fmt.Errorf("go list did not return Dir for %s", pkgPath)
	}
	return info.Name, info.Dir, nil
}

// parseJSON is a small helper to unmarshal JSON output.
func parseJSON(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

// ClassifySymbols validates requested symbol names against the export list
// and returns classified LoadedSymbol entries. Returns an error listing all
// unknown symbol names.
func ClassifySymbols(requested []string, exports []LoadedSymbol) ([]LoadedSymbol, error) {
	exportMap := make(map[string]LoadedSymbol, len(exports))
	for _, e := range exports {
		exportMap[e.Name] = e
	}

	var result []LoadedSymbol
	var unknown []string
	for _, name := range requested {
		if sym, ok := exportMap[name]; ok {
			result = append(result, sym)
		} else {
			unknown = append(unknown, name)
		}
	}

	if len(unknown) > 0 {
		return nil, fmt.Errorf("symbol(s) not found: %s", strings.Join(unknown, ", "))
	}
	return result, nil
}
