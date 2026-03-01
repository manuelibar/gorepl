package runner

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Result holds the output of a single execution.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
}

// Runner manages a temp directory for code generation and execution.
type Runner struct {
	tempDir   string
	workDir   string // original working directory; binaries run from here
	moduleDir string // the user's module root (may be empty)
	timeout   time.Duration
	extraEnv  map[string]string
}

// New creates a Runner. It writes the go.mod and optionally copies go.sum
// from the target module directory.
func New(goMod string, moduleDir string) (*Runner, error) {
	dir, err := os.MkdirTemp("", "gorepl-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644); err != nil {
		os.RemoveAll(dir)
		return nil, fmt.Errorf("writing go.mod: %w", err)
	}

	if moduleDir != "" {
		if data, err := os.ReadFile(filepath.Join(moduleDir, "go.sum")); err == nil {
			_ = os.WriteFile(filepath.Join(dir, "go.sum"), data, 0o644)
		}
	}

	workDir, _ := os.Getwd()

	return &Runner{
		tempDir:   dir,
		workDir:   workDir,
		moduleDir: moduleDir,
		timeout:   30 * time.Second,
		extraEnv:  map[string]string{},
	}, nil
}

// NewWithDir creates a Runner that uses the given directory instead of a temp
// directory. If go.mod doesn't already exist in dir, it writes one (fresh
// session). If it does exist, the directory is reused as-is (resume).
func NewWithDir(dir, goMod, moduleDir string) (*Runner, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating session dir: %w", err)
	}

	modPath := filepath.Join(dir, "go.mod")
	if _, err := os.Stat(modPath); os.IsNotExist(err) {
		// Fresh session — write go.mod and copy go.sum.
		if err := os.WriteFile(modPath, []byte(goMod), 0o644); err != nil {
			return nil, fmt.Errorf("writing go.mod: %w", err)
		}
		if moduleDir != "" {
			if data, err := os.ReadFile(filepath.Join(moduleDir, "go.sum")); err == nil {
				_ = os.WriteFile(filepath.Join(dir, "go.sum"), data, 0o644)
			}
		}
	}

	workDir, _ := os.Getwd()

	return &Runner{
		tempDir:   dir,
		workDir:   workDir,
		moduleDir: moduleDir,
		timeout:   30 * time.Second,
		extraEnv:  map[string]string{},
	}, nil
}

// TempDir returns the runner's temporary directory path.
func (r *Runner) TempDir() string {
	return r.tempDir
}

// WorkDir returns the directory binaries execute from.
func (r *Runner) WorkDir() string {
	if r.workDir == "" {
		return r.tempDir
	}
	return r.workDir
}

// SetWorkDir changes the directory binaries execute from.
// dir may be absolute or relative to the current WorkDir.
func (r *Runner) SetWorkDir(dir string) error {
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(r.WorkDir(), dir)
	}
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("%s: %w", dir, err)
	}
	r.workDir = dir
	return nil
}

// SetEnv sets a custom environment variable for subsequent executions.
func (r *Runner) SetEnv(key, value string) {
	r.extraEnv[key] = value
}

// Env returns a copy of the custom environment variables.
func (r *Runner) Env() map[string]string {
	m := make(map[string]string, len(r.extraEnv))
	for k, v := range r.extraEnv {
		m[k] = v
	}
	return m
}

// RestoreEnv bulk-sets environment variables (used when resuming a session).
func (r *Runner) RestoreEnv(env map[string]string) {
	for k, v := range env {
		r.extraEnv[k] = v
	}
}

// AddDep runs `go get <pkg>` in the temp directory to add a dependency.
func (r *Runner) AddDep(ctx context.Context, pkg string) error {
	cmd := exec.CommandContext(ctx, "go", "get", pkg)
	cmd.Dir = r.tempDir
	cmd.Env = r.buildEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("go get %s: %s", pkg, strings.TrimSpace(string(out)))
	}
	return nil
}

// Eval writes source to main.go and runs it. It auto-fixes unused variables
// and unused imports on the first failure before returning an error.
func (r *Runner) Eval(ctx context.Context, source string) (*Result, error) {
	if r.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.timeout)
		defer cancel()
	}

	mainPath := filepath.Join(r.tempDir, "main.go")
	if err := os.WriteFile(mainPath, []byte(source), 0o644); err != nil {
		return nil, fmt.Errorf("writing main.go: %w", err)
	}

	result := r.goRun(ctx)
	if result.ExitCode == 0 {
		return result, nil
	}

	// Try auto-fixing unused vars/imports.
	fixed := autoFix(source, result.Stderr)
	if fixed == source {
		return result, nil
	}

	if err := os.WriteFile(mainPath, []byte(fixed), 0o644); err != nil {
		return result, nil
	}
	fixedResult := r.goRun(ctx)
	if fixedResult.ExitCode == 0 {
		return fixedResult, nil
	}

	// Fix didn't help — restore original source and return original error.
	_ = os.WriteFile(mainPath, []byte(source), 0o644)
	return result, nil
}

