package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// AdminRoleName is the built-in site role that holds the admin permission.
const AdminRoleName = "admin"

// SitePermCreateInvites lets a role send site invites even when user invites are off.
const SitePermCreateInvites = "createinvites"

const (
	MaxSiteRoleNameLen = 50
	MaxSiteCustomRoles = 20
)

// ErrSiteRoleNameTaken is returned when a site role name is already in use (case-insensitive).
var ErrSiteRoleNameTaken = errors.New("site role name already in use")

// SiteRole is a row in roles with how many accounts use it.
type SiteRole struct {
	ID          int
	Name        string
	Permissions []string
	UserCount   int
}

// BuiltIn reports whether the role is one of the two roles the site depends on.
func (r SiteRole) BuiltIn() bool {
	return r.Name == AdminRoleName || r.Name == MemberRoleName
}

// SitePermInfo describes one site permission admins can grant to a role.
type SitePermInfo struct {
	ID          string
	Label       string
	Description string
	Group       string
}

// Admin is not in the catalog: only the built-in admin role holds it.
var sitePermCatalog = []SitePermInfo{
	{ID: SitePermProjectsCreate, Label: "Create projects", Description: "Start new projects", Group: "Create"},
	{ID: SitePermOrgsCreate, Label: "Create organizations", Description: "Start new organizations", Group: "Create"},
	{ID: SitePermCreateInvites, Label: "Send site invites", Description: "Invite people to join this site, even when user invites are turned off", Group: "Invites"},
}

// SitePermissionCatalog returns the site permissions admins can assign.
func SitePermissionCatalog() []SitePermInfo {
	out := make([]SitePermInfo, len(sitePermCatalog))
	copy(out, sitePermCatalog)
	return out
}

// IsSitePermInCatalog reports whether perm can be assigned from the admin UI.
func IsSitePermInCatalog(perm string) bool {
	for _, p := range sitePermCatalog {
		if p.ID == perm {
			return true
		}
	}
	return false
}

const siteRoleSelect = `
	SELECT r.id, r.name, r.permissions,
	       (SELECT COUNT(*) FROM users u WHERE u.role_id = r.id AND NOT COALESCE(u.is_agent, FALSE))
	FROM roles r`

func scanSiteRole(row pgx.Row, r *SiteRole) error {
	if err := row.Scan(&r.ID, &r.Name, &r.Permissions, &r.UserCount); err != nil {
		return err
	}
	if r.Permissions == nil {
		r.Permissions = []string{}
	}
	return nil
}

// ListSiteRoles returns admin, then user, then custom roles by name.
func ListSiteRoles() ([]SiteRole, error) {
	pool, err := OpenDatabase()
	if err != nil {
		return nil, err
	}
	defer CloseDatabase(pool)

	rows, err := pool.Query(context.Background(), siteRoleSelect+`
		ORDER BY CASE r.name WHEN 'admin' THEN 0 WHEN 'user' THEN 1 ELSE 2 END, LOWER(r.name), r.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SiteRole{}
	for rows.Next() {
		var r SiteRole
		if err := scanSiteRole(rows, &r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetSiteRole loads one role by id.
func GetSiteRole(id int) (*SiteRole, error) {
	pool, err := OpenDatabase()
	if err != nil {
		return nil, err
	}
	defer CloseDatabase(pool)

	var r SiteRole
	if err := scanSiteRole(pool.QueryRow(context.Background(), siteRoleSelect+` WHERE r.id = $1`, id), &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// CountCustomSiteRoles counts roles other than admin and user.
func CountCustomSiteRoles() (int, error) {
	pool, err := OpenDatabase()
	if err != nil {
		return 0, err
	}
	defer CloseDatabase(pool)

	var n int
	err = pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM roles WHERE name NOT IN ($1, $2)`, AdminRoleName, MemberRoleName).Scan(&n)
	return n, err
}

