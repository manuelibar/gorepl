package session

// CellStatus represents the execution state of a cell.
type CellStatus string

const (
	CellPending  CellStatus = "pending"
	CellSuccess  CellStatus = "success"
	CellError    CellStatus = "error"
)

// Cell represents a single unit of code in the notebook session.
type Cell struct {
	ID     int        `json:"id"`
	Code   string     `json:"code"`
	Status CellStatus `json:"status"`
	Output string     `json:"output,omitempty"`
	Error  string     `json:"error,omitempty"`
}
