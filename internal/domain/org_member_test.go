package domain

import "testing"

func TestRoleAtLeast(t *testing.T) {
	cases := []struct {
		role    Role
		require Role
		want    bool
	}{
		{RoleOwner, RoleAdmin, true},
		{RoleOwner, RoleOwner, true},
		{RoleAdmin, RoleOwner, false},
		{RoleAdmin, RoleMember, true},
		{RoleMember, RoleAdmin, false},
		{RoleMember, RoleViewer, true},
		{RoleViewer, RoleMember, false},
		{Role("bogus"), RoleViewer, false},
	}
	for _, tc := range cases {
		if got := tc.role.AtLeast(tc.require); got != tc.want {
			t.Fatalf("%s.AtLeast(%s) = %v, want %v", tc.role, tc.require, got, tc.want)
		}
	}
}

func TestRoleValid(t *testing.T) {
	valid := []Role{RoleOwner, RoleAdmin, RoleMember, RoleViewer}
	for _, r := range valid {
		if !r.Valid() {
			t.Fatalf("%s should be valid", r)
		}
	}
	if Role("").Valid() || Role("god").Valid() {
		t.Fatal("empty/unknown roles must not be valid")
	}
}
