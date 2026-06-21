package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/angrosist/demo/internal/auth"
	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

// uniqueEmail makes a per-run email so repeated test runs don't collide on the
// users.email UNIQUE constraint.
func uniqueEmail(prefix string) string {
	return fmt.Sprintf("%s+%d@example.com", prefix, time.Now().UnixNano())
}

func TestUserRepo_CreateGetList(t *testing.T) {
	requireDB(t)
	repo := NewUserRepo()
	ctx := context.Background()

	email := uniqueEmail("staff")
	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatal(err)
	}
	u := &domain.User{Email: email, Name: "Test Staff", Role: domain.RoleStaff, PasswordHash: hash}
	if err := repo.Create(ctx, u); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if u.ID == "" || u.CreatedAt.IsZero() {
		t.Fatalf("Create did not populate id/created_at: %+v", u)
	}

	got, err := repo.GetByEmail(ctx, email)
	if err != nil {
		t.Fatalf("GetByEmail: %v", err)
	}
	if got.ID != u.ID || got.Role != domain.RoleStaff || got.PasswordHash != hash {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if !got.Active {
		t.Fatal("expected user to be active by default")
	}

	byID, err := repo.GetByID(ctx, u.ID)
	if err != nil || byID.Email != email {
		t.Fatalf("GetByID: %v %+v", err, byID)
	}

	users, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := false
	for _, x := range users {
		if x.ID == u.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("created user not present in List")
	}
}

func TestUserRepo_GetByEmail_NotFound(t *testing.T) {
	requireDB(t)
	repo := NewUserRepo()
	_, err := repo.GetByEmail(context.Background(), uniqueEmail("nobody"))
	if !errors.Is(err, ports.ErrUserNotFound) {
		t.Fatalf("err = %v, want ErrUserNotFound", err)
	}
}

func TestUserRepo_UpsertByEmail_Idempotent(t *testing.T) {
	requireDB(t)
	repo := NewUserRepo()
	ctx := context.Background()

	email := uniqueEmail("admin")
	hash, _ := auth.HashPassword("adminpass1")
	a := &domain.User{Email: email, Name: "Administrator", Role: domain.RoleAdmin, PasswordHash: hash}
	if err := repo.UpsertByEmail(ctx, a); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	firstID := a.ID

	// Second upsert with a new password should update in place, not duplicate.
	hash2, _ := auth.HashPassword("adminpass2")
	b := &domain.User{Email: email, Name: "Administrator", Role: domain.RoleAdmin, PasswordHash: hash2}
	if err := repo.UpsertByEmail(ctx, b); err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if b.ID != firstID {
		t.Fatalf("upsert created a new row: %s != %s", b.ID, firstID)
	}
	got, _ := repo.GetByEmail(ctx, email)
	if got.PasswordHash != hash2 {
		t.Fatal("upsert did not update the password hash")
	}
}

// TestBootstrapAdmin_LoginRoundtrip exercises the full bootstrap path: upsert an
// admin (as the container does), then verify the stored hash authenticates and
// the user resolves by the JWT subject.
func TestBootstrapAdmin_LoginRoundtrip(t *testing.T) {
	requireDB(t)
	repo := NewUserRepo()
	ctx := context.Background()

	email := uniqueEmail("bootstrap")
	const password = "bootstrap-pass-1"
	hash, _ := auth.HashPassword(password)
	admin := &domain.User{Email: email, Name: "Administrator", Role: domain.RoleAdmin, PasswordHash: hash}
	if err := repo.UpsertByEmail(ctx, admin); err != nil {
		t.Fatalf("bootstrap upsert: %v", err)
	}

	got, err := repo.GetByEmail(ctx, email)
	if err != nil {
		t.Fatalf("GetByEmail: %v", err)
	}
	if got.Role != domain.RoleAdmin {
		t.Fatalf("bootstrapped user role = %q, want admin", got.Role)
	}
	if !auth.VerifyPassword(got.PasswordHash, password) {
		t.Fatal("bootstrapped admin password does not verify")
	}

	tokens, _ := auth.NewTokenIssuer("itest-secret")
	tok, _ := tokens.Issue(got)
	claims, err := tokens.Verify(tok)
	if err != nil {
		t.Fatalf("verify token: %v", err)
	}
	resolved, err := repo.GetByID(ctx, claims.Subject)
	if err != nil || resolved.Email != email {
		t.Fatalf("token subject did not resolve to the admin: %v %+v", err, resolved)
	}
}