// EvalOverlay writes the entrySource as a virtual file overlaid into pkgDir,
// writes mainSource as main.go, and runs `go run -overlay=overlay.json main.go`.
// This allows cells to access unexported symbols from the target package.
func (r *Runner) EvalOverlay(ctx context.Context, mainSource, entrySource, pkgDir string) (*Result, error) {
	if r.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.timeout)
		defer cancel()
	}

	mainPath := filepath.Join(r.tempDir, "main.go")
	entryPath := filepath.Join(r.tempDir, "gorepl_entry.go")
	overlayPath := filepath.Join(r.tempDir, "overlay.json")

	// Write main.go.
	if err := os.WriteFile(mainPath, []byte(mainSource), 0o644); err != nil {
		return nil, fmt.Errorf("writing main.go: %w", err)
	}

	// Write the entry file to the temp dir (actual content).
	if err := os.WriteFile(entryPath, []byte(entrySource), 0o644); err != nil {
		return nil, fmt.Errorf("writing gorepl_entry.go: %w", err)
	}

	// Build overlay JSON: map the temp entry file into the target package directory.
	virtualPath := filepath.Join(pkgDir, "gorepl_entry.go")
	overlayJSON := fmt.Sprintf(`{"Replace":{%q:%q}}`, virtualPath, entryPath)
	if err := os.WriteFile(overlayPath, []byte(overlayJSON), 0o644); err != nil {
		return nil, fmt.Errorf("writing overlay.json: %w", err)
	}

	// Run with overlay.
	result := r.goRunOverlay(ctx, overlayPath)
	if result.ExitCode == 0 {
		return result, nil
	}

	// Try auto-fix (unused vars/imports).
	fixed := autoFix(entrySource, result.Stderr)
	if fixed == entrySource {
		return result, nil
	}

	if err := os.WriteFile(entryPath, []byte(fixed), 0o644); err != nil {
		return result, nil
	}
	fixedResult := r.goRunOverlay(ctx, overlayPath)
	if fixedResult.ExitCode == 0 {
		return fixedResult, nil
	}

	// Restore original and return original error.
	_ = os.WriteFile(entryPath, []byte(entrySource), 0o644)
	return result, nil
}

func (r *Runner) goRunOverlay(ctx context.Context, overlayPath string) *Result {
	binPath := filepath.Join(r.tempDir, "gorepl_main")

	buildCmd := exec.CommandContext(ctx, "go", "build", "-overlay="+overlayPath, "-o", binPath, "main.go")
	buildCmd.Dir = r.tempDir
	buildCmd.Env = r.buildEnv()

	start := time.Now()
	var buildStderr bytes.Buffer
	buildCmd.Stderr = &buildStderr
	if err := buildCmd.Run(); err != nil {
		exitCode := 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		return &Result{Stderr: buildStderr.String(), ExitCode: exitCode, Duration: time.Since(start)}
	}

	runDir := r.workDir
	if runDir == "" {
		runDir = r.tempDir
	}
	runCmd := exec.CommandContext(ctx, binPath)
	runCmd.Dir = runDir
	runCmd.Env = r.buildEnv()

	var stdout, stderr bytes.Buffer
	runCmd.Stdout = &stdout
	runCmd.Stderr = &stderr

	err := runCmd.Run()
	res := &Result{Stdout: stdout.String(), Stderr: stderr.String(), Duration: time.Since(start)}
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			res.ExitCode = exitErr.ExitCode()
		} else {
			res.ExitCode = 1
			res.Stderr = err.Error()
		}
	}
	return res
}

func (r *Runner) buildEnv() []string {
	env := append(os.Environ(), "GOFLAGS=-mod=mod")
	for k, v := range r.extraEnv {
		env = append(env, k+"="+v)
	}
	return env
}

func (r *Runner) goRun(ctx context.Context) *Result {
	binPath := filepath.Join(r.tempDir, "gorepl_main")

	buildCmd := exec.CommandContext(ctx, "go", "build", "-o", binPath, "main.go")
	buildCmd.Dir = r.tempDir
	buildCmd.Env = r.buildEnv()

	start := time.Now()
	var buildStderr bytes.Buffer
	buildCmd.Stderr = &buildStderr
	if err := buildCmd.Run(); err != nil {
		exitCode := 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		return &Result{Stderr: buildStderr.String(), ExitCode: exitCode, Duration: time.Since(start)}
	}

	runDir := r.workDir
	if runDir == "" {
		runDir = r.tempDir
	}
	runCmd := exec.CommandContext(ctx, binPath)
	runCmd.Dir = runDir
	runCmd.Env = r.buildEnv()

	var stdout, stderr bytes.Buffer
	runCmd.Stdout = &stdout
	runCmd.Stderr = &stderr

	err := runCmd.Run()
	res := &Result{Stdout: stdout.String(), Stderr: stderr.String(), Duration: time.Since(start)}
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			res.ExitCode = exitErr.ExitCode()
		} else {
			res.ExitCode = 1
			res.Stderr = err.Error()
		}
	}
	return res
}

// Cleanup removes the temp directory.
func (r *Runner) Cleanup() {
	os.RemoveAll(r.tempDir)
}

var (
	// Go <1.22: "x declared and not used", Go >=1.22: "declared and not used: x"
	unusedVarRe    = regexp.MustCompile(`(?:(\w+) declared and not used|declared and not used: (\w+))`)
	unusedImportRe = regexp.MustCompile(`"([^"]+)" imported and not used`)
)

func autoFix(source, stderr string) string {
	fixed := source

	// Remove unused imports.
	for _, m := range unusedImportRe.FindAllStringSubmatch(stderr, -1) {
		pkg := m[1]
		fixed = strings.Replace(fixed, fmt.Sprintf("\t%q\n", pkg), "", 1)
	}

	// Suppress unused variables by adding _ = varName before closing brace.
	vars := unusedVarRe.FindAllStringSubmatch(stderr, -1)
	if len(vars) > 0 {
		var blanks strings.Builder
		for _, m := range vars {
			name := m[1]
			if name == "" {
				name = m[2] // Go >=1.22 format
			}
			fmt.Fprintf(&blanks, "\t_ = %s\n", name)
		}
		idx := strings.LastIndex(fixed, "}")
		if idx >= 0 {
			fixed = fixed[:idx] + blanks.String() + fixed[idx:]
		}
	}

	return fixed
}
