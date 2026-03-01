# Contributing

Thank you for your interest in contributing to **gorepl**.
This guide covers the conventions we follow so every change stays consistent.

---

## Project Structure

```
gorepl/
├── cmd/gorepl/         # CLI entry point (flag parsing, startup)
├── internal/
│   ├── repl/           # REPL loop, command dispatch, input parsing
│   ├── runner/         # Temp dir management, go build/run execution
│   └── session/        # Cell model, codegen, stdlib map, entrypoint traversal
└── docs/               # Concept-oriented documentation
```

- **`cmd/gorepl/`** is the binary entry point. Keep it thin — flag parsing and wiring only.
- **`internal/`** contains all logic. Nothing here is importable by external code.

---

## Go Style

We follow [Effective Go](https://go.dev/doc/effective_go) and the
[Go Code Review Comments](https://go.dev/wiki/CodeReviewComments) wiki.
When in doubt, match the standard library.

### Doc Comments

Every exported symbol **must** have a doc comment.
Every doc comment **must** earn its place — if it restates what the name and
signature already tell you, rewrite it or remove the noise.

| ✅ Good | ❌ Bad |
|---|---|
| `// Generate produces a complete main.go source from the current cell list.` | `// Generate generates code.` |
| `// TraverseEntrypoint discovers local packages reachable from the given file.` | `// TraverseEntrypoint traverses the entrypoint.` |
| `// IsEmpty reports whether the session has no cells.` | `// IsEmpty checks if empty.` |

**Rules of thumb:**

1. **Start with the symbol name** — Go convention: `// Foo does X.`
2. **Say _why_, not _what_** — unless the _what_ is non-obvious.
3. **No section-label comments** above `const` or `var` blocks. The code structure speaks for itself.
4. **No file-name references** in doc comments. Describe the concept, not the file.
5. **Bool-returning functions** — use `reports whether`: `// IsEmpty reports whether the session has no cells.`
6. **Internal code can be richly commented** — explain design decisions, trade-offs, and non-obvious choices.

### Inline Comments

- **Explain _why_, not _what_:** `// Auto-fix: suppress unused var rather than removing it.` is useful; `// add variable` is noise.
- **Algorithm phase labels are OK** inside long functions when they separate distinct logical sections.

### No Function Duplication for Optional Parameters

Use **functional options** instead of multiple function variants:

```go
// RunOption configures optional behaviour for Runner.
type RunOption func(*runConfig)

// WithTimeout sets the execution deadline.
func WithTimeout(d time.Duration) RunOption {
    return func(c *runConfig) { c.timeout = d }
}
```

This gives callers a single entry point and lets us add future options without breaking any existing signature.

---

## Documentation (`.md` files)

Markdown docs live in `docs/` and the project root `README.md`.

### Principles

1. **Concept-oriented, not code-specific.** Describe _roles_ and _responsibilities_, not variable names or function signatures.
2. **Friendly to change.** Avoid hard-coding struct field names or implementation details that may shift.
3. **Narrative tone.** Write for a human scanning for understanding.
4. **Mermaid diagrams** over ASCII art — they render on GitHub and are easier to maintain.
5. **Link, don't duplicate.** Cross-reference between docs; don't repeat the same explanation.

### Structure

| File | Purpose |
|---|---|
| `README.md` | User-facing: rationale, quick start, usage examples |
| `docs/architecture.md` | Internal design: pipeline stages, component map |
| `CONTRIBUTING.md` | This file — style guide and workflow |

---

## Tests & Benchmarks

- **Test files live next to the code they test.** No top-level `test/` directory.
- Prefer **table-driven tests** when testing many inputs against the same logic.
- Use `t.Helper()` in test helpers so failures report the caller's line.
- Benchmarks should call `b.ReportAllocs()` and `b.ResetTimer()` after setup.

```bash
# Run all tests
./run test

# Run benchmarks
./run bench
```

---

## Pull Request Checklist

- [ ] `./run all` passes (build + vet + test).
- [ ] New exported symbols have doc comments that add insight.
- [ ] No section-label comments or file-name references in doc comments.
- [ ] No function duplication for optional parameters — use functional options.
- [ ] Markdown docs describe concepts, not code specifics.

---

## Commit Messages

Use [Conventional Commits](https://www.conventionalcommits.org/) style:

```
feat: add :dep command for runtime dependency injection
fix: handle bracket imbalance in multi-line input
docs: add architecture overview
refactor: extract autofix logic from runner
test: add table-driven tests for codegen imports
```

Keep the subject line under 72 characters. Use the body for _why_, not _what_.
