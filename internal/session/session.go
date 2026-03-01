package session

import (
	"fmt"
	"sync"
)

// ImportMode describes how a package was loaded into the session.
type ImportMode string

const (
	ImportModeDot       ImportMode = "dot"       // import . "pkg" — all exports promoted
	ImportModeSelective ImportMode = "selective" // import "pkg" + type/var aliases for named symbols
	ImportModeQualified ImportMode = "qualified" // import "pkg" — use pkg.Symbol
	ImportModeBlank     ImportMode = "blank"     // import _ "pkg" — side-effect only
)

// LoadedPackage tracks how a module package was imported into the session.
type LoadedPackage struct {
	FullPath  string         `json:"full_path"`
	ShortName string         `json:"short_name"`
	Mode      ImportMode     `json:"mode"`
	Symbols   []LoadedSymbol `json:"symbols,omitempty"`
}

// PackageMode describes a --deep overlay session targeting a specific package.
type PackageMode struct {
	FullPath string `json:"full_path"` // full import path, e.g. "github.com/x/app/foo"
	Name     string `json:"name"`      // declared package name, e.g. "foo"
	Dir      string `json:"dir"`       // on-disk directory of the package
}

// Session holds the state of a notebook session — an ordered list of cells
// and imported packages.
type Session struct {
	mu          sync.Mutex
	cells       []*Cell
	nextID      int
	loadedPkgs  []LoadedPackage
	packageMode *PackageMode // nil = normal main mode
}

// New creates a new empty session.
func New() *Session {
	return &Session{
		nextID: 1,
	}
}

// AddCell appends a new cell with the given code and returns it.
func (s *Session) AddCell(code string) *Cell {
	s.mu.Lock()
	defer s.mu.Unlock()

	c := &Cell{
		ID:     s.nextID,
		Code:   code,
		Status: CellPending,
	}
	s.nextID++
	s.cells = append(s.cells, c)
	return c
}

// Cells returns a copy of the cell list.
func (s *Session) Cells() []*Cell {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]*Cell, len(s.cells))
	copy(out, s.cells)
	return out
}

// RemoveCell removes a cell by ID. Returns an error if not found.
func (s *Session) RemoveCell(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, c := range s.cells {
		if c.ID == id {
			s.cells = append(s.cells[:i], s.cells[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("cell %d not found", id)
}

// Reset clears all cells, imported packages, and exits package mode.
func (s *Session) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cells = nil
	s.nextID = 1
	s.loadedPkgs = nil
	s.packageMode = nil
}

// EnterPackage switches the session into package mode for the given package.
// It clears existing cells and loaded packages, resetting cell numbering.
func (s *Session) EnterPackage(pm *PackageMode) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.packageMode = pm
	s.cells = nil
	s.nextID = 1
	s.loadedPkgs = nil
}

// GetPackageMode returns the current package mode, or nil if in normal mode.
func (s *Session) GetPackageMode() *PackageMode {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.packageMode
}

// NextID returns the ID that will be assigned to the next cell.
func (s *Session) NextID() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.nextID
}

// UpdateCell updates the status and output of a cell by ID.
func (s *Session) UpdateCell(id int, status CellStatus, output, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, c := range s.cells {
		if c.ID == id {
			c.Status = status
			c.Output = output
			c.Error = errMsg
			return
		}
	}
}

// Import adds a package as a dot-import (all exports promoted).
// If the package is already selectively imported, it upgrades to dot-import.
// Returns an error if already dot-imported.
func (s *Session) Import(fullPath, shortName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, pkg := range s.loadedPkgs {
		if pkg.FullPath == fullPath {
			if pkg.Mode == ImportModeDot {
				return fmt.Errorf("%s already imported", shortName)
			}
			// Upgrade to dot-import.
			s.loadedPkgs[i].Mode = ImportModeDot
			s.loadedPkgs[i].Symbols = nil
			return nil
		}
	}

	s.loadedPkgs = append(s.loadedPkgs, LoadedPackage{
		FullPath:  fullPath,
		ShortName: shortName,
		Mode:      ImportModeDot,
	})
	return nil
}

// ImportSymbols adds a package with only the named symbols promoted (selective dot-import).
// Returns an error if any symbol name collides with an already-loaded symbol.
func (s *Session) ImportSymbols(fullPath, shortName string, symbols []LoadedSymbol) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check for collisions with existing loaded symbols.
	taken := s.loadedSymbolNamesLocked()
	for _, sym := range symbols {
		if owner, ok := taken[sym.Name]; ok && owner != fullPath {
			return fmt.Errorf("%s already imported from %s — use :reset or qualified %s.%s",
				sym.Name, owner, shortName, sym.Name)
		}
	}

	// If already loaded, merge/replace symbols.
	for i, pkg := range s.loadedPkgs {
		if pkg.FullPath == fullPath {
			if pkg.Mode == ImportModeDot {
				return fmt.Errorf("%s already dot-imported (all symbols available)", shortName)
			}
			s.loadedPkgs[i].Mode = ImportModeSelective
			s.loadedPkgs[i].Symbols = symbols
			return nil
		}
	}

	s.loadedPkgs = append(s.loadedPkgs, LoadedPackage{
		FullPath:  fullPath,
		ShortName: shortName,
		Mode:      ImportModeSelective,
		Symbols:   symbols,
	})
	return nil
}