func siteRoleNameTaken(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, name string, exceptID int) (bool, error) {
	var taken bool
	err := q.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM roles WHERE LOWER(name) = LOWER($1) AND id <> $2)`, name, exceptID).Scan(&taken)
	return taken, err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// CreateSiteRole inserts a custom role.
func CreateSiteRole(name string, perms []string) (*SiteRole, error) {
	pool, err := OpenDatabase()
	if err != nil {
		return nil, err
	}
	defer CloseDatabase(pool)

	ctx := context.Background()
	if taken, err := siteRoleNameTaken(ctx, pool, name, 0); err != nil {
		return nil, err
	} else if taken {
		return nil, ErrSiteRoleNameTaken
	}
	if perms == nil {
		perms = []string{}
	}
	var id int
	if err := pool.QueryRow(ctx,
		`INSERT INTO roles (name, permissions) VALUES ($1, $2) RETURNING id`, name, perms).Scan(&id); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrSiteRoleNameTaken
		}
		return nil, fmt.Errorf("create site role: %w", err)
	}
	return GetSiteRole(id)
}

// UpdateSiteRole renames a role and/or sets its catalog permissions. Permissions
// outside the catalog (admin, legacy ones) are kept as they are.
func UpdateSiteRole(id int, name *string, perms *[]string) (*SiteRole, error) {
	pool, err := OpenDatabase()
	if err != nil {
		return nil, err
	}
	defer CloseDatabase(pool)

	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var current []string
	if err := tx.QueryRow(ctx, `SELECT permissions FROM roles WHERE id = $1 FOR UPDATE`, id).Scan(&current); err != nil {
		return nil, err
	}
	if name != nil {
		if taken, err := siteRoleNameTaken(ctx, tx, *name, id); err != nil {
			return nil, err
		} else if taken {
			return nil, ErrSiteRoleNameTaken
		}
		if _, err := tx.Exec(ctx, `UPDATE roles SET name = $2 WHERE id = $1`, id, *name); err != nil {
			if isUniqueViolation(err) {
				return nil, ErrSiteRoleNameTaken
			}
			return nil, err
		}
	}
	if perms != nil {
		next := []string{}
		for _, p := range current {
			if !IsSitePermInCatalog(p) {
				next = append(next, p)
			}
		}
		next = append(next, *perms...)
		if _, err := tx.Exec(ctx, `UPDATE roles SET permissions = $2 WHERE id = $1`, id, next); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return GetSiteRole(id)
}

// DeleteSiteRole moves the role's accounts to the member role, then deletes it.
// It returns how many accounts were moved.
func DeleteSiteRole(id int) (int, error) {
	pool, err := OpenDatabase()
	if err != nil {
		return 0, err
	}
	defer CloseDatabase(pool)

	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	var memberID int
	if err := tx.QueryRow(ctx, `SELECT id FROM roles WHERE name = $1`, MemberRoleName).Scan(&memberID); err != nil {
		return 0, fmt.Errorf("member role: %w", err)
	}
	if memberID == id {
		return 0, fmt.Errorf("cannot delete the member role")
	}
	tag, err := tx.Exec(ctx, `UPDATE users SET role_id = $2 WHERE role_id = $1`, id, memberID)
	if err != nil {
		return 0, err
	}
	del, err := tx.Exec(ctx, `DELETE FROM roles WHERE id = $1`, id)
	if err != nil {
		return 0, err
	}
	if del.RowsAffected() == 0 {
		return 0, pgx.ErrNoRows
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// GetUserSiteRoleID returns the user's role id, and whether the account is an agent.
func GetUserSiteRoleID(userID int) (roleID int, isAgent bool, err error) {
	pool, err := OpenDatabase()
	if err != nil {
		return 0, false, err
	}
	defer CloseDatabase(pool)

	err = pool.QueryRow(context.Background(),
		`SELECT role_id, COALESCE(is_agent, FALSE) FROM users WHERE id = $1`, userID).Scan(&roleID, &isAgent)
	return roleID, isAgent, err
}

// SetUserSiteRole assigns a site role to an account.
func SetUserSiteRole(userID, roleID int) error {
	pool, err := OpenDatabase()
	if err != nil {
		return err
	}
	defer CloseDatabase(pool)

	tag, err := pool.Exec(context.Background(), `UPDATE users SET role_id = $2 WHERE id = $1`, userID, roleID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// CountActiveAdmins counts unbanned, non-agent accounts whose role grants admin.
func CountActiveAdmins() (int, error) {
	pool, err := OpenDatabase()
	if err != nil {
		return 0, err
	}
	defer CloseDatabase(pool)

	var n int
	err = pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM users u
		JOIN roles r ON r.id = u.role_id
		WHERE $1::text = ANY(r.permissions)
		  AND NOT COALESCE(u.is_agent, FALSE)
		  AND NOT COALESCE(u.is_banned, FALSE)`, SitePermAdmin).Scan(&n)
	return n, err
}

// RoleGrantsAdmin reports whether the permission list includes admin.
func RoleGrantsAdmin(perms []string) bool {
	for _, p := range perms {
		if strings.EqualFold(p, SitePermAdmin) {
			return true
		}
	}
	return false
}
