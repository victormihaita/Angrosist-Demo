package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

// UserRepo is the Postgres adapter for ports.UserRepo. It maps the `users` table
// (migration 011 + 024): id, email, name (nullable), role, active, password_hash
// (nullable bcrypt), created_at, updated_at. All statements are parameterized.
type UserRepo struct{}

// NewUserRepo constructs the Postgres-backed user repository.
func NewUserRepo() *UserRepo { return &UserRepo{} }

// scanUser maps a row with the canonical column order into a domain.User. The
// nullable name and password_hash columns are coalesced to empty strings.
func scanUser(row pgx.Row) (*domain.User, error) {
	var u domain.User
	err := row.Scan(
		&u.ID,
		&u.Email,
		&u.Name,
		&u.Role,
		&u.Active,
		&u.PasswordHash,
		&u.CreatedAt,
		&u.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

const userColumns = `
	id::text,
	email,
	COALESCE(name, ''),
	role,
	active,
	COALESCE(password_hash, ''),
	created_at,
	updated_at`

// GetByID returns the user with the given id or ports.ErrUserNotFound.
func (r *UserRepo) GetByID(ctx context.Context, id string) (*domain.User, error) {
	row := GetPool().QueryRow(ctx, `
		SELECT `+userColumns+`
		FROM users WHERE id = $1::uuid
	`, id)
	u, err := scanUser(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ports.ErrUserNotFound
		}
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return u, nil
}

// GetByEmail returns the user with the given email or ports.ErrUserNotFound.
func (r *UserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	row := GetPool().QueryRow(ctx, `
		SELECT `+userColumns+`
		FROM users WHERE email = $1
	`, email)
	u, err := scanUser(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ports.ErrUserNotFound
		}
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return u, nil
}

// Create inserts a new user and writes the assigned id/timestamps back onto u.
func (r *UserRepo) Create(ctx context.Context, u *domain.User) error {
	row := GetPool().QueryRow(ctx, `
		INSERT INTO users (email, name, role, password_hash)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text, active, created_at, updated_at
	`, u.Email, u.Name, string(u.Role), nullStr(u.PasswordHash))
	if err := row.Scan(&u.ID, &u.Active, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

// List returns all users newest-first.
func (r *UserRepo) List(ctx context.Context) ([]*domain.User, error) {
	rows, err := GetPool().Query(ctx, `
		SELECT `+userColumns+`
		FROM users ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []*domain.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// UpsertByEmail inserts the user or, on email conflict, updates name, role and
// password hash. It backs idempotent admin bootstrap; the assigned id/timestamps
// are written back onto u.
func (r *UserRepo) UpsertByEmail(ctx context.Context, u *domain.User) error {
	row := GetPool().QueryRow(ctx, `
		INSERT INTO users (email, name, role, password_hash)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (email) DO UPDATE
		SET name = EXCLUDED.name,
		    role = EXCLUDED.role,
		    password_hash = EXCLUDED.password_hash,
		    updated_at = now()
		RETURNING id::text, active, created_at, updated_at
	`, u.Email, u.Name, string(u.Role), nullStr(u.PasswordHash))
	if err := row.Scan(&u.ID, &u.Active, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return fmt.Errorf("upsert user by email: %w", err)
	}
	return nil
}

// Ensure UserRepo satisfies the port.
var _ ports.UserRepo = (*UserRepo)(nil)
