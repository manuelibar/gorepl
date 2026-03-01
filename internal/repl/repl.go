package repl

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mibar/gorepl/internal/runner"
	"github.com/mibar/gorepl/internal/session"
)

const Version = "0.1.0"

// REPL is the interactive Read-Eval-Print Loop.
type REPL struct {
	session   *session.Session
	runner    *runner.Runner
	stdlibMap map[string]string
	pkgIndex  map[string]string // short name → full import path (may be nil)
	moduleDir string            // module root for go doc calls (may be empty)
	onSave    func() error      // called on :quit for persistent sessions
	onDestroy func() error      // called on :destroy for persistent sessions
}

// Option configures optional REPL behavior.
type Option func(*REPL)

// WithPersistence enables auto-save on quit and the :destroy command.
func WithPersistence(onSave, onDestroy func() error) Option {
	return func(r *REPL) {
		r.onSave = onSave
		r.onDestroy = onDestroy
	}
}

// New creates a REPL with the given dependencies.
func New(sess *session.Session, r *runner.Runner, stdlibMap map[string]string, pkgIndex map[string]string, moduleDir string, opts ...Option) *REPL {
	rl := &REPL{
		session:   sess,
		runner:    r,
		stdlibMap: stdlibMap,
		pkgIndex:  pkgIndex,
		moduleDir: moduleDir,
	}
	for _, opt := range opts {
		opt(rl)
	}
	return rl
}

// Run starts the interactive loop, reading from in and writing to out/errOut.
// It blocks until the user quits or input is exhausted.
func (r *REPL) Run(ctx context.Context, in io.Reader, out io.Writer, errOut io.Writer) {
	fmt.Fprintf(errOut, "gorepl v%s — type Go code, :help for commands\n", Version)

	scanner := bufio.NewScanner(in)
	for {
		fmt.Fprint(out, r.prompt())
		if !scanner.Scan() {
			break
		}
		line := scanner.Text()

		if cmd := strings.TrimSpace(line); strings.HasPrefix(cmd, ":") {
			if r.handleCommand(ctx, cmd, out, errOut) {
				return
			}
			continue
		}

		code := line
		for bracketDepth(code) > 0 {
			fmt.Fprint(out, "...  ")
			if !scanner.Scan() {
				break
			}
			code += "\n" + scanner.Text()
		}
		if strings.TrimSpace(code) == "" {
			continue
		}

		cell := r.session.AddCell(code)

		var result *runner.Result
		var err error
		var source string

		if pm := r.session.GetPackageMode(); pm != nil {
			// Overlay mode: generate entry file + main wrapper.
			entrySource := session.GenerateEntryFile(pm.Name, r.session.Cells(), r.stdlibMap)
			mainSource := session.GenerateEntryMain(pm.FullPath, pm.Name)
			source = entrySource // for error mapping
			result, err = r.runner.EvalOverlay(ctx, mainSource, entrySource, pm.Dir)
		} else {
			// Normal mode: single main.go.
			source = session.Generate(r.session.Cells(), r.stdlibMap, r.session.LoadedPackages())
			result, err = r.runner.Eval(ctx, source)
		}

		if err != nil {
			r.session.RemoveCell(cell.ID)
			fmt.Fprintf(errOut, "error: %v\n", err)
			continue
		}

		if result.ExitCode != 0 {
			r.session.RemoveCell(cell.ID)
			fmt.Fprint(errOut, mapErrorsToCell(result.Stderr, source))
			continue
		}

		r.session.UpdateCell(cell.ID, session.CellSuccess, result.Stdout, "")

		// Extract only the last cell's output using the delimiter.
		newOut := extractLastCellOutput(result.Stdout)
		if newOut != "" {
			fmt.Fprint(out, newOut)
		}
	}

	// EOF — auto-save if persistent.
	if r.onSave != nil {
		_ = r.onSave()
	}
	fmt.Fprintln(out)
}

// prompt returns the current prompt string, reflecting package mode if active.
func (r *REPL) prompt() string {
	if pm := r.session.GetPackageMode(); pm != nil {
		return fmt.Sprintf("[%s:%d]> ", pm.Name, r.session.NextID())
	}
	return fmt.Sprintf("[%d]> ", r.session.NextID())
}

// extractLastCellOutput splits stdout by the cell delimiter and returns
// only the last segment (i.e., the output from the most recently added cell).
func extractLastCellOutput(stdout string) string {
	parts := strings.Split(stdout, session.CellDelimiter)
	return parts[len(parts)-1]
}
