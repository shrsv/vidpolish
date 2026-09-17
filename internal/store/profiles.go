package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Profile is a named, saved set of edit-cell params (see
// server.ProfileParams) that can be applied to any edit cell later.
// Profiles are global, not scoped to a project.
type Profile struct {
	ID         string
	Name       string
	ParamsJSON string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// CreateProfile inserts a new named profile.
func (db *DB) CreateProfile(name, paramsJSON string) (*Profile, error) {
	now := time.Now()
	p := &Profile{ID: uuid.NewString(), Name: name, ParamsJSON: paramsJSON, CreatedAt: now, UpdatedAt: now}
	_, err := db.sql.Exec(
		`INSERT INTO profiles (id, name, params_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.ParamsJSON, now.Unix(), now.Unix(),
	)
	if err != nil {
		if isUniqueConstraintErr(err) {
			return nil, fmt.Errorf("a profile named %q already exists", name)
		}
		return nil, fmt.Errorf("creating profile: %w", err)
	}
	return p, nil
}

// ListProfiles returns all profiles, alphabetically by name.
func (db *DB) ListProfiles() ([]*Profile, error) {
	rows, err := db.sql.Query(`SELECT id, name, params_json, created_at, updated_at FROM profiles ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("listing profiles: %w", err)
	}
	defer rows.Close()

	var out []*Profile
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetProfile returns one profile by id.
func (db *DB) GetProfile(id string) (*Profile, error) {
	row := db.sql.QueryRow(`SELECT id, name, params_json, created_at, updated_at FROM profiles WHERE id = ?`, id)
	p, err := scanProfile(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("profile %s not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("getting profile: %w", err)
	}
	return p, nil
}

// GetProfileByName returns one profile by its (case-sensitive) name.
func (db *DB) GetProfileByName(name string) (*Profile, error) {
	row := db.sql.QueryRow(`SELECT id, name, params_json, created_at, updated_at FROM profiles WHERE name = ?`, name)
	p, err := scanProfile(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("profile %q not found", name)
	}
	if err != nil {
		return nil, fmt.Errorf("getting profile: %w", err)
	}
	return p, nil
}

// UpdateProfile renames and/or overwrites the params of an existing profile.
func (db *DB) UpdateProfile(id, name, paramsJSON string) error {
	res, err := db.sql.Exec(
		`UPDATE profiles SET name = ?, params_json = ?, updated_at = ? WHERE id = ?`,
		name, paramsJSON, time.Now().Unix(), id,
	)
	if err != nil {
		if isUniqueConstraintErr(err) {
			return fmt.Errorf("a profile named %q already exists", name)
		}
		return fmt.Errorf("updating profile: %w", err)
	}
	return checkAffected(res, "profile", id)
}

// DeleteProfile removes a profile.
func (db *DB) DeleteProfile(id string) error {
	res, err := db.sql.Exec(`DELETE FROM profiles WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting profile: %w", err)
	}
	return checkAffected(res, "profile", id)
}

func scanProfile(row scannable) (*Profile, error) {
	var p Profile
	var created, updated int64
	if err := row.Scan(&p.ID, &p.Name, &p.ParamsJSON, &created, &updated); err != nil {
		return nil, err
	}
	p.CreatedAt = time.Unix(created, 0)
	p.UpdatedAt = time.Unix(updated, 0)
	return &p, nil
}

func isUniqueConstraintErr(err error) bool {
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}
