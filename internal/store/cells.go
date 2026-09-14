package store

import (
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Cell kinds. Every project has exactly one 'source' cell (seq 1, created
// alongside the project); 'edit', 'upload', and 'text' cells are added
// afterward. 'text' cells hold freeform markdown notes/links about the
// video and never run.
const (
	KindSource = "source"
	KindEdit   = "edit"
	KindUpload = "upload"
	KindText   = "text"
)

// Cell statuses.
const (
	StatusIdle    = "idle"
	StatusRunning = "running"
	StatusDone    = "done"
	StatusError   = "error"
)

// Cell is one notebook step. Seq is a stable per-project identity number
// (1, 2, 3, ...) assigned once at creation and never reused or changed by
// reordering; Name is an optional user-given override of the default
// "<Kind> #<Seq>" label. Both can be used to refer to a cell.
type Cell struct {
	ID             string
	ProjectID      string
	Seq            int
	ParentCellID   *string // nil only for the source cell
	Name           *string // nil = use DefaultName(Kind, Seq)
	Kind           string
	ParamsJSON     string
	Status         string
	StatusMessage  *string
	OutputPath     *string
	SourceFilename *string // source cells only: the original dropped-in filename
	YouTubeURL     *string
	Position       int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// DefaultName returns a cell's label when it has no user-given name.
func DefaultName(kind string, seq int) string {
	switch kind {
	case KindSource:
		return "Source"
	case KindEdit:
		return "Edit #" + strconv.Itoa(seq)
	case KindUpload:
		return "Upload #" + strconv.Itoa(seq)
	case KindText:
		return "Note #" + strconv.Itoa(seq)
	default:
		return "Cell #" + strconv.Itoa(seq)
	}
}

// DisplayName returns c.Name if set, else its default label.
func (c *Cell) DisplayName() string {
	if c.Name != nil && *c.Name != "" {
		return *c.Name
	}
	return DefaultName(c.Kind, c.Seq)
}

// CreateCell inserts a new edit/upload cell. seq should come from
// DB.NextSeq(projectID). name may be nil to use the default label.
func (db *DB) CreateCell(projectID string, seq int, parentCellID string, name *string, kind, paramsJSON string, position int) (*Cell, error) {
	c := &Cell{
		ID:           uuid.NewString(),
		ProjectID:    projectID,
		Seq:          seq,
		ParentCellID: &parentCellID,
		Name:         name,
		Kind:         kind,
		ParamsJSON:   paramsJSON,
		Status:       StatusIdle,
		Position:     position,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	_, err := db.sql.Exec(
		`INSERT INTO cells (id, project_id, seq, parent_cell_id, name, kind, params_json, status, position, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.ProjectID, c.Seq, c.ParentCellID, c.Name, c.Kind, c.ParamsJSON, c.Status, c.Position, c.CreatedAt.Unix(), c.UpdatedAt.Unix(),
	)
	if err != nil {
		return nil, fmt.Errorf("creating cell: %w", err)
	}
	return c, nil
}

// ListCellsByProject returns a project's cells in display order.
func (db *DB) ListCellsByProject(projectID string) ([]*Cell, error) {
	rows, err := db.sql.Query(cellSelect+` WHERE project_id = ? ORDER BY position ASC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("listing cells: %w", err)
	}
	defer rows.Close()

	var out []*Cell
	for rows.Next() {
		c, err := scanCell(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetCell returns one cell by id.
func (db *DB) GetCell(id string) (*Cell, error) {
	row := db.sql.QueryRow(cellSelect+` WHERE id = ?`, id)
	c, err := scanCell(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("cell %s not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("getting cell: %w", err)
	}
	return c, nil
}

// GetCellByRef resolves a cell within a project by either its seq number
// (e.g. "2") or a case-insensitive match on its display name (custom name
// if set, else the default "<Kind> #<Seq>" label) — the two ways the UI
// and API let you refer to a cell.
func (db *DB) GetCellByRef(projectID, ref string) (*Cell, error) {
	cells, err := db.ListCellsByProject(projectID)
	if err != nil {
		return nil, err
	}
	if seq, err := strconv.Atoi(strings.TrimPrefix(ref, "#")); err == nil {
		for _, c := range cells {
			if c.Seq == seq {
				return c, nil
			}
		}
	}
	for _, c := range cells {
		if strings.EqualFold(c.DisplayName(), ref) {
			return c, nil
		}
	}
	return nil, fmt.Errorf("no cell matching %q in project %s", ref, projectID)
}

// UpdateCellStatus sets a cell's status and latest status/progress message.
func (db *DB) UpdateCellStatus(id, status string, message string) error {
	res, err := db.sql.Exec(
		`UPDATE cells SET status = ?, status_message = ?, updated_at = ? WHERE id = ?`,
		status, message, time.Now().Unix(), id,
	)
	if err != nil {
		return fmt.Errorf("updating cell status: %w", err)
	}
	return checkAffected(res, "cell", id)
}

// SetCellOutput records a source/edit cell's output video path and marks
// it done.
func (db *DB) SetCellOutput(id, outputPath string) error {
	res, err := db.sql.Exec(
		`UPDATE cells SET output_path = ?, status = ?, updated_at = ? WHERE id = ?`,
		outputPath, StatusDone, time.Now().Unix(), id,
	)
	if err != nil {
		return fmt.Errorf("setting cell output: %w", err)
	}
	return checkAffected(res, "cell", id)
}

// SetSourceCellVideo records the dropped-in video on the project's source
// cell and marks it done.
func (db *DB) SetSourceCellVideo(id, path, filename string) error {
	res, err := db.sql.Exec(
		`UPDATE cells SET output_path = ?, source_filename = ?, status = ?, updated_at = ? WHERE id = ?`,
		path, filename, StatusDone, time.Now().Unix(), id,
	)
	if err != nil {
		return fmt.Errorf("setting source cell video: %w", err)
	}
	return checkAffected(res, "cell", id)
}

// SetCellYouTubeURL records an upload cell's resulting video link. It does
// not change status, since the link is known before processing finishes.
func (db *DB) SetCellYouTubeURL(id, url string) error {
	res, err := db.sql.Exec(
		`UPDATE cells SET youtube_url = ?, updated_at = ? WHERE id = ?`,
		url, time.Now().Unix(), id,
	)
	if err != nil {
		return fmt.Errorf("setting cell youtube url: %w", err)
	}
	return checkAffected(res, "cell", id)
}

// RenameCell sets (or clears, if name is "") a cell's custom display name.
func (db *DB) RenameCell(id, name string) error {
	var namePtr *string
	if name != "" {
		namePtr = &name
	}
	res, err := db.sql.Exec(`UPDATE cells SET name = ?, updated_at = ? WHERE id = ?`, namePtr, time.Now().Unix(), id)
	if err != nil {
		return fmt.Errorf("renaming cell: %w", err)
	}
	return checkAffected(res, "cell", id)
}

// UpdateCellParams replaces a cell's params (only sensible while idle).
func (db *DB) UpdateCellParams(id, paramsJSON string) error {
	res, err := db.sql.Exec(`UPDATE cells SET params_json = ?, updated_at = ? WHERE id = ?`, paramsJSON, time.Now().Unix(), id)
	if err != nil {
		return fmt.Errorf("updating cell params: %w", err)
	}
	return checkAffected(res, "cell", id)
}

// ReorderCells reassigns the display position of exactly the cells in
// orderedIDs (which must all belong to projectID and be of kind) to
// match that order. It reuses the same set of position values those
// cells already occupy (rather than renumbering the whole project), so
// cells of other kinds interleaved in the position sequence are
// undisturbed. seq numbers are never touched. The whole operation is one
// transaction: a partial failure leaves positions as they were.
func (db *DB) ReorderCells(projectID, kind string, orderedIDs []string) error {
	if len(orderedIDs) == 0 {
		return nil
	}

	tx, err := db.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	placeholders := make([]string, len(orderedIDs))
	args := make([]any, len(orderedIDs))
	for i, id := range orderedIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	rows, err := tx.Query(
		`SELECT id, position FROM cells WHERE project_id = ? AND kind = ? AND id IN (`+strings.Join(placeholders, ",")+`)`,
		append([]any{projectID, kind}, args...)...,
	)
	if err != nil {
		return fmt.Errorf("reordering cells: %w", err)
	}
	positionByID := make(map[string]int, len(orderedIDs))
	var slots []int
	for rows.Next() {
		var id string
		var pos int
		if err := rows.Scan(&id, &pos); err != nil {
			rows.Close()
			return err
		}
		positionByID[id] = pos
		slots = append(slots, pos)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()

	if len(positionByID) != len(orderedIDs) {
		return fmt.Errorf("reordering cells: one or more ids are not %s cells in project %s", kind, projectID)
	}
	sort.Ints(slots)

	for i, id := range orderedIDs {
		if _, err := tx.Exec(`UPDATE cells SET position = ?, updated_at = ? WHERE id = ?`, slots[i], time.Now().Unix(), id); err != nil {
			return fmt.Errorf("reordering cells: %w", err)
		}
	}
	return tx.Commit()
}

// DeleteCell removes a cell. Callers should refuse to delete a project's
// source cell (kind == KindSource) at the handler layer, since a project
// without one no longer makes sense; the store itself stays generic.
func (db *DB) DeleteCell(id string) error {
	res, err := db.sql.Exec(`DELETE FROM cells WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting cell: %w", err)
	}
	return checkAffected(res, "cell", id)
}

const cellSelect = `SELECT id, project_id, seq, parent_cell_id, name, kind, params_json, status, status_message, output_path, source_filename, youtube_url, position, created_at, updated_at FROM cells`

func scanCell(row scannable) (*Cell, error) {
	var c Cell
	var created, updated int64
	if err := row.Scan(
		&c.ID, &c.ProjectID, &c.Seq, &c.ParentCellID, &c.Name, &c.Kind, &c.ParamsJSON,
		&c.Status, &c.StatusMessage, &c.OutputPath, &c.SourceFilename, &c.YouTubeURL, &c.Position,
		&created, &updated,
	); err != nil {
		return nil, err
	}
	c.CreatedAt = time.Unix(created, 0)
	c.UpdatedAt = time.Unix(updated, 0)
	return &c, nil
}
