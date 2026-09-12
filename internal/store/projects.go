package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Project is one "notebook": a name plus an ordered list of Cells,
// always starting with one 'source' cell (seq 1).
type Project struct {
	ID        string
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CreateProject inserts a new project along with its seq=1 source cell
// (the "Add" cell that a video gets dropped onto), and returns both.
func (db *DB) CreateProject(name string) (*Project, *Cell, error) {
	now := time.Now()
	p := &Project{ID: uuid.NewString(), Name: name, CreatedAt: now, UpdatedAt: now}

	tx, err := db.sql.Begin()
	if err != nil {
		return nil, nil, fmt.Errorf("creating project: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`INSERT INTO projects (id, name, next_seq, created_at, updated_at) VALUES (?, ?, 2, ?, ?)`,
		p.ID, p.Name, now.Unix(), now.Unix(),
	); err != nil {
		return nil, nil, fmt.Errorf("creating project: %w", err)
	}

	source := &Cell{
		ID:         uuid.NewString(),
		ProjectID:  p.ID,
		Seq:        1,
		Kind:       KindSource,
		ParamsJSON: "{}",
		Status:     StatusIdle,
		Position:   0,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if _, err := tx.Exec(
		`INSERT INTO cells (id, project_id, seq, parent_cell_id, name, kind, params_json, status, position, created_at, updated_at)
		 VALUES (?, ?, ?, NULL, NULL, ?, ?, ?, ?, ?, ?)`,
		source.ID, source.ProjectID, source.Seq, source.Kind, source.ParamsJSON, source.Status, source.Position, now.Unix(), now.Unix(),
	); err != nil {
		return nil, nil, fmt.Errorf("creating source cell: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("creating project: %w", err)
	}
	return p, source, nil
}

// ListProjects returns all projects, newest first.
func (db *DB) ListProjects() ([]*Project, error) {
	rows, err := db.sql.Query(`SELECT id, name, created_at, updated_at FROM projects ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("listing projects: %w", err)
	}
	defer rows.Close()

	var out []*Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetProject returns one project by id, or an error if it doesn't exist.
func (db *DB) GetProject(id string) (*Project, error) {
	row := db.sql.QueryRow(`SELECT id, name, created_at, updated_at FROM projects WHERE id = ?`, id)
	p, err := scanProject(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("project %s not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("getting project: %w", err)
	}
	return p, nil
}

// NextSeq allocates and returns the next cell sequence number for a
// project, incrementing its counter so numbers are never reused even if
// the cell that used them is later deleted.
func (db *DB) NextSeq(projectID string) (int, error) {
	tx, err := db.sql.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var seq int
	if err := tx.QueryRow(`SELECT next_seq FROM projects WHERE id = ?`, projectID).Scan(&seq); err != nil {
		if err == sql.ErrNoRows {
			return 0, fmt.Errorf("project %s not found", projectID)
		}
		return 0, err
	}
	if _, err := tx.Exec(`UPDATE projects SET next_seq = ?, updated_at = ? WHERE id = ?`, seq+1, time.Now().Unix(), projectID); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return seq, nil
}

// DeleteProject removes a project and (via ON DELETE CASCADE) its cells.
func (db *DB) DeleteProject(id string) error {
	res, err := db.sql.Exec(`DELETE FROM projects WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting project: %w", err)
	}
	return checkAffected(res, "project", id)
}

type scannable interface {
	Scan(dest ...any) error
}

func scanProject(row scannable) (*Project, error) {
	var p Project
	var created, updated int64
	if err := row.Scan(&p.ID, &p.Name, &created, &updated); err != nil {
		return nil, err
	}
	p.CreatedAt = time.Unix(created, 0)
	p.UpdatedAt = time.Unix(updated, 0)
	return &p, nil
}

func checkAffected(res sql.Result, kind, id string) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%s %s not found", kind, id)
	}
	return nil
}
