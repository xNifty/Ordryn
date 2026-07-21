package domain

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"GoTodo/internal/storage"

	"github.com/jackc/pgx/v5"
)

const defaultInviteTTL = 14 * 24 * time.Hour

// ProjectInviteAckMessage is returned for all invite attempts (privacy-preserving).
const ProjectInviteAckMessage = "If a user with this email is in the system they will be sent an invite."

// InviteToProject attempts to create a pending invite (owner only).
// It only creates an invite when the email belongs to an existing user who
// allows project invites. Callers must always show ProjectInviteAckMessage
// and must not reveal whether the invite was created.
func InviteToProject(ctx context.Context, actorUserID, projectID int, email, role string) error {
	_ = ctx
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return fmt.Errorf("%w: email is required", ErrValidation)
	}
	role = strings.TrimSpace(strings.ToLower(role))
	if !storage.ValidInviteRole(role) {
		return fmt.Errorf("%w: role must be editor or viewer", ErrValidation)
	}
	proj, err := storage.GetAccessibleProjectByID(projectID, actorUserID)
	if err != nil {
		return ErrNotFound
	}
	if !storage.RoleCanManage(proj.Role) {
		return ErrForbidden
	}
	// Privacy: never reveal whether the email exists, is the owner, opted out, or is already a member.
	if strings.EqualFold(email, proj.OwnerEmail) {
		return nil
	}
	user, err := storage.GetUserByEmail(email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		// Treat lookup misses the same for privacy; only fail on unexpected errors after a found user.
		if strings.Contains(strings.ToLower(err.Error()), "no rows") {
			return nil
		}
		return err
	}
	if user == nil {
		return nil
	}
	allow, err := storage.UserAllowsProjectInvites(user.ID)
	if err != nil || !allow {
		return nil
	}
	existingRole, err := storage.GetProjectRole(projectID, user.ID)
	if err != nil {
		return err
	}
	if existingRole != "" {
		return nil
	}
	inv, err := storage.CreateProjectInvite(projectID, email, role, actorUserID, time.Now().Add(defaultInviteTTL))
	if err != nil {
		return err
	}
	_ = storage.LogProjectEvent(projectID, actorUserID, "invited", map[string]interface{}{
		"role": role, "invite_id": inv.ID,
	})
	return nil
}

// UpdateProjectMemberRole changes a member's role (owner only). Cannot change owner role via this.
func UpdateProjectMemberRole(ctx context.Context, actorUserID, projectID, memberUserID int, role string) error {
	_ = ctx
	role = strings.TrimSpace(strings.ToLower(role))
	if role != storage.RoleEditor && role != storage.RoleViewer {
		return fmt.Errorf("%w: role must be editor or viewer", ErrValidation)
	}
	proj, err := storage.GetAccessibleProjectByID(projectID, actorUserID)
	if err != nil {
		return ErrNotFound
	}
	if !storage.RoleCanManage(proj.Role) {
		return ErrForbidden
	}
	current, err := storage.GetProjectRole(projectID, memberUserID)
	if err != nil {
		return err
	}
	if current == "" {
		return ErrNotFound
	}
	if current == storage.RoleOwner {
		return fmt.Errorf("%w: cannot change owner role", ErrValidation)
	}
	if err := storage.UpsertProjectMember(projectID, memberUserID, role); err != nil {
		return err
	}
	_ = storage.LogProjectEvent(projectID, actorUserID, "role_changed", map[string]interface{}{
		"user_id": memberUserID, "role": role,
	})
	return nil
}

// RemoveProjectMember removes a non-owner member, or allows self-leave.
func RemoveProjectMember(ctx context.Context, actorUserID, projectID, memberUserID int) error {
	_ = ctx
	proj, err := storage.GetAccessibleProjectByID(projectID, actorUserID)
	if err != nil {
		return ErrNotFound
	}
	selfLeave := actorUserID == memberUserID
	if !selfLeave && !storage.RoleCanManage(proj.Role) {
		return ErrForbidden
	}
	targetRole, err := storage.GetProjectRole(projectID, memberUserID)
	if err != nil {
		return err
	}
	if targetRole == "" {
		return ErrNotFound
	}
	if targetRole == storage.RoleOwner {
		return fmt.Errorf("%w: cannot remove the project owner", ErrValidation)
	}
	if err := storage.RemoveProjectMember(projectID, memberUserID); err != nil {
		return ErrNotFound
	}
	event := "removed"
	if selfLeave {
		event = "left"
	}
	_ = storage.LogProjectEvent(projectID, actorUserID, event, map[string]interface{}{
		"user_id": memberUserID,
	})
	return nil
}

// AcceptProjectInvite accepts a pending invite for the current user.
func AcceptProjectInvite(ctx context.Context, userID int, userEmail string, inviteID int) error {
	_ = ctx
	inv, err := storage.GetProjectInviteByID(inviteID)
	if err != nil {
		return ErrNotFound
	}
	if err := storage.AcceptProjectInvite(inviteID, userID, userEmail); err != nil {
		if strings.Contains(err.Error(), "mismatch") || strings.Contains(err.Error(), "expired") || strings.Contains(err.Error(), "accepted") {
			return fmt.Errorf("%w: %s", ErrValidation, err.Error())
		}
		return err
	}
	_ = storage.LogProjectEvent(inv.ProjectID, userID, "accepted", map[string]interface{}{
		"invite_id": inviteID, "role": inv.Role,
	})
	return nil
}

// DeclineProjectInvite declines a pending invite.
func DeclineProjectInvite(ctx context.Context, userEmail string, inviteID int) error {
	_ = ctx
	if err := storage.DeclineProjectInvite(inviteID, userEmail); err != nil {
		return ErrNotFound
	}
	return nil
}

// CreateShareLinkForScope creates a read-only share link for a project or tag.
func CreateShareLinkForScope(ctx context.Context, userID int, scopeType string, scopeID int, expiresAt *time.Time) (*storage.ShareLink, error) {
	_ = ctx
	scopeType = strings.TrimSpace(strings.ToLower(scopeType))
	switch scopeType {
	case storage.ShareScopeProject:
		proj, err := storage.GetAccessibleProjectByID(scopeID, userID)
		if err != nil {
			return nil, ErrNotFound
		}
		if !storage.RoleCanManage(proj.Role) {
			return nil, ErrForbidden
		}
	case storage.ShareScopeTag:
		ok, err := storage.GetTagOwnedByUser(scopeID, userID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrNotFound
		}
	default:
		return nil, fmt.Errorf("%w: scope_type must be project or tag", ErrValidation)
	}
	link, err := storage.CreateShareLink(userID, scopeType, scopeID, expiresAt)
	if err != nil {
		return nil, err
	}
	if scopeType == storage.ShareScopeProject {
		_ = storage.LogProjectEvent(scopeID, userID, "link_created", map[string]interface{}{
			"share_link_id": link.ID,
		})
	}
	return link, nil
}

// RevokeShareLinkForUser revokes a share link owned by the user.
func RevokeShareLinkForUser(ctx context.Context, userID, linkID int) error {
	_ = ctx
	link, err := storage.GetShareLinkByID(linkID)
	if err != nil {
		return ErrNotFound
	}
	if err := storage.RevokeShareLink(linkID, userID); err != nil {
		return ErrNotFound
	}
	if link.ScopeType == storage.ShareScopeProject {
		_ = storage.LogProjectEvent(link.ScopeID, userID, "link_revoked", map[string]interface{}{
			"share_link_id": linkID,
		})
	}
	return nil
}
