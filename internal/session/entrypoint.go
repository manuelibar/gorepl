package session

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// ReadModulePath parses the `module` directive from go.mod contents.
func ReadModulePath(goModContents string) string {
	for _, line := range strings.Split(goModContents, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module"))
		}
	}
	return ""
}

// TraverseEntrypoint discovers all reachable local packages starting from the
// given Go source file. It walks the module graph via BFS, parsing imports in
// each package's .go files.
//
// Returns:
//   - localPkgMap: short package name → import path (e.g. "foo" → "github.com/x/app/foo")
//   - modulePath:  the module path from go.mod (e.g. "github.com/x/app")
//   - moduleDir:   the directory containing go.mod
//
// Short-name resolution priority:
//  1. Explicit alias from the entrypoint file (import svc "module/service" → svc)
//  2. Declared package name from the imported package's .go files
//
// Packages in a go.work sibling module are skipped (see TODO below).
// Unreadable dirs/files are skipped silently (best-effort).
func TraverseEntrypoint(file string) (localPkgMap map[string]string, modulePath, moduleDir string, err error) {
	absFile, err := filepath.Abs(file)
	if err != nil {
		return nil, "", "", err
	}

	_, modDir, err := findGoMod(filepath.Dir(absFile))
	if err != nil {
		return nil, "", "", err
	}

	modData, err := os.ReadFile(filepath.Join(modDir, "go.mod"))
	if err != nil {
		return nil, "", "", fmt.Errorf("reading go.mod: %w", err)
	}

	modPath := ReadModulePath(string(modData))
	if modPath == "" {
		return nil, "", "", fmt.Errorf("module directive not found in %s/go.mod", modDir)
	}

	localPkgMap = make(map[string]string)
	visited := make(map[string]bool)

	// Seed BFS from entrypoint file; collect any explicit aliases.
	seedImports, entrypointAliases, err := parseLocalImports(absFile, modPath)
	if err != nil {
		// Best-effort: return empty map rather than failing.
		return localPkgMap, modPath, modDir, nil
	}

	queue := make([]string, 0, len(seedImports))
	queue = append(queue, seedImports...)

	for len(queue) > 0 {
		importPath := queue[0]
		queue = queue[1:]

		if visited[importPath] {
			continue
		}
		visited[importPath] = true

		pkgDir := importPathToDir(importPath, modPath, modDir)
		entries, err := os.ReadDir(pkgDir)
		if err != nil {
			continue // best-effort: skip unreadable dirs
		}

		var pkgName string
		fset := token.NewFileSet()

		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
				continue
			}
			filePath := filepath.Join(pkgDir, entry.Name())
			f, err := parser.ParseFile(fset, filePath, nil, parser.ImportsOnly)
			if err != nil {
				continue // best-effort: skip unreadable files
			}

			// f.Name.Name is always populated even with ImportsOnly.
			if pkgName == "" {
				pkgName = f.Name.Name
			}

			for _, imp := range f.Imports {
				impPath := strings.Trim(imp.Path.Value, `"`)
				if strings.HasPrefix(impPath, modPath+"/") {
					if !visited[impPath] {
						queue = append(queue, impPath)
					}
				}
				// TODO(go.work): imports not prefixed with modPath may belong to a
				// sibling module in a go.work workspace. GenerateGoMod would need
				// multiple replace directives to support that case.
			}
		}

		if pkgName == "" {
			continue
		}

		// Priority: explicit alias from the entrypoint file, then declared package name.
		shortName := pkgName
		if alias, ok := entrypointAliases[importPath]; ok {
			shortName = alias
		}
		localPkgMap[shortName] = importPath
	}

	return localPkgMap, modPath, modDir, nil
}

// findGoMod walks up from startDir until it finds go.mod.
func findGoMod(startDir string) (goModPath, moduleDir string, err error) {
	dir := startDir
	for {
		candidate := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", fmt.Errorf("go.mod not found starting from %s", startDir)
		}
		dir = parent
	}
}

// importPathToDir converts an import path to its on-disk directory.
// e.g. "github.com/foo/app/service" with module "github.com/foo/app" at "/src/app" → "/src/app/service"
func importPathToDir(importPath, modulePath, moduleDir string) string {
	rel := strings.TrimPrefix(importPath, modulePath)
	rel = strings.TrimPrefix(rel, "/")
	return filepath.Join(moduleDir, filepath.FromSlash(rel))
}

// parseLocalImports parses a single .go file and returns local import paths
// (those belonging to modulePath) plus any explicit aliases from the file.
func parseLocalImports(filePath, modulePath string) (imports []string, aliases map[string]string, err error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, nil, parser.ImportsOnly)
	if err != nil {
		return nil, nil, err
	}

	aliases = make(map[string]string)
	for _, imp := range f.Imports {
		impPath := strings.Trim(imp.Path.Value, `"`)
		if strings.HasPrefix(impPath, modulePath+"/") {
			imports = append(imports, impPath)
			if imp.Name != nil && imp.Name.Name != "_" && imp.Name.Name != "." {
				aliases[impPath] = imp.Name.Name
			}
		}
	}
	return imports, aliases, nil
}
