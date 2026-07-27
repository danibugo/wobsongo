package layout

import (
	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/model"
)

// CurrentUser holds the authenticated user's identity extracted from JWT claims.
type CurrentUser struct {
	ID   uuid.UUID
	Name string
	Role model.UserRole
}

// IsAdmin reports whether the current user is at least an admin.
func (u CurrentUser) IsAdmin() bool { return u.Role.AtLeast(model.RoleAdmin) }

// IsSuperadmin reports whether the current user is a superadmin.
func (u CurrentUser) IsSuperadmin() bool { return u.Role.AtLeast(model.RoleSuperadmin) }

// Crumb is a single breadcrumb entry.
// Href should be empty for the current (last) page — rendered as plain text.
type Crumb struct {
	Label string
	Href  string // empty = current page (non-linked)
}

// AppLayoutData is passed to every authenticated page layout.
type AppLayoutData struct {
	User             CurrentUser
	CurrentPath      string // used by the sidebar for active-state highlighting
	PageTitle        string
	Subtitle         string // optional topbar subtitle
	Breadcrumbs      []Crumb
	CSRFToken        string // rendered as a hidden field in every POST form
	SidebarCollapsed bool   // initial server-side sidebar state (false = open)
}
