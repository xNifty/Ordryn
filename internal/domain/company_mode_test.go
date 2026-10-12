package domain

import (
	"context"
	"errors"
	"testing"

	"GoTodo/internal/storage"
)

func insertUserWithRole(t *testing.T, email, roleName string) int {
	t.Helper()
	pool, err := storage.OpenDatabase()
	if err != nil {
		t.Fatal(err)
	}
	var id int
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO users (email, password, role_id, user_name)
		VALUES ($1, 'x', (SELECT id FROM roles WHERE name = $2), $1) RETURNING id`, email, roleName).Scan(&id); err != nil {
		t.Fatalf("insert %s: %v", email, err)
	}
	return id
}

func companyMemberRole(t *testing.T, orgID, userID int) string {
	t.Helper()
	role, err := storage.GetOrganizationRole(orgID, userID)
	if err != nil {
		t.Fatal(err)
	}
	return role
}

func TestCompanyMode(t *testing.T) {
	useFreshDatabase(t, "gotodo_company_mode_test")
	ctx := context.Background()

	admin := insertUserWithRole(t, "boss@example.com", storage.AdminRoleName)
	alice := insertUserWithRole(t, "alice@example.com", storage.MemberRoleName)
	bob := insertUserWithRole(t, "bob@example.com", storage.MemberRoleName)

	// Choosing company mode creates the company, enrolls everyone, and leaves team
	// creation to admins. The company itself doesn't lock the mode.
	if err := storage.ChangeSiteMode(storage.SiteModeCompany); err != nil {
		t.Fatalf("enter company mode: %v", err)
	}
	companyID, err := storage.CompanyOrganizationID()
	if err != nil || companyID == 0 {
		t.Fatalf("company id = %d, %v", companyID, err)
	}
	for _, u := range []int{admin, alice, bob} {
		if role := companyMemberRole(t, companyID, u); role != storage.CompanyMemberRole {
			t.Fatalf("user %d company role = %q, want member", u, role)
		}
	}
	if has, _ := storage.RoleHasPermission(storage.MemberRoleName, storage.SitePermOrgsCreate); has {
		t.Fatal("company mode should stop regular users creating teams")
	}
	if locked, _ := storage.SiteModeLocked(); locked {
		t.Fatal("the company organization alone must not lock the mode")
	}

	// Switching back while still empty removes the company; switching again restores it.
	if err := storage.ChangeSiteMode(storage.SiteModeMulti); err != nil {
		t.Fatalf("leave company mode: %v", err)
	}
	if id, _ := storage.CompanyOrganizationID(); id != 0 {
		t.Fatalf("company should be removed in multi mode, got %d", id)
	}
	if err := storage.ChangeSiteMode(storage.SiteModeCompany); err != nil {
		t.Fatalf("re-enter company mode: %v", err)
	}
	companyID, _ = storage.CompanyOrganizationID()

	// New accounts join automatically.
	carol, err := storage.CreateUser("carol@example.com", "x", "UTC", mustRoleID(t, storage.MemberRoleName))
	if err != nil {
		t.Fatal(err)
	}
	if role := companyMemberRole(t, companyID, carol); role != storage.CompanyMemberRole {
		t.Fatalf("new account company role = %q, want member", role)
	}

	// A regular user's project lands in the company with only its creator on it.
	proj, err := CreateProject(ctx, alice, "Alice's project", "")
	if err != nil {
		t.Fatalf("regular user create project: %v", err)
	}
	if proj.OrganizationID == nil || *proj.OrganizationID != companyID {
		t.Fatalf("project organization = %v, want company %d", proj.OrganizationID, companyID)
	}
	if _, err := storage.GetAccessibleProjectByID(proj.ID, bob); err == nil {
		t.Fatal("company projects must stay members-only")
	}
	if err := storage.ChangeSiteMode(storage.SiteModeMulti); !errors.Is(err, storage.ErrSiteModeLocked) {
		t.Fatalf("switch after a project exists: err=%v, want ErrSiteModeLocked", err)
	}

	// The company's project list hides projects a regular member can't open.
	bobView, err := ListOrganizationProjectRostersForUser(ctx, bob, companyID)
	if err != nil {
		t.Fatal(err)
	}
	if len(bobView) != 0 {
		t.Fatalf("bob sees %d company projects, want 0", len(bobView))
	}
	aliceView, _ := ListOrganizationProjectRostersForUser(ctx, alice, companyID)
	adminView, _ := ListOrganizationProjectRostersForUser(ctx, admin, companyID)
	if len(aliceView) != 1 || len(adminView) != 1 {
		t.Fatalf("alice sees %d, admin sees %d; want 1 each", len(aliceView), len(adminView))
	}

	// Site admins manage the company; regular members don't.
	newName := "Acme"
	if _, err := UpdateOrganizationForUser(ctx, admin, companyID, &newName, nil); err != nil {
		t.Fatalf("admin rename company: %v", err)
	}
	if _, err := UpdateOrganizationForUser(ctx, bob, companyID, &newName, nil); !errors.Is(err, ErrForbidden) {
		t.Fatalf("member rename company: err=%v, want ErrForbidden", err)
	}

	// Nobody can delete, leave, be removed from, or be invited to the company.
	if err := DeleteOrganizationForUser(ctx, admin, companyID); !errors.Is(err, ErrValidation) {
		t.Fatalf("delete company: err=%v, want ErrValidation", err)
	}
	if err := RemoveOrganizationMemberForUser(ctx, bob, companyID, bob); !errors.Is(err, ErrValidation) {
		t.Fatalf("leave company: err=%v, want ErrValidation", err)
	}
	if err := RemoveOrganizationMemberForUser(ctx, admin, companyID, bob); !errors.Is(err, ErrValidation) {
		t.Fatalf("remove from company: err=%v, want ErrValidation", err)
	}
	if _, err := InviteToOrganization(ctx, admin, companyID, "bob@example.com", storage.CompanyMemberRole); !errors.Is(err, ErrValidation) {
		t.Fatalf("invite to company: err=%v, want ErrValidation", err)
	}

	// Teams: admins create them; regular users can't by default.
	if _, err := CreateOrganizationForUser(ctx, bob, "Bob's team", ""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("member create team: err=%v, want ErrForbidden", err)
	}
	team, err := CreateOrganizationForUser(ctx, admin, "Platform", "")
	if err != nil {
		t.Fatalf("admin create team: %v", err)
	}
	if team.IsCompany {
		t.Fatal("a team must not be the company")
	}
	tid := team.ID
	teamProj, err := CreateProjectForUser(ctx, admin, CreateProjectInput{
		Name: "Team board", OrganizationID: &tid, ImportMode: storage.OrgImportCopy,
	})
	if err != nil {
		t.Fatalf("team project: %v", err)
	}
	if err := DeleteOrganizationForUser(ctx, admin, team.ID); err != nil {
		t.Fatalf("delete team: %v", err)
	}
	moved, err := storage.GetProjectByID(teamProj.ID, admin)
	if err != nil {
		t.Fatal(err)
	}
	if moved.OrganizationID == nil || *moved.OrganizationID != companyID {
		t.Fatalf("after deleting its team, project organization = %v; want the company", moved.OrganizationID)
	}

	// Deleting the account that created the company must not delete the company.
	pool, err := storage.OpenDatabase()
	if err != nil {
		t.Fatal(err)
	}
	var creator int
	if err := pool.QueryRow(ctx, `SELECT created_by FROM organizations WHERE id = $1`, companyID).Scan(&creator); err != nil {
		t.Fatal(err)
	}
	if creator != admin {
		t.Fatalf("company created_by = %d, want the admin %d", creator, admin)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, creator); err == nil {
		t.Fatal("deleting the company's creator should be refused")
	}
	if id, _ := storage.CompanyOrganizationID(); id != companyID {
		t.Fatal("company must survive an attempt to delete its creator")
	}
}

func TestMultiModeProjectsStayPersonal(t *testing.T) {
	proj, err := CreateProject(context.Background(), 1, "Personal in multi mode", "")
	if err != nil {
		t.Fatal(err)
	}
	if proj.OrganizationID != nil {
		t.Fatalf("multi mode project organization = %v, want none", *proj.OrganizationID)
	}
}
