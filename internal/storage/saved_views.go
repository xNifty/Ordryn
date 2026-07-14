package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const MaxSavedViewsPerUser = 20

var (
	ErrSavedViewNotFound     = errors.New("saved view not found")
	ErrSavedViewLimit        = errors.New("maximum saved views reached")
	ErrSavedViewNameConflict = errors.New("a saved view with this name already exists")
)

// SavedViewFilter contains the task-list query parameters stored by a view.
// Page is intentionally excluded so applying a view starts at the first page.
type SavedViewFilter struct {
	Project  string `json:"project"`
	Status   string `json:"status"`
	Due      string `json:"due"`
	Priority string `json:"priority"`
	Tag      string `json:"tag"`
	Sort     string `json:"sort"`
	Search   string `json:"search"`
}

// SavedView is a user-owned named task filter.
type SavedView struct {
	ID        int
	UserID    int
	Name      string
	Filter    SavedViewFilter
	SortOrder int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CreateSavedViewsTable ensures the saved-views schema exists.
func CreateSavedViewsTable() error {
	pool, err := OpenDatabase()
	if err != nil {
		return err
	}
	defer CloseDatabase(pool)

	_, err = pool.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS saved_views (
			id SERIAL PRIMARY KEY,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			name TEXT NOT NULL,
			filter_json JSONB NOT NULL DEFAULT '{}',
			sort_order INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (user_id, name)
		);
		ALTER TABLE saved_views
			ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
		CREATE INDEX IF NOT EXISTS idx_saved_views_user_id
			ON saved_views(user_id);
	`)
	if err != nil {
		return fmt.Errorf("failed to create saved_views table: %w", err)
	}
	return nil
}

// ListSavedViewsForUser returns all views owned by userID.
func ListSavedViewsForUser(userID int) ([]SavedView, error) {
	pool, err := OpenDatabase()
	if err != nil {
		return nil, err
	}
	defer CloseDatabase(pool)

	rows, err := pool.Query(context.Background(), `
		SELECT id, user_id, name, filter_json, sort_order, created_at, updated_at
		FROM saved_views
		WHERE user_id = $1
		ORDER BY sort_order ASC, LOWER(name) ASC, id ASC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list saved views: %w", err)
	}
	defer rows.Close()

	views := make([]SavedView, 0)
	for rows.Next() {
		view, err := scanSavedView(rows)
		if err != nil {
			return nil, err
		}
		views = append(views, *view)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to list saved views: %w", err)
	}
	return views, nil
}

// GetSavedViewByID returns a user-owned saved view.
func GetSavedViewByID(id, userID int) (*SavedView, error) {
	pool, err := OpenDatabase()
	if err != nil {
		return nil, err
	}
	defer CloseDatabase(pool)

	view, err := scanSavedView(pool.QueryRow(context.Background(), `
		SELECT id, user_id, name, filter_json, sort_order, created_at, updated_at
		FROM saved_views
		WHERE id = $1 AND user_id = $2
	`, id, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSavedViewNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get saved view: %w", err)
	}
	return view, nil
}

// CreateSavedView inserts a saved view, enforcing the per-user limit.
func CreateSavedView(userID int, name string, filter SavedViewFilter, sortOrder *int) (*SavedView, error) {
	pool, err := OpenDatabase()
	if err != nil {
		return nil, err
	}
	defer CloseDatabase(pool)

	tx, err := pool.Begin(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to begin saved view transaction: %w", err)
	}
	defer tx.Rollback(context.Background())

	// Lock the owner row so concurrent creates cannot exceed the view limit.
	var ownerID int
	if err := tx.QueryRow(context.Background(),
		"SELECT id FROM users WHERE id = $1 FOR UPDATE", userID).Scan(&ownerID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSavedViewNotFound
		}
		return nil, fmt.Errorf("failed to lock saved view owner: %w", err)
	}

	var count int
	if err := tx.QueryRow(context.Background(), `
		SELECT COUNT(*)
		FROM saved_views
		WHERE user_id = $1
	`, userID).Scan(&count); err != nil {
		return nil, fmt.Errorf("failed to count saved views: %w", err)
	}
	if count >= MaxSavedViewsPerUser {
		return nil, ErrSavedViewLimit
	}
	nextSortOrder := count
	if sortOrder != nil {
		nextSortOrder = *sortOrder
	}

	rawFilter, err := json.Marshal(filter)
	if err != nil {
		return nil, fmt.Errorf("failed to encode saved view filter: %w", err)
	}

	view, err := scanSavedView(tx.QueryRow(context.Background(), `
		INSERT INTO saved_views (user_id, name, filter_json, sort_order)
		VALUES ($1, $2, $3, $4)
		RETURNING id, user_id, name, filter_json, sort_order, created_at, updated_at
	`, userID, name, rawFilter, nextSortOrder))
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrSavedViewNameConflict
		}
		return nil, fmt.Errorf("failed to create saved view: %w", err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to commit saved view: %w", err)
	}
	return view, nil
}

// UpdateSavedView applies the supplied fields to a user-owned saved view.
func UpdateSavedView(id, userID int, name *string, filter *SavedViewFilter, sortOrder *int) (*SavedView, error) {
	pool, err := OpenDatabase()
	if err != nil {
		return nil, err
	}
	defer CloseDatabase(pool)

	var rawFilter any
	if filter != nil {
		encoded, err := json.Marshal(filter)
		if err != nil {
			return nil, fmt.Errorf("failed to encode saved view filter: %w", err)
		}
		rawFilter = encoded
	}

	view, err := scanSavedView(pool.QueryRow(context.Background(), `
		UPDATE saved_views
		SET name = COALESCE($1, name),
			filter_json = COALESCE($2, filter_json),
			sort_order = COALESCE($3, sort_order),
			updated_at = NOW()
		WHERE id = $4 AND user_id = $5
		RETURNING id, user_id, name, filter_json, sort_order, created_at, updated_at
	`, name, rawFilter, sortOrder, id, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSavedViewNotFound
	}
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrSavedViewNameConflict
		}
		return nil, fmt.Errorf("failed to update saved view: %w", err)
	}
	return view, nil
}

// DeleteSavedView removes a user-owned saved view.
func DeleteSavedView(id, userID int) error {
	pool, err := OpenDatabase()
	if err != nil {
		return err
	}
	defer CloseDatabase(pool)

	result, err := pool.Exec(context.Background(),
		"DELETE FROM saved_views WHERE id = $1 AND user_id = $2", id, userID)
	if err != nil {
		return fmt.Errorf("failed to delete saved view: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrSavedViewNotFound
	}
	return nil
}

type savedViewScanner interface {
	Scan(dest ...any) error
}

func scanSavedView(row savedViewScanner) (*SavedView, error) {
	var view SavedView
	var rawFilter []byte
	if err := row.Scan(
		&view.ID,
		&view.UserID,
		&view.Name,
		&rawFilter,
		&view.SortOrder,
		&view.CreatedAt,
		&view.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(rawFilter, &view.Filter); err != nil {
		return nil, fmt.Errorf("failed to decode saved view filter: %w", err)
	}
	return &view, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
