---
name: go-explorer
description: Run Go code interactively via gorepl — execute cells, inspect types at runtime, test function behavior, and access unexported symbols. Use when needing to execute Go code, verify type assumptions, debug functions, explore a Go module's API, or inspect internals.
---

# Go Explorer

Runs Go code interactively against a target module using gorepl.
Each invocation sends code cells via piped stdin, receives output on stdout.

## Prerequisites

```bash
go install github.com/manuelibar/gorepl/cmd/gorepl@latest
```

## Invocation

```bash
printf '<cells>\n:quit\n' | gorepl --entrypoint <module-path> [--session <name>]
```

| Flag | Purpose |
|------|---------|
| `--entrypoint <dir>` | Directory with `go.mod` (`.` for current module) |
| `--entrypoint <file>` | Go file — also pre-imports its reachable local packages |
| `--session <name>` | Persistent session (cells survive across invocations) |

### Single-shot

```bash
printf 'x := 42\nfmt.Println(x)\n:quit\n' | gorepl --entrypoint .
```

### Multi-turn session

```bash
# Turn 1
printf 'x := 42\n:quit\n' | gorepl --entrypoint . --session explore1

# Turn 2 — x is still in scope
printf 'fmt.Println(x * 2)\n:quit\n' | gorepl --entrypoint . --session explore1
```

## Output Parsing

| Pattern | Meaning |
|---------|---------|
| `[N]> <text>` | Cell N accepted; text after `>` is output (empty = success, no output) |
| `[cell N]: <msg>` | Cell N failed; error message follows |
| Other lines | Banners, command responses |

## Import Commands

Send as cells before code. Short names work when `--entrypoint` is set.

| Goal | Command |
|------|---------|
| All exports (dot-import) | `:import pkg` |
| Specific symbols only | `:import pkg --symbols=Foo,New` |
| Qualified access (`pkg.Symbol`) | `:import pkg --ns` |
| Side-effect only | `:import pkg --blank` |
| Unexported access (package mode) | `:import pkg --deep` |
| List exports | `:import pkg ?` |
| List all symbols (incl. unexported) | `:import pkg ??` |

## Other Commands

| Command | Purpose |
|---------|---------|
| `:cells` | List all cells with status |
| `:state` | Full session state (cells, imports, mode, env) |
| `:remove <id>` | Remove a cell by ID |
| `:reset` | Clear cells, imports, exit package mode |
| `:env KEY=VALUE` | Set environment variable for execution |
| `:dep github.com/pkg` | Add third-party dependency mid-session |
| `:cd <dir>` | Change working directory |

## Common Patterns

### Inspect a type

```bash
printf ':import mypackage\nval := New()\nfmt.Printf("%%T %%+v\\n", val, val)\n:quit\n' | gorepl --entrypoint .
```

### Test a function

```bash
printf ':import store --ns\nresult, err := store.Fetch("key-123")\nfmt.Printf("result=%%v err=%%v\\n", result, err)\n:quit\n' | gorepl --entrypoint .
```

### Debug unexported internals

```bash
printf ':import internal/auth --deep\nfmt.Println(tokenTTL)\nfmt.Println(hashSecret("test"))\n:quit\n' | gorepl --entrypoint .
```

### Add external dependency

```bash
printf ':dep github.com/google/uuid\n:import github.com/google/uuid --ns\nfmt.Println(uuid.New())\n:quit\n' | gorepl --entrypoint .
```

### Multi-line code

Bracket-balanced input is handled automatically:

```bash
printf 'for i := 0; i < 3; i++ {\nfmt.Println(i)\n}\n:quit\n' | gorepl --entrypoint .
```

## Auto-fix

gorepl transparently fixes before reporting errors:

- **Unused imports** — removed from generated source
- **Unused variables** — suppressed with `_ = varName`

No need to add `_ = x` for intermediate exploration cells.

## Session Lifecycle

```bash
# List saved sessions
gorepl --list-sessions

# Destroy a session
printf ':destroy\n' | gorepl --session <name>
```

## Tips

- Stdlib packages (`fmt`, `os`, `strings`, etc.) are auto-imported from usage — no `:import` needed.
- Failing cells are discarded automatically; the session rolls back to the last good state.
- `--entrypoint <file>` pre-imports all reachable local packages — best for agents exploring a codebase.
- Use `--session` for multi-turn exploration to avoid re-importing and rebuilding state.
- `:import pkg ??` before `:import pkg --deep` to see what unexported symbols are available.
