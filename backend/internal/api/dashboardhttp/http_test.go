package dashboardhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
	"github.com/angrosist/demo/internal/usecases"
)

// --- stub repos ------------------------------------------------------------

type stubLeadRepo struct {
	page        []*domain.LeadSummary
	detail      *domain.LeadDetail
	detailErr   error
	handoffs    []*domain.HandoffItem
	validStatus map[string]bool
	kpis        *domain.KPIs
	updErr      error
	updSummary  *domain.LeadSummary
	gotFilter   domain.LeadFilter
}

func (s *stubLeadRepo) Create(context.Context, *domain.Lead) error { return nil }
func (s *stubLeadRepo) GetByConversationID(context.Context, string) (*domain.Lead, error) {
	return nil, nil
}
func (s *stubLeadRepo) UpdateCompanyContact(context.Context, string, string, string) error {
	return nil
}
func (s *stubLeadRepo) SetHumanFlagsByConversation(context.Context, string, bool, bool) error {
	return nil
}
func (s *stubLeadRepo) List(context.Context) ([]*domain.LeadSummary, error) { return nil, nil }
func (s *stubLeadRepo) GetByID(context.Context, string) (*domain.LeadDetail, error) {
	return s.detail, s.detailErr
}
func (s *stubLeadRepo) ListPage(_ context.Context, f domain.LeadFilter) ([]*domain.LeadSummary, error) {
	s.gotFilter = f
	return s.page, nil
}
func (s *stubLeadRepo) Handoffs(context.Context, domain.LeadFilter) ([]*domain.HandoffItem, error) {
	return s.handoffs, nil
}
func (s *stubLeadRepo) UpdateOffer(context.Context, string, domain.OfferUpdate) (*domain.LeadSummary, error) {
	return s.updSummary, s.updErr
}
func (s *stubLeadRepo) Assign(context.Context, string, *string) (*domain.LeadSummary, error) {
	return s.updSummary, s.updErr
}
func (s *stubLeadRepo) StatusExists(_ context.Context, status string) (bool, error) {
	return s.validStatus[status], nil
}
func (s *stubLeadRepo) KPIs(context.Context) (*domain.KPIs, error) { return s.kpis, nil }

type stubUserRepo struct{}

func (stubUserRepo) GetByID(context.Context, string) (*domain.User, error) {
	return &domain.User{ID: "u1"}, nil
}
func (stubUserRepo) GetByEmail(context.Context, string) (*domain.User, error) {
	return nil, ports.ErrUserNotFound
}
func (stubUserRepo) Create(context.Context, *domain.User) error        { return nil }
func (stubUserRepo) List(context.Context) ([]*domain.User, error)      { return nil, nil }
func (stubUserRepo) UpsertByEmail(context.Context, *domain.User) error { return nil }

type stubActivityRepo struct{}

func (stubActivityRepo) Append(context.Context, domain.ActivityLog) error { return nil }

type stubCompanyRepo struct {
	page      []*domain.CompanySummary
	detail    *domain.CompanyDetail
	detailErr error
	gotFilter domain.CompanyFilter
}

func (s *stubCompanyRepo) GetByCUI(context.Context, string) (*domain.Company, error) { return nil, nil }
func (s *stubCompanyRepo) GetByRegNo(context.Context, string, string) (*domain.Company, error) {
	return nil, nil
}
func (s *stubCompanyRepo) Upsert(context.Context, *domain.Company) error { return nil }
func (s *stubCompanyRepo) ListPage(_ context.Context, f domain.CompanyFilter) ([]*domain.CompanySummary, error) {
	s.gotFilter = f
	return s.page, nil
}
func (s *stubCompanyRepo) Detail(context.Context, string) (*domain.CompanyDetail, error) {
	return s.detail, s.detailErr
}

func newService(lr *stubLeadRepo, cr *stubCompanyRepo) *Service {
	leads := usecases.NewLeadUseCase(lr, stubUserRepo{}, stubActivityRepo{})
	companies := usecases.NewCompanyUseCase(cr)
	return NewService(leads, companies)
}

func summary(id string, created time.Time) *domain.LeadSummary {
	return &domain.LeadSummary{ID: id, Status: "new", CreatedAt: created}
}

// --- tests -----------------------------------------------------------------

