package domain

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"GoTodo/internal/storage"

	"github.com/jackc/pgx/v5"
)

func requireSiteAdmin(userID int) error {
	if !storage.UserHasSitePermission(userID, storage.SitePermAdmin) {
		return ErrForbidden
	}
	return nil
}

func normalizeSiteRoleName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", fmt.Errorf("%w: role name is required", ErrValidation)
	}
	if len(name) > storage.MaxSiteRoleNameLen {
		return "", fmt.Errorf("%w: role name must be %d characters or less", ErrValidation, storage.MaxSiteRoleNameLen)
	}
	return name, nil
}

// normalizeSitePerms dedupes perms and rejects anything outside the catalog.
func normalizeSitePerms(perms []string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, p := range perms {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		if !storage.IsSitePermInCatalog(p) {
			return nil, fmt.Errorf("%w: unknown site permission %q", ErrValidation, p)
		}
		seen[p] = true
		out = append(out, p)
	}
	return out, nil
}

func siteRoleStorageErr(err error) error {
	if errors.Is(err, storage.ErrSiteRoleNameTaken) {
		return fmt.Errorf("%w: a role with that name already exists", ErrConflict)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// ListSiteRolesForAdmin returns the assignable permission catalog and every site role.
func ListSiteRolesForAdmin(ctx context.Context, actorUserID int) ([]storage.SitePermInfo, []storage.SiteRole, error) {
	_ = ctx
	if err := requireSiteAdmin(actorUserID); err != nil {
		return nil, nil, err
	}
	roles, err := storage.ListSiteRoles()
	if err != nil {
		return nil, nil, err
	}
	return storage.SitePermissionCatalog(), roles, nil
}

// CreateSiteRoleForAdmin adds a custom site role.
func CreateSiteRoleForAdmin(ctx context.Context, actorUserID int, name string, perms []string) (*storage.SiteRole, error) {
	_ = ctx
	if err := requireSiteAdmin(actorUserID); err != nil {
		return nil, err
	}
	name, err := normalizeSiteRoleName(name)
	if err != nil {
		return nil, err
	}
	perms, err = normalizeSitePerms(perms)
	if err != nil {
		return nil, err
	}
	n, err := storage.CountCustomSiteRoles()
	if err != nil {
		return nil, err
	}
	if n >= storage.MaxSiteCustomRoles {
		return nil, fmt.Errorf("%w: at most %d custom site roles", ErrValidation, storage.MaxSiteCustomRoles)
	}
	role, err := storage.CreateSiteRole(name, perms)
	if err != nil {
		return nil, siteRoleStorageErr(err)
	}
	return role, nil
}

// UpdateSiteRoleForAdmin renames a role or replaces its catalog permissions. The admin
// role is locked, and the member role keeps its name because signups look it up by name.
func UpdateSiteRoleForAdmin(ctx context.Context, actorUserID, roleID int, name *string, perms *[]string) (*storage.SiteRole, error) {
	_ = ctx
	if err := requireSiteAdmin(actorUserID); err != nil {
		return nil, err
	}
	cur, err := storage.GetSiteRole(roleID)
	if err != nil {
		return nil, siteRoleStorageErr(err)
	}
	if cur.Name == storage.AdminRoleName {
		return nil, fmt.Errorf("%w: the Admin role always has every permission and can't be changed", ErrValidation)
	}
	if name != nil {
		n, err := normalizeSiteRoleName(*name)
		if err != nil {
			return nil, err
		}
		if cur.Name == storage.MemberRoleName && n != cur.Name {
			return nil, fmt.Errorf("%w: the default User role can't be renamed", ErrValidation)
		}
		name = &n
	}
	if perms != nil {
		p, err := normalizeSitePerms(*perms)
		if err != nil {
			return nil, err
		}
		perms = &p
	}
	updated, err := storage.UpdateSiteRole(roleID, name, perms)
	if err != nil {
		return nil, siteRoleStorageErr(err)
	}
	return updated, nil
}

// DeleteSiteRoleForAdmin deletes a custom role. Its accounts move to the User role;
// the number moved is returned.
func DeleteSiteRoleForAdmin(ctx context.Context, actorUserID, roleID int) (*storage.SiteRole, int, error) {
	_ = ctx
	if err := requireSiteAdmin(actorUserID); err != nil {
		return nil, 0, err
	}
	cur, err := storage.GetSiteRole(roleID)
	if err != nil {
		return nil, 0, siteRoleStorageErr(err)
	}
	if cur.BuiltIn() {
		return nil, 0, fmt.Errorf("%w: built-in roles can't be deleted", ErrValidation)
	}
	if storage.RoleGrantsAdmin(cur.Permissions) && cur.UserCount > 0 {
		admins, err := storage.CountActiveAdmins()
		if err != nil {
			return nil, 0, err
		}
		if admins-cur.UserCount < 1 {
			return nil, 0, fmt.Errorf("%w: deleting this role would leave the site without an admin", ErrConflict)
		}
	}
	moved, err := storage.DeleteSiteRole(roleID)
	if err != nil {
		return nil, 0, siteRoleStorageErr(err)
	}
	return cur, moved, nil
}

// SetUserSiteRoleForAdmin assigns a site role to an account and returns the old and new roles.
// Admins can't change their own role, which also keeps the site from losing its last admin.
func SetUserSiteRoleForAdmin(ctx context.Context, actorUserID, targetUserID, roleID int) (from, to *storage.SiteRole, err error) {
	_ = ctx
	if err := requireSiteAdmin(actorUserID); err != nil {
		return nil, nil, err
	}
	if targetUserID == actorUserID {
		return nil, nil, fmt.Errorf("%w: you can't change your own role; ask another admin", ErrValidation)
	}
	if storage.IsSystemUser(targetUserID) {
		return nil, nil, fmt.Errorf("%w: the automation account is managed by the system", ErrForbidden)
	}
	curID, isAgent, err := storage.GetUserSiteRoleID(targetUserID)
	if err != nil {
		return nil, nil, siteRoleStorageErr(err)
	}
	if isAgent {
		return nil, nil, fmt.Errorf("%w: agent accounts are managed per project", ErrForbidden)
	}
	to, err = storage.GetSiteRole(roleID)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: role not found", ErrValidation)
	}
	from, err = storage.GetSiteRole(curID)
	if err != nil {
		from = &storage.SiteRole{ID: curID, Permissions: []string{}}
	}
	if from.ID == to.ID {
		return from, to, nil
	}
	if storage.RoleGrantsAdmin(from.Permissions) && !storage.RoleGrantsAdmin(to.Permissions) {
		admins, err := storage.CountActiveAdmins()
		if err != nil {
			return nil, nil, err
		}
		if admins <= 1 {
			return nil, nil, fmt.Errorf("%w: the site needs at least one admin", ErrConflict)
		}
	}
	if err := storage.SetUserSiteRole(targetUserID, to.ID); err != nil {
		return nil, nil, siteRoleStorageErr(err)
	}
	return from, to, nil
}
