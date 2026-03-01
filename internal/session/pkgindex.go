package session

import (
	"fmt"
	"os/exec"
	"path"
	"strings"
)

const ambiguousMarker = "__ambiguous__"

// BuildPackageIndex runs `go list ./...` against moduleDir and returns a map
// from short package name (last path segment) to full import path.
// When two packages share the same short name, both full paths are stored
// joined by ambiguousMarker so the caller can detect and report collisions.
// An error is returned only if `go list` itself fails.
func BuildPackageIndex(moduleDir string) (map[string]string, error) {
	cmd := exec.Command("go", "list", "./...")
	cmd.Dir = moduleDir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list ./... in %s: %w", moduleDir, err)
	}

	idx := make(map[string]string)
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		short := path.Base(line)
		if existing, ok := idx[short]; ok {
			// Mark as ambiguous by joining with marker.
			if !strings.Contains(existing, ambiguousMarker) {
				idx[short] = existing + ambiguousMarker + line
			} else {
				idx[short] = existing + ambiguousMarker + line
			}
		} else {
			idx[short] = line
		}
	}
	return idx, nil
}

// IsAmbiguous reports whether a package index entry is ambiguous (multiple
// packages share the same short name).
func IsAmbiguous(entry string) bool {
	return strings.Contains(entry, ambiguousMarker)
}

// AmbiguousPaths splits an ambiguous package index entry into its full paths.
func AmbiguousPaths(entry string) []string {
	return strings.Split(entry, ambiguousMarker)
}

// AmbiguousMarkerForTest returns the ambiguous marker string for use in tests
// outside this package.
func AmbiguousMarkerForTest() string {
	return ambiguousMarker
}