func TestListLeads_PaginationCursor(t *testing.T) {
	now := time.Now().UTC()
	lr := &stubLeadRepo{page: []*domain.LeadSummary{
		summary("a", now), summary("b", now.Add(-time.Minute)), summary("c", now.Add(-2*time.Minute)),
	}}
	svc := newService(lr, &stubCompanyRepo{})

	rr := httptest.NewRecorder()
	svc.ListLeads(rr, httptest.NewRequest(http.MethodGet, "/api/leads?limit=2", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data []domain.LeadSummary `json:"data"`
		Page page                 `json:"page"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("want 2 items (limit), got %d", len(resp.Data))
	}
	if resp.Page.NextCursor == nil {
		t.Fatal("expected a next_cursor when more rows exist")
	}
	if resp.Page.Limit != 2 || resp.Page.Count != 2 {
		t.Fatalf("page meta wrong: %+v", resp.Page)
	}
}

func TestListLeads_FiltersPassThrough(t *testing.T) {
	lr := &stubLeadRepo{}
	svc := newService(lr, &stubCompanyRepo{})
	rr := httptest.NewRecorder()
	svc.ListLeads(rr, httptest.NewRequest(http.MethodGet,
		"/api/leads?status=qualified&vertical=angrosist&assigned_to=u9&q=cement", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	f := lr.gotFilter
	if f.Status != "qualified" || f.Vertical != "angrosist" || f.Query != "cement" {
		t.Fatalf("filters not parsed: %+v", f)
	}
	if f.AssignedTo == nil || *f.AssignedTo != "u9" {
		t.Fatalf("assigned_to not parsed: %+v", f.AssignedTo)
	}
}

func TestListLeads_BadCursor(t *testing.T) {
	svc := newService(&stubLeadRepo{}, &stubCompanyRepo{})
	rr := httptest.NewRecorder()
	// "bm90anNvbg" is base64url("notjson") — decodes fine but is not a JSON cursor.
	svc.ListLeads(rr, httptest.NewRequest(http.MethodGet, "/api/leads?cursor=bm90anNvbg", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestGetLead_NotFound(t *testing.T) {
	svc := newService(&stubLeadRepo{detailErr: ports.ErrNotFound}, &stubCompanyRepo{})
	rr := httptest.NewRecorder()
	svc.GetLead(rr, httptest.NewRequest(http.MethodGet, "/api/leads/x", nil), "x")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

func TestGetLead_NilDetail(t *testing.T) {
	svc := newService(&stubLeadRepo{detail: nil, detailErr: nil}, &stubCompanyRepo{})
	rr := httptest.NewRecorder()
	svc.GetLead(rr, httptest.NewRequest(http.MethodGet, "/api/leads/x", nil), "x")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

func TestUpdateOffer_InvalidStatus(t *testing.T) {
	lr := &stubLeadRepo{
		detail:      &domain.LeadDetail{LeadSummary: domain.LeadSummary{ID: "l1"}},
		validStatus: map[string]bool{},
	}
	svc := newService(lr, &stubCompanyRepo{})
	body, _ := json.Marshal(map[string]any{"status": "bogus"})
	rr := httptest.NewRecorder()
	svc.UpdateOffer(rr, httptest.NewRequest(http.MethodPatch, "/api/leads/l1/offer", bytes.NewReader(body)), "l1")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
}

func TestUpdateOffer_NegativeValue(t *testing.T) {
	svc := newService(&stubLeadRepo{}, &stubCompanyRepo{})
	body, _ := json.Marshal(map[string]any{"value": -5})
	rr := httptest.NewRecorder()
	svc.UpdateOffer(rr, httptest.NewRequest(http.MethodPatch, "/api/leads/l1/offer", bytes.NewReader(body)), "l1")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestUpdateOffer_EmptyBody(t *testing.T) {
	svc := newService(&stubLeadRepo{}, &stubCompanyRepo{})
	body, _ := json.Marshal(map[string]any{})
	rr := httptest.NewRecorder()
	svc.UpdateOffer(rr, httptest.NewRequest(http.MethodPatch, "/api/leads/l1/offer", bytes.NewReader(body)), "l1")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for empty patch", rr.Code)
	}
}

func TestUpdateOffer_Valid(t *testing.T) {
	v := 99.0
	lr := &stubLeadRepo{
		detail:      &domain.LeadDetail{LeadSummary: domain.LeadSummary{ID: "l1", Status: "qualified"}},
		validStatus: map[string]bool{"offer_sent": true},
		updSummary:  &domain.LeadSummary{ID: "l1", Status: "offer_sent", OfferValue: &v},
	}
	svc := newService(lr, &stubCompanyRepo{})
	body, _ := json.Marshal(map[string]any{"status": "offer_sent", "value": 99})
	rr := httptest.NewRecorder()
	svc.UpdateOffer(rr, httptest.NewRequest(http.MethodPatch, "/api/leads/l1/offer", bytes.NewReader(body)), "l1")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
}

func TestListCompanies_RoleCountryFilters(t *testing.T) {
	cr := &stubCompanyRepo{page: []*domain.CompanySummary{{ID: "c1", Roles: []string{"distributor"}}}}
	svc := newService(&stubLeadRepo{}, cr)
	rr := httptest.NewRecorder()
	svc.ListCompanies(rr, httptest.NewRequest(http.MethodGet,
		"/api/companies?role=distributor&country=RO&q=acme", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if cr.gotFilter.Role != "distributor" || cr.gotFilter.Country != "RO" || cr.gotFilter.Query != "acme" {
		t.Fatalf("company filters not parsed: %+v", cr.gotFilter)
	}
}

func TestGetCompany_NotFound(t *testing.T) {
	svc := newService(&stubLeadRepo{}, &stubCompanyRepo{detailErr: ports.ErrNotFound})
	rr := httptest.NewRecorder()
	svc.GetCompany(rr, httptest.NewRequest(http.MethodGet, "/api/companies/x", nil), "x")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

func TestHandoffs_OnlyNeedsHuman(t *testing.T) {
	lr := &stubLeadRepo{handoffs: []*domain.HandoffItem{{ID: "h1", LastMessage: "help"}}}
	svc := newService(lr, &stubCompanyRepo{})
	rr := httptest.NewRecorder()
	svc.ListHandoffs(rr, httptest.NewRequest(http.MethodGet, "/api/handoffs", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var resp struct {
		Data []domain.HandoffItem `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Data) != 1 || resp.Data[0].ID != "h1" {
		t.Fatalf("unexpected handoffs: %+v", resp.Data)
	}
}

func TestKPIs_OK(t *testing.T) {
	lr := &stubLeadRepo{kpis: &domain.KPIs{OffersSent: 3, Won: 1, Qualified: 4, ConversionRate: 0.25, PipelineValue: 1000}}
	svc := newService(lr, &stubCompanyRepo{})
	rr := httptest.NewRecorder()
	svc.KPIs(rr, httptest.NewRequest(http.MethodGet, "/api/kpis", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var k domain.KPIs
	if err := json.Unmarshal(rr.Body.Bytes(), &k); err != nil {
		t.Fatal(err)
	}
	if k.OffersSent != 3 || k.ConversionRate != 0.25 {
		t.Fatalf("kpis mismatch: %+v", k)
	}
}
