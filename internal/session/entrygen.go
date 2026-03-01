package session

import (
	"fmt"
	"strings"
)

// GenerateEntryFile produces a source file that lives inside the target package
// (using its declared package name). The cells are placed inside a function
// called GoreplEntry() so the overlay can inject this file into the package
// directory and call it from a thin main wrapper.
//
// Because the file belongs to the target package, all unexported symbols are
// accessible from cells.
func GenerateEntryFile(pkgName string, cells []*Cell, stdlibMap map[string]string) string {
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

	// Auto-detect stdlib imports from code references.
	imports := resolveImports(cells, stdlibMap, nil)

	// Ensure "os" is present when we have multi-cell delimiters.
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

	var src strings.Builder
	src.WriteString(fmt.Sprintf("package %s\n\n", pkgName))

	if len(imports) > 0 {
		src.WriteString("import (\n")
		for _, imp := range imports {
			src.WriteString(fmt.Sprintf("\t%q\n", imp))
		}
		src.WriteString(")\n\n")
	}

	src.WriteString("// GoreplEntry is the entrypoint called by the overlay main wrapper.\n")
	src.WriteString("func GoreplEntry() {\n")
	src.WriteString(body.String())
	src.WriteString("}\n")

	return src.String()
}

// GenerateEntryMain produces a thin main.go that imports the target package
// and calls its GoreplEntry function.
func GenerateEntryMain(fullPkgPath, pkgName string) string {
	var src strings.Builder
	src.WriteString("package main\n\n")
	src.WriteString(fmt.Sprintf("import %s %q\n\n", pkgName, fullPkgPath))
	src.WriteString("func main() {\n")
	src.WriteString(fmt.Sprintf("\t%s.GoreplEntry()\n", pkgName))
	src.WriteString("}\n")
	return src.String()
}
