package repl

import (
	"regexp"
	"strconv"
	"strings"
)

// bracketDepth counts unmatched opening brackets in code, respecting strings,
// runes, and comments. Returns 0 when all brackets are balanced.
//
// Uses a stack to track opening brackets so that only correctly paired closers
// reduce the depth. Unmatched or mismatched closing brackets are ignored.
func bracketDepth(code string) int {
	var brackets []rune // stack of unmatched opening brackets
	inString, inRaw, inRune, inLineComment, inBlockComment := false, false, false, false, false
	escaped := false
	prev := rune(0)

	for _, ch := range code {
		switch {
		case inBlockComment:
			if prev == '*' && ch == '/' {
				inBlockComment = false
			}
		case inLineComment:
			if ch == '\n' {
				inLineComment = false
			}
		case inRaw:
			if ch == '`' {
				inRaw = false
			}
		case inString:
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inString = false
			}
		case inRune:
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '\'' {
				inRune = false
			}
		default:
			switch {
			case ch == '/' && prev == '/':
				inLineComment = true
			case ch == '*' && prev == '/':
				inBlockComment = true
			case ch == '`':
				inRaw = true
			case ch == '"':
				inString = true
			case ch == '\'':
				inRune = true
			case ch == '{' || ch == '(' || ch == '[':
				brackets = append(brackets, ch)
			case ch == '}' || ch == ')' || ch == ']':
				if n := len(brackets); n > 0 && bracketPair(brackets[n-1], ch) {
					brackets = brackets[:n-1]
				}
			}
		}
		prev = ch
	}
	return len(brackets)
}

// bracketPair reports whether open and close form a matching bracket pair.
func bracketPair(open, close rune) bool {
	switch open {
	case '{':
		return close == '}'
	case '(':
		return close == ')'
	case '[':
		return close == ']'
	}
	return false
}

var lineRefRe = regexp.MustCompile(`(?m)^(.*?)/(main|gorepl_entry)\.go:(\d+):(.*)$`)

// mapErrorsToCell translates temp-dir error references (main.go:15:2: ...)
// into cell-relative references ([cell 3]: ...) using the generated source's
// "// --- Cell N ---" comment positions.
func mapErrorsToCell(stderr, source string) string {
	cellMap := buildCellLineMap(source)

	var out []string
	for line := range strings.SplitSeq(stderr, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		// Skip build system noise.
		if strings.HasPrefix(strings.TrimSpace(line), "# ") {
			continue
		}

		m := lineRefRe.FindStringSubmatch(line)
		if m != nil {
			// m[1]=path, m[2]=filename(main|gorepl_entry), m[3]=lineNo, m[4]=rest
			fileName := m[2] + ".go"
			lineNo, err := strconv.Atoi(m[3])
			if err == nil {
				if cellID, ok := cellMap[lineNo]; ok {
					line = "[cell " + strconv.Itoa(cellID) + "]:" + m[4]
				} else {
					line = fileName + ":" + m[3] + ":" + m[4]
				}
			} else {
				line = fileName + ":" + m[3] + ":" + m[4]
			}
		}
		line = strings.TrimPrefix(line, "./")
		out = append(out, line)
	}
	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, "\n") + "\n"
}

var cellCommentRe = regexp.MustCompile(`// --- Cell (\d+) ---`)

// buildCellLineMap parses the generated source and returns a map from
// source line number to the cell ID that contains that line.
func buildCellLineMap(source string) map[int]int {
	m := map[int]int{}
	currentCell := 0
	lineNo := 1
	for line := range strings.SplitSeq(source, "\n") {
		if match := cellCommentRe.FindStringSubmatch(line); match != nil {
			id, _ := strconv.Atoi(match[1])
			currentCell = id
		}
		if currentCell > 0 {
			m[lineNo] = currentCell
		}
		lineNo++
	}
	return m
}
