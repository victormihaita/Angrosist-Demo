package domain

import "time"

// Role enumerates the dashboard roles enforced by RBAC. The set is extensible
// (the DB stores it as TEXT, not a PG enum); P2 will add provider/admin_global/
// country_operator. Phase 1 uses staff and admin.
type Role string

const (
	// RoleStaff can read/work leads, companies, the handoff queue, and edit
	// offers/assignment.
	RoleStaff Role = "staff"
	// RoleAdmin is staff plus user management.
	RoleAdmin Role = "admin"
)

// User is a dashboard operator (staff or admin). PasswordHash is the bcrypt hash
// of the user's password; it is never serialized to API clients (json:"-").
type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	Role         Role      `json:"role"`
	Active       bool      `json:"active"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// IsAdmin reports whether the user holds the admin role.
func (u *User) IsAdmin() bool { return u.Role == RoleAdmin }

// PublicUser is the safe projection returned in API responses (no hash).
type PublicUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  Role   `json:"role"`
}

// Public returns the API-safe view of the user.
func (u *User) Public() PublicUser {
	return PublicUser{ID: u.ID, Email: u.Email, Name: u.Name, Role: u.Role}
}
