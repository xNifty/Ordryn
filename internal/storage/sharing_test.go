package storage

import (
	"testing"
)

func TestValidInviteRole(t *testing.T) {
	if !ValidInviteRole(RoleEditor) || !ValidInviteRole(RoleViewer) {
		t.Fatal("editor/viewer should be valid invite roles")
	}
	if ValidInviteRole(RoleOwner) || ValidInviteRole("admin") {
		t.Fatal("owner/admin should not be invite roles")
	}
}

func TestRoleCanWrite(t *testing.T) {
	if !RoleCanWrite(RoleOwner) || !RoleCanWrite(RoleEditor) {
		t.Fatal("owner/editor should write")
	}
	if RoleCanWrite(RoleViewer) {
		t.Fatal("viewer should not write")
	}
}

func TestTaskVisibleCondition(t *testing.T) {
	cond := TaskVisibleCondition("t", "$1")
	if cond == "" || len(cond) < 20 {
		t.Fatalf("unexpected condition: %s", cond)
	}
}
