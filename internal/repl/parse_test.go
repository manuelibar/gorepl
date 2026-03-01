package repl

import (
	"testing"
)

func TestBracketDepth(t *testing.T) {
	tests := []struct {
		name string
		code string
		want int
	}{
		{"empty", "", 0},
		{"balanced braces", "func() { x := 1 }", 0},
		{"open brace", "func() {", 1},
		{"nested", "if true { for { ", 2},
		{"parens", "fmt.Println(", 1},
		{"brackets", "x := []int{", 1},
		{"mixed balanced", "x := map[string]int{\"a\": 1}", 0},
		{"string with brace", `x := "{"`, 0},
		{"string with escaped quote", `x := "hello \"world\""`, 0},
		{"raw string with brace", "x := `{`", 0},
		{"line comment with brace", "// {", 0},
		{"block comment with brace", "/* { */", 0},
		{"rune literal", "x := '{'", 0},
		{"escaped backslash in rune", "x := '\\\\'", 0},
		{"escaped quote in string", `x := "\\"`, 0},
		{"close exceeds open clamps to zero", "}", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := bracketDepth(tt.code)
			if got != tt.want {
				t.Errorf("bracketDepth(%q) = %d, want %d", tt.code, got, tt.want)
			}
		})
	}
}

func TestMapErrorsToCell(t *testing.T) {
	source := `package main

import "fmt"

func main() {
	// --- Cell 1 ---
	x := 1

	// --- Cell 2 ---
	fmt.Println(undefined)
}
`
	stderr := `# command-line-arguments
/tmp/gorepl-123/main.go:10:15: undefined: undefined
`
	got := mapErrorsToCell(stderr, source)
	if got == "" {
		t.Fatal("expected non-empty output")
	}
	if !contains(got, "[cell 2]:") {
		t.Errorf("expected cell reference, got: %s", got)
	}
	if contains(got, "# command-line-arguments") {
		t.Error("build noise should be stripped")
	}
}

func TestMapErrorsToCell_NoSource(t *testing.T) {
	got := mapErrorsToCell("", "")
	if got != "" {
		t.Errorf("expected empty, got: %q", got)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
