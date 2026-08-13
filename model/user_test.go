package model

import "testing"

func TestUserRole_StringAndParseRoundTrip(t *testing.T) {
	roles := []UserRole{RoleUser, RoleAdmin, RoleSuperadmin}
	for _, r := range roles {
		got, err := ParseUserRole(r.String())
		if err != nil {
			t.Errorf("ParseUserRole(%q) returned unexpected error: %v", r.String(), err)
		}
		if got != r {
			t.Errorf("round-trip mismatch: %v -> %q -> %v", r, r.String(), got)
		}
	}
}

func TestUserRole_StringOutOfRange(t *testing.T) {
	if got := UserRole(999).String(); got != "user" {
		t.Errorf("String() for out-of-range value = %q, want %q", got, "user")
	}
}

func TestParseUserRole_Unrecognized(t *testing.T) {
	got, err := ParseUserRole("not-a-real-role")
	if err == nil {
		t.Error("expected an error for an unrecognized user role")
	}
	if got != RoleUser {
		t.Errorf("expected the safe default RoleUser, got %v", got)
	}
}

func TestParseUserRole_CaseInsensitiveAndTrimmed(t *testing.T) {
	got, err := ParseUserRole("  Admin  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != RoleAdmin {
		t.Errorf("expected RoleAdmin, got %v", got)
	}
}

func TestUserRole_AtLeast(t *testing.T) {
	tests := []struct {
		name    string
		role    UserRole
		minimum UserRole
		want    bool
	}{
		{"user meets user minimum", RoleUser, RoleUser, true},
		{"user does not meet admin minimum", RoleUser, RoleAdmin, false},
		{"admin meets user minimum", RoleAdmin, RoleUser, true},
		{"admin meets admin minimum", RoleAdmin, RoleAdmin, true},
		{"admin does not meet superadmin minimum", RoleAdmin, RoleSuperadmin, false},
		{"superadmin meets every minimum", RoleSuperadmin, RoleAdmin, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.role.AtLeast(tt.minimum); got != tt.want {
				t.Errorf("%v.AtLeast(%v) = %v, want %v", tt.role, tt.minimum, got, tt.want)
			}
		})
	}
}
