package usecases

import (
	"context"

	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

type LeadUseCase struct {
	leadRepo ports.LeadRepo
}

func NewLeadUseCase(leadRepo ports.LeadRepo) *LeadUseCase {
	return &LeadUseCase{leadRepo: leadRepo}
}

func (uc *LeadUseCase) List(ctx context.Context) ([]*domain.LeadSummary, error) {
	return uc.leadRepo.List(ctx)
}

func (uc *LeadUseCase) GetByID(ctx context.Context, id string) (*domain.LeadDetail, error) {
	return uc.leadRepo.GetByID(ctx, id)
}