// ImportQualified adds a package as a qualified import (use pkg.Symbol).
// When symbols is non-nil, only those symbols are tracked for hints.
func (s *Session) ImportQualified(fullPath, shortName string, symbols []LoadedSymbol) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, pkg := range s.loadedPkgs {
		if pkg.FullPath == fullPath {
			s.loadedPkgs[i].Mode = ImportModeQualified
			s.loadedPkgs[i].Symbols = symbols
			return nil
		}
	}

	s.loadedPkgs = append(s.loadedPkgs, LoadedPackage{
		FullPath:  fullPath,
		ShortName: shortName,
		Mode:      ImportModeQualified,
		Symbols:   symbols,
	})
	return nil
}

// ImportBlank adds a side-effect-only import (import _ "pkg").
func (s *Session) ImportBlank(fullPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, pkg := range s.loadedPkgs {
		if pkg.FullPath == fullPath {
			return fmt.Errorf("%s already imported", fullPath)
		}
	}

	s.loadedPkgs = append(s.loadedPkgs, LoadedPackage{
		FullPath: fullPath,
		Mode:     ImportModeBlank,
	})
	return nil
}

// LoadedPackages returns a copy of the loaded packages list.
func (s *Session) LoadedPackages() []LoadedPackage {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]LoadedPackage, len(s.loadedPkgs))
	copy(out, s.loadedPkgs)
	return out
}

// Snapshot exports the session's current state for persistence.
// The caller receives copies — mutations won't affect the session.
func (s *Session) Snapshot() ([]*Cell, int, []LoadedPackage) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cells := make([]*Cell, len(s.cells))
	for i, c := range s.cells {
		cp := *c
		cells[i] = &cp
	}

	pkgs := make([]LoadedPackage, len(s.loadedPkgs))
	for i, p := range s.loadedPkgs {
		cp := p
		if p.Symbols != nil {
			cp.Symbols = make([]LoadedSymbol, len(p.Symbols))
			copy(cp.Symbols, p.Symbols)
		}
		pkgs[i] = cp
	}

	return cells, s.nextID, pkgs
}

// Restore replaces the session state with previously snapshotted data.
func (s *Session) Restore(cells []*Cell, nextID int, pkgs []LoadedPackage) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cells = cells
	s.nextID = nextID
	s.loadedPkgs = pkgs
}

// LoadedSymbolNames returns a map of symbol name → full package path for all
// selectively or qualified loaded symbols. Dot-imported and blank-imported
// packages are not included.
func (s *Session) LoadedSymbolNames() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadedSymbolNamesLocked()
}

func (s *Session) loadedSymbolNamesLocked() map[string]string {
	m := make(map[string]string)
	for _, pkg := range s.loadedPkgs {
		if pkg.Mode == ImportModeDot || pkg.Mode == ImportModeBlank {
			continue
		}
		for _, sym := range pkg.Symbols {
			m[sym.Name] = pkg.FullPath
		}
	}
	return m
}
