package session

import (
	"os/exec"
	"strings"
)

// BuildStdlibMap returns a map from package short name to full import path
// for all non-internal stdlib packages. Falls back to a hardcoded subset if
// `go list std` fails.
func BuildStdlibMap() map[string]string {
	cmd := exec.Command("go", "list", "std")
	out, err := cmd.Output()
	if err != nil {
		return FallbackStdlibMap()
	}
	m := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "/internal") || strings.Contains(line, "vendor/") {
			continue
		}
		parts := strings.Split(line, "/")
		pkgName := parts[len(parts)-1]
		// Prefer shorter import paths when package names collide.
		if existing, ok := m[pkgName]; !ok || len(line) < len(existing) {
			m[pkgName] = line
		}
	}
	return m
}

// FallbackStdlibMap returns a minimal hardcoded map for when `go list std` is unavailable.
func FallbackStdlibMap() map[string]string {
	return map[string]string{
		"fmt": "fmt", "os": "os", "io": "io", "log": "log",
		"strings": "strings", "strconv": "strconv", "bytes": "bytes",
		"errors": "errors", "context": "context", "time": "time",
		"math": "math", "sort": "sort", "sync": "sync",
		"regexp": "regexp", "reflect": "reflect", "runtime": "runtime",
		"http": "net/http", "json": "encoding/json", "xml": "encoding/xml",
		"filepath": "path/filepath", "bufio": "bufio",
	}
}
