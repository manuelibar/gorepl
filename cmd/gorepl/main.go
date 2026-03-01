package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/manuelibar/gorepl/internal/repl"
	"github.com/manuelibar/gorepl/internal/runner"
	"github.com/manuelibar/gorepl/internal/session"
)

func main() {
	entrypoint := flag.String("entrypoint", "", "Go source file or module directory.\n\tFile: auto-discovers the module and pre-imports its local dependencies.\n\tDirectory: loads the module for package access (must contain go.mod).")
	sessionName := flag.String("session", "", "named session (stored in ~/.gorepl/sessions/<name>)")
	sessionDir := flag.String("session-dir", "", "explicit session directory path")
	listSessions := flag.Bool("list-sessions", false, "list saved sessions and exit")
	flag.Parse()

	if *listSessions {
		printSessions()
		return
	}

	if *sessionName != "" && *sessionDir != "" {
		fmt.Fprintln(os.Stderr, "fatal: --session and --session-dir are mutually exclusive")
		os.Exit(1)
	}

	persistent := *sessionName != "" || *sessionDir != ""

	stdlibMap := session.BuildStdlibMap()

	var modulePath, absModuleDir, goVersion string
	var pkgIndex map[string]string
	var entrypointPkgs map[string]string

	if *entrypoint != "" {
		info, err := os.Stat(*entrypoint)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
			os.Exit(1)
		}

		if info.IsDir() {
			// Directory mode: load the module and build package index.
			absDir, err := filepath.Abs(*entrypoint)
			if err != nil {
				fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
				os.Exit(1)
			}
			modData, err := os.ReadFile(filepath.Join(absDir, "go.mod"))
			if err != nil {
				fmt.Fprintf(os.Stderr, "fatal: reading go.mod: %v\n", err)
				os.Exit(1)
			}
			modContents := string(modData)
			modPath := session.ReadModulePath(modContents)
			if modPath == "" {
				fmt.Fprintln(os.Stderr, "fatal: module directive not found in go.mod")
				os.Exit(1)
			}
			absModuleDir = absDir
			modulePath = modPath
			goVersion = session.ReadGoVersion(modContents)
			pkgIndex, err = session.BuildPackageIndex(absDir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: package index unavailable: %v\n", err)
			}
		} else {
			// File mode: traverse imports from the file, auto-discover the module.
			localPkgMap, modPath, modDir, err := session.TraverseEntrypoint(*entrypoint)
			if err != nil {
				fmt.Fprintf(os.Stderr, "fatal: traversing entrypoint: %v\n", err)
				os.Exit(1)
			}
			absModuleDir = modDir
			modulePath = modPath
			if modData, err := os.ReadFile(filepath.Join(modDir, "go.mod")); err == nil {
				goVersion = session.ReadGoVersion(string(modData))
			}
			pkgIndex, err = session.BuildPackageIndex(absModuleDir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: package index unavailable: %v\n", err)
			}
			entrypointPkgs = localPkgMap
		}
	}

	goMod := session.GenerateGoMod(modulePath, absModuleDir, goVersion)

	sess := session.New()
	var r *runner.Runner
	var dir string
	var opts []repl.Option

	if persistent {
		var err error
		dir, err = resolveSessionDir(*sessionName, *sessionDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
			os.Exit(1)
		}

		r, err = runner.NewWithDir(dir, goMod, absModuleDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
			os.Exit(1)
		}

		// Resume if session.json exists; record the original creation time.
		var createdAt time.Time
		if snap, err := session.LoadSnapshot(dir); err == nil {
			sess.Restore(snap.Cells, snap.NextID, snap.LoadedPkgs)
			r.RestoreEnv(snap.EnvVars)
			createdAt = snap.Metadata.CreatedAt
			fmt.Fprintf(os.Stderr, "Resumed session (%d cells)\n", len(snap.Cells))
		} else {
			createdAt = time.Now().UTC()
		}

		saveFn := func() error {
			cells, nextID, pkgs := sess.Snapshot()
			snap := &session.SessionSnapshot{
				Metadata: session.Metadata{
					ID:        sessionID(*sessionName, dir),
					CreatedAt: createdAt,
					Module:    modulePath,
					ModuleDir: absModuleDir,
				},
				Cells:      cells,
				NextID:     nextID,
				LoadedPkgs: pkgs,
				EnvVars:    r.Env(),
			}
			return snap.Save(dir)
		}
		destroyFn := func() error {
			return session.DestroySession(dir)
		}
		opts = append(opts, repl.WithPersistence(saveFn, destroyFn))
	} else {
		// Ephemeral mode — temp dir, auto-cleanup.
		var err error
		r, err = runner.New(goMod, absModuleDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
			os.Exit(1)
		}
		defer r.Cleanup()
	}

	// Pre-load entrypoint packages as qualified imports.
	if len(entrypointPkgs) > 0 {
		loaded := 0
		for shortName, fullPath := range entrypointPkgs {
			if err := sess.ImportQualified(fullPath, shortName, nil); err != nil {
				fmt.Fprintf(os.Stderr, "warning: pre-import %s: %v\n", shortName, err)
				continue
			}
			loaded++
		}
		if loaded > 0 {
			fmt.Fprintf(os.Stderr, "Pre-imported %d packages from %s:\n", loaded, *entrypoint)
			for shortName, fullPath := range entrypointPkgs {
				fmt.Fprintf(os.Stderr, "  %-20s %s\n", shortName, fullPath)
			}
		}
	}

	replInstance := repl.New(sess, r, stdlibMap, pkgIndex, absModuleDir, opts...)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	replInstance.Run(ctx, os.Stdin, os.Stdout, os.Stderr)
}

func resolveSessionDir(name, explicit string) (string, error) {
	if explicit != "" {
		abs, err := filepath.Abs(explicit)
		if err != nil {
			return "", err
		}
		if err := os.MkdirAll(abs, 0o755); err != nil {
			return "", fmt.Errorf("creating session dir: %w", err)
		}
		return abs, nil
	}
	return session.ResolveNamedSession(name)
}

func sessionID(name, dir string) string {
	if name != "" {
		return name
	}
	return filepath.Base(dir)
}

func printSessions() {
	sessions, err := session.ListSessions()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
	if len(sessions) == 0 {
		fmt.Println("No saved sessions.")
		return
	}
	fmt.Printf("%-20s  %-5s  %-20s  %s\n", "NAME", "CELLS", "UPDATED", "DIR")
	for _, s := range sessions {
		fmt.Printf("%-20s  %-5d  %-20s  %s\n",
			s.Name, s.CellCount, s.UpdatedAt.Format("2006-01-02 15:04:05"), s.Dir)
	}
}
