package model

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// UserRole classifies a registered user's access level. Roles are ordered —
// use AtLeast to check whether a user meets a minimum required role rather
// than comparing for equality.
type UserRole int

const (
	// RoleUser is the default role every self-registered account gets.
	RoleUser UserRole = iota

	// RoleAdmin can manage the knowledge base and other operational data.
	RoleAdmin

	// RoleSuperadmin has full control, including user management. Only
	// created via the createsuperadmin CLI command, never through
	// self-registration.
	RoleSuperadmin
)

// userRoleNames is the canonical string form of each UserRole, used both for
// String() and ParseUserRole — the wire format a JWT/DB value communicates
// roles in.
var userRoleNames = map[UserRole]string{
	RoleUser:       "user",
	RoleAdmin:      "admin",
	RoleSuperadmin: "superadmin",
}

// String returns r's canonical lowercase name, or "user" for an
// out-of-range value — the safest fallback given it's the lowest privilege.
func (r UserRole) String() string {
	if name, ok := userRoleNames[r]; ok {
		return name
	}
	return "user"
}

// ParseUserRole parses s (case-insensitive) into a UserRole, matching the
// names String() produces. Returns an error for anything else.
func ParseUserRole(s string) (UserRole, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	for role, name := range userRoleNames {
		if name == s {
			return role, nil
		}
	}
	return RoleUser, fmt.Errorf("unrecognized user role %q", s)
}

// AtLeast returns true if this role is at or above the given minimum role.
func (r UserRole) AtLeast(minimum UserRole) bool {
	return r >= minimum
}

// User represents a registered user of the web application.
type User struct {
	ID           uuid.UUID
	Email        string
	Name         string
	PasswordHash string
	Role         UserRole
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
