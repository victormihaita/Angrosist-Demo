package usecases

import (
	"context"
	"errors"
	"testing"

	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

// --- mock ports ------------------------------------------------------------

type fakeLeadRepo struct {
	detail         *domain.LeadDetail
	detailErr      error
	updated        domain.OfferUpdate
	updatedSummary *domain.LeadSummary
	updateErr      error
	assignedUser   *string
	assignSummary  *domain.LeadSummary
	assignErr      error
	validStatus    map[string]bool
}

func (f *fakeLeadRepo) Create(context.Context, *domain.Lead) error { return nil }
func (f *fakeLeadRepo) GetByConversationID(context.Context, string) (*domain.Lead, error) {
	return nil, nil
}
func (f *fakeLeadRepo) UpdateCompanyContact(context.Context, string, string, string) error {
	return nil
}
func (f *fakeLeadRepo) List(context.Context) ([]*domain.LeadSummary, error) { return nil, nil }
func (f *fakeLeadRepo) GetByID(_ context.Context, _ string) (*domain.LeadDetail, error) {
	return f.detail, f.detailErr
}
func (f *fakeLeadRepo) ListPage(context.Context, domain.LeadFilter) ([]*domain.LeadSummary, error) {
	return nil, nil
}
func (f *fakeLeadRepo) Handoffs(context.Context, domain.LeadFilter) ([]*domain.HandoffItem, error) {
	return nil, nil
}
func (f *fakeLeadRepo) UpdateOffer(_ context.Context, _ string, upd domain.OfferUpdate) (*domain.LeadSummary, error) {
	f.updated = upd
	return f.updatedSummary, f.updateErr
}
func (f *fakeLeadRepo) Assign(_ context.Context, _ string, userID *string) (*domain.LeadSummary, error) {
	f.assignedUser = userID
	return f.assignSummary, f.assignErr
}
func (f *fakeLeadRepo) StatusExists(_ context.Context, status string) (bool, error) {
	return f.validStatus[status], nil
}
func (f *fakeLeadRepo) KPIs(context.Context) (*domain.KPIs, error) { return nil, nil }

type fakeUserRepo struct{ exists map[string]bool }

func (f *fakeUserRepo) GetByID(_ context.Context, id string) (*domain.User, error) {
	if f.exists[id] {
		return &domain.User{ID: id}, nil
	}
	return nil, ports.ErrUserNotFound
}
func (f *fakeUserRepo) GetByEmail(context.Context, string) (*domain.User, error) {
	return nil, ports.ErrUserNotFound
}
func (f *fakeUserRepo) Create(context.Context, *domain.User) error        { return nil }
func (f *fakeUserRepo) List(context.Context) ([]*domain.User, error)      { return nil, nil }
func (f *fakeUserRepo) UpsertByEmail(context.Context, *domain.User) error { return nil }

type fakeActivityRepo struct{ entries []domain.ActivityLog }

func (f *fakeActivityRepo) Append(_ context.Context, e domain.ActivityLog) error {
	f.entries = append(f.entries, e)
	return nil
}

// --- tests -----------------------------------------------------------------

func TestUpdateOffer_ValidPersistsAndAudits(t *testing.T) {
	v := 1200.0
	leadRepo := &fakeLeadRepo{
		detail:         &domain.LeadDetail{LeadSummary: domain.LeadSummary{ID: "l1", Status: "qualified"}},
		updatedSummary: &domain.LeadSummary{ID: "l1", Status: "offer_sent", OfferValue: &v},
		validStatus:    map[string]bool{"offer_sent": true},
	}
	act := &fakeActivityRepo{}
	uc := NewLeadUseCase(leadRepo, &fakeUserRepo{}, act)

	status := "offer_sent"
	out, err := uc.UpdateOffer(context.Background(), "u1", "l1", domain.OfferUpdate{Status: &status, Value: &v})
	if err != nil {
		t.Fatalf("UpdateOffer: %v", err)
	}
	if out.Status != "offer_sent" {
		t.Fatalf("status not applied: %+v", out)
	}
	if len(act.entries) != 1 || act.entries[0].Action != "offer.update" || act.entries[0].ActorID != "u1" {
		t.Fatalf("expected one offer.update audit row, got %+v", act.entries)
	}
}

func TestUpdateOffer_InvalidStatus(t *testing.T) {
	leadRepo := &fakeLeadRepo{validStatus: map[string]bool{}}
	uc := NewLeadUseCase(leadRepo, &fakeUserRepo{}, &fakeActivityRepo{})
	bad := "nope"
	_, err := uc.UpdateOffer(context.Background(), "u1", "l1", domain.OfferUpdate{Status: &bad})
	if !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("err = %v, want ErrInvalidStatus", err)
	}
}

func TestUpdateOffer_NotFound(t *testing.T) {
	leadRepo := &fakeLeadRepo{detailErr: ports.ErrNotFound, validStatus: map[string]bool{"won": true}}
	uc := NewLeadUseCase(leadRepo, &fakeUserRepo{}, &fakeActivityRepo{})
	st := "won"
	_, err := uc.UpdateOffer(context.Background(), "u1", "missing", domain.OfferUpdate{Status: &st})
	if !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestAssign_KnownUserAuditsAndAssigns(t *testing.T) {
	leadRepo := &fakeLeadRepo{assignSummary: &domain.LeadSummary{ID: "l1"}}
	act := &fakeActivityRepo{}
	uc := NewLeadUseCase(leadRepo, &fakeUserRepo{exists: map[string]bool{"u2": true}}, act)

	target := "u2"
	if _, err := uc.Assign(context.Background(), "u1", "l1", &target); err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if leadRepo.assignedUser == nil || *leadRepo.assignedUser != "u2" {
		t.Fatalf("assigned user not passed through: %v", leadRepo.assignedUser)
	}
	if len(act.entries) != 1 || act.entries[0].Action != "lead.assign" {
		t.Fatalf("expected lead.assign audit, got %+v", act.entries)
	}
}

func TestAssign_UnknownUser(t *testing.T) {
	leadRepo := &fakeLeadRepo{}
	uc := NewLeadUseCase(leadRepo, &fakeUserRepo{exists: map[string]bool{}}, &fakeActivityRepo{})
	target := "ghost"
	_, err := uc.Assign(context.Background(), "u1", "l1", &target)
	if !errors.Is(err, ErrUnknownUser) {
		t.Fatalf("err = %v, want ErrUnknownUser", err)
	}
}

func TestAssign_UnassignWithNil(t *testing.T) {
	leadRepo := &fakeLeadRepo{assignSummary: &domain.LeadSummary{ID: "l1"}}
	act := &fakeActivityRepo{}
	uc := NewLeadUseCase(leadRepo, &fakeUserRepo{}, act)
	if _, err := uc.Assign(context.Background(), "u1", "l1", nil); err != nil {
		t.Fatalf("Assign(nil): %v", err)
	}
	if leadRepo.assignedUser != nil {
		t.Fatalf("expected nil (unassign), got %v", *leadRepo.assignedUser)
	}
	if len(act.entries) != 1 {
		t.Fatalf("expected one audit row, got %d", len(act.entries))
	}
}
