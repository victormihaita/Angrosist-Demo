package usecases

import (
	"context"
	"errors"
	"fmt"

	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

// ErrInvalidStatus is returned by UpdateOffer when the requested status is not a
// valid lead_statuses code. The handler maps it to a 400/422 envelope.
var ErrInvalidStatus = errors.New("invalid lead status")

// ErrUnknownUser is returned by Assign when the target user id does not resolve.
// The handler maps it to a 400 envelope.
var ErrUnknownUser = errors.New("unknown user")

// LeadUseCase orchestrates the dashboard lead operations over the lead repository,
// the user repository (assignment validation) and the audit log. It owns no
// transport or SQL — those live in the adapters.
type LeadUseCase struct {
	leadRepo     ports.LeadRepo
	users        ports.UserRepo
	activityRepo ports.ActivityLogRepo
}

// NewLeadUseCase wires the lead use-case. users and activityRepo may be nil for
// the legacy demo wiring (List/GetByID only); the dashboard wiring supplies all.
func NewLeadUseCase(leadRepo ports.LeadRepo, users ports.UserRepo, activityRepo ports.ActivityLogRepo) *LeadUseCase {
	return &LeadUseCase{leadRepo: leadRepo, users: users, activityRepo: activityRepo}
}

// List returns the unbounded legacy demo list (used by the Vercel demo handlers).
func (uc *LeadUseCase) List(ctx context.Context) ([]*domain.LeadSummary, error) {
	return uc.leadRepo.List(ctx)
}

// ListPage returns one keyset page of leads matching the filter.
func (uc *LeadUseCase) ListPage(ctx context.Context, f domain.LeadFilter) ([]*domain.LeadSummary, error) {
	return uc.leadRepo.ListPage(ctx, f)
}

// Handoffs returns one keyset page of the human-handoff queue.
func (uc *LeadUseCase) Handoffs(ctx context.Context, f domain.LeadFilter) ([]*domain.HandoffItem, error) {
	return uc.leadRepo.Handoffs(ctx, f)
}

// GetByID returns the lead detail or ports.ErrNotFound.
func (uc *LeadUseCase) GetByID(ctx context.Context, id string) (*domain.LeadDetail, error) {
	return uc.leadRepo.GetByID(ctx, id)
}

// KPIs returns the dashboard aggregate KPIs.
func (uc *LeadUseCase) KPIs(ctx context.Context) (*domain.KPIs, error) {
	return uc.leadRepo.KPIs(ctx)
}

// UpdateOffer applies a manual offer change and audits it. The status (when
// supplied) is validated against the lead_statuses lookup; on success it writes an
// 'offer.update' activity_logs row with the old→new diff. Value/note bounds are
// the handler's responsibility; ErrInvalidStatus / ports.ErrNotFound surface here.
func (uc *LeadUseCase) UpdateOffer(ctx context.Context, actorID, leadID string, upd domain.OfferUpdate) (*domain.LeadSummary, error) {
	if upd.Status != nil {
		ok, err := uc.leadRepo.StatusExists(ctx, *upd.Status)
		if err != nil {
			return nil, fmt.Errorf("validate status: %w", err)
		}
		if !ok {
			return nil, ErrInvalidStatus
		}
	}

	before, err := uc.leadRepo.GetByID(ctx, leadID)
	if err != nil {
		return nil, err // includes ports.ErrNotFound
	}

	after, err := uc.leadRepo.UpdateOffer(ctx, leadID, upd)
	if err != nil {
		return nil, err
	}

	if uc.activityRepo != nil {
		meta := map[string]any{
			"old": map[string]any{
				"status":      before.Status,
				"offer_value": before.OfferValue,
				"offer_note":  before.OfferNote,
			},
			"new": map[string]any{
				"status":      after.Status,
				"offer_value": after.OfferValue,
				"offer_note":  after.OfferNote,
			},
		}
		_ = uc.activityRepo.Append(ctx, domain.ActivityLog{
			ActorType:  "staff",
			ActorID:    actorID,
			Action:     "offer.update",
			EntityType: "lead",
			EntityID:   leadID,
			Meta:       meta,
		})
	}
	return after, nil
}

// Assign sets (or clears with nil) a lead's owner and audits it. The target user
// must exist (when non-nil); an unknown user yields ErrUnknownUser. On success it
// writes a 'lead.assign' activity_logs row.
func (uc *LeadUseCase) Assign(ctx context.Context, actorID, leadID string, userID *string) (*domain.LeadSummary, error) {
	if userID != nil {
		u, err := uc.users.GetByID(ctx, *userID)
		if err != nil || u == nil {
			return nil, ErrUnknownUser
		}
	}

	after, err := uc.leadRepo.Assign(ctx, leadID, userID)
	if err != nil {
		return nil, err
	}

	if uc.activityRepo != nil {
		var assigned any
		if userID != nil {
			assigned = *userID
		}
		_ = uc.activityRepo.Append(ctx, domain.ActivityLog{
			ActorType:  "staff",
			ActorID:    actorID,
			Action:     "lead.assign",
			EntityType: "lead",
			EntityID:   leadID,
			Meta:       map[string]any{"assigned_to": assigned},
		})
	}
	return after, nil
}

// CompanyUseCase orchestrates the B2B directory reads.
type CompanyUseCase struct {
	companyRepo ports.CompanyRepo
}

// NewCompanyUseCase wires the directory use-case.
func NewCompanyUseCase(companyRepo ports.CompanyRepo) *CompanyUseCase {
	return &CompanyUseCase{companyRepo: companyRepo}
}

// ListPage returns one keyset page of directory companies matching the filter.
func (uc *CompanyUseCase) ListPage(ctx context.Context, f domain.CompanyFilter) ([]*domain.CompanySummary, error) {
	return uc.companyRepo.ListPage(ctx, f)
}

// GetByID returns the company directory detail or ports.ErrNotFound.
func (uc *CompanyUseCase) GetByID(ctx context.Context, id string) (*domain.CompanyDetail, error) {
	return uc.companyRepo.Detail(ctx, id)
}
