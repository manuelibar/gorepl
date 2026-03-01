package session

import (
	"testing"
)

func TestBuildStdlibMap_NonEmpty(t *testing.T) {
	m := BuildStdlibMap()
	if len(m) == 0 {
		t.Fatal("expected non-empty stdlib map")
	}
}

func TestBuildStdlibMap_ContainsExpected(t *testing.T) {
	m := BuildStdlibMap()
	expected := []struct {
		key  string
		path string
	}{
		{"fmt", "fmt"},
		{"http", "net/http"},
		{"json", "encoding/json"},
		{"filepath", "path/filepath"},
	}
	for _, e := range expected {
		got, ok := m[e.key]
		if !ok {
			t.Errorf("missing key %q", e.key)
			continue
		}
		if got != e.path {
			t.Errorf("m[%q] = %q, want %q", e.key, got, e.path)
		}
	}
}

func TestFallbackStdlibMap(t *testing.T) {
	m := FallbackStdlibMap()
	if m["fmt"] != "fmt" {
		t.Error("fallback should contain fmt")
	}
	if m["http"] != "net/http" {
		t.Error("fallback should contain net/http")
	}
}
