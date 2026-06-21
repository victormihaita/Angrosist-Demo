package postgres

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/angrosist/demo/internal/auth"
	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

// seedFixture holds the ids of a minimal, self-contained dashboard dataset so the
// tests can assert against exactly the rows they created (filters narrow within
// the seed; counts use the seed ids, never the whole table).
type seedFixture struct {
	staffID      string
	companyAID   string // RO distributor/wholesaler, verified
	companyBID   string // BG importer
	contactID    string
	leadNew      string // status=new, vertical=angrosist, unassigned
	leadQualAsgn string // status=qualified, assigned to staff
	leadOffer    string // status=offer_sent, offer_value set
	leadWon      string // status=won
	leadHandoff  string // needs_human=true
	leadPallet   string // vertical=palletclearance
	convHandoff  string
}

// seedDashboard inserts a couple companies (+verification), a contact, leads
// across statuses/verticals, a sourcing_request, messages, and a needs_human
// lead. It returns the fixture ids. Cleanup is by ON DELETE CASCADE when the
// owning rows are removed at the end of the test.
func seedDashboard(t *testing.T) *seedFixture {
	t.Helper()
	ctx := context.Background()
	pool := GetPool()
	f := &seedFixture{}

	hash, _ := auth.HashPassword("password123")
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (email, name, role, password_hash)
		VALUES ($1, 'Dash Staff', 'staff', $2) RETURNING id::text
	`, uniqueEmail("dashstaff"), hash).Scan(&f.staffID); err != nil {
		t.Fatalf("seed staff: %v", err)
	}

	regA := "70000001"
	if err := pool.QueryRow(ctx, `
		INSERT INTO companies (cui, country, reg_no, name, address, county, is_active, vat_status, caen, roles)
		VALUES ($1, 'RO', $1, 'Acme Distributie Seed', 'Str. Seed 1', 'Cluj', true, 'active', '4690',
		        ARRAY['distributor','wholesaler','buyer']::text[])
		RETURNING id::text
	`, regA).Scan(&f.companyAID); err != nil {
		t.Fatalf("seed company A: %v", err)
	}
	admins, _ := json.Marshal([]map[string]string{{"name": "Ion Popescu", "role": "administrator"}})
	if _, err := pool.Exec(ctx, `
		INSERT INTO company_verifications (company_id, source, vat_status, administrators, checked_at)
		VALUES ($1::uuid, 'demoanaf', 'active', $2, now())
	`, f.companyAID, admins); err != nil {
		t.Fatalf("seed verification: %v", err)
	}

	regB := "70000002"
	if err := pool.QueryRow(ctx, `
		INSERT INTO companies (cui, country, reg_no, name, is_active, roles)
		VALUES ($1, 'BG', $1, 'Sofia Import Seed', true, ARRAY['importer']::text[])
		RETURNING id::text
	`, regB).Scan(&f.companyBID); err != nil {
		t.Fatalf("seed company B: %v", err)
	}

	if err := pool.QueryRow(ctx, `
		INSERT INTO contacts (company_id, name, phone, email)
		VALUES ($1::uuid, 'Seed Buyer', '+40700000000', 'buyer@seed.test')
		RETURNING id::text
	`, f.companyAID).Scan(&f.contactID); err != nil {
		t.Fatalf("seed contact: %v", err)
	}

	// Each lead needs a conversation (conversation_id NOT NULL on leads).
	mkLead := func(status, vertical, companyID, contactID string, assigned *string, needsHuman bool, offer *float64) (leadID, convID string) {
		t.Helper()
		if err := pool.QueryRow(ctx, `
			INSERT INTO conversations (channel, state, extracted)
			VALUES ('test', 'qualifying', '{}') RETURNING id::text
		`).Scan(&convID); err != nil {
			t.Fatalf("seed conversation: %v", err)
		}
		var comp, cont, asg any
		if companyID != "" {
			comp = companyID
		}
		if contactID != "" {
			cont = contactID
		}
		if assigned != nil {
			asg = *assigned
		}
		if err := pool.QueryRow(ctx, `
			INSERT INTO leads (conversation_id, company_id, contact_id, status, vertical, intent, assigned_to, needs_human, offer_value)
			VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, 'buy', $6::uuid, $7, $8)
			RETURNING id::text
		`, convID, comp, cont, status, vertical, asg, needsHuman, offer).Scan(&leadID); err != nil {
			t.Fatalf("seed lead (%s): %v", status, err)
		}
		return leadID, convID
	}

	offer := 2500.0
	f.leadNew, _ = mkLead("new", "angrosist", f.companyAID, f.contactID, nil, false, nil)
	f.leadQualAsgn, _ = mkLead("qualified", "angrosist", f.companyAID, f.contactID, &f.staffID, false, nil)
	f.leadOffer, _ = mkLead("offer_sent", "angrosist", f.companyAID, f.contactID, nil, false, &offer)
	f.leadWon, _ = mkLead("won", "angrosist", f.companyAID, f.contactID, nil, false, &offer)
	f.leadPallet, _ = mkLead("new", "palletclearance", f.companyBID, "", nil, false, nil)
	f.leadHandoff, f.convHandoff = mkLead("needs_human", "angrosist", f.companyAID, f.contactID, nil, true, nil)

	// A sourcing_request on the qualified lead + a transcript on the handoff lead.
	if _, err := pool.Exec(ctx, `
		INSERT INTO sourcing_requests (lead_id, product_name, product, quantity, unit, delivery_location, recurring)
		VALUES ($1::uuid, 'Ciment Portland', 'Ciment Portland', 40, 'tone', 'Cluj-Napoca', true)
	`, f.leadQualAsgn); err != nil {
		t.Fatalf("seed sourcing_request: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO messages (conversation_id, role, content) VALUES
			($1::uuid, 'user', 'Vreau o oferta'),
			($1::uuid, 'assistant', 'Va rog asteptati un operator uman pentru detalii.')
	`, f.convHandoff); err != nil {
		t.Fatalf("seed messages: %v", err)
	}

	t.Cleanup(func() { cleanupFixture(t, f) })
	return f
}

func cleanupFixture(t *testing.T, f *seedFixture) {
	ctx := context.Background()
	pool := GetPool()
	// Deleting conversations cascades to leads (conversation_id CASCADE) and their
	// sourcing_requests/messages; companies/contacts/users removed explicitly.
	for _, id := range []string{f.leadNew, f.leadQualAsgn, f.leadOffer, f.leadWon, f.leadPallet, f.leadHandoff} {
		_, _ = pool.Exec(ctx, `DELETE FROM leads WHERE id = $1::uuid`, id)
	}
	_, _ = pool.Exec(ctx, `DELETE FROM contacts WHERE id = $1::uuid`, f.contactID)
	_, _ = pool.Exec(ctx, `DELETE FROM companies WHERE id IN ($1::uuid, $2::uuid)`, f.companyAID, f.companyBID)
	_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1::uuid`, f.staffID)
}

func idsOf(items []*domain.LeadSummary) map[string]*domain.LeadSummary {
	m := map[string]*domain.LeadSummary{}
	for _, it := range items {
		m[it.ID] = it
	}
	return m
}

func TestLeadRepo_ListPage_FiltersAndOrder(t *testing.T) {
	requireDB(t)
	f := seedDashboard(t)
	repo := NewLeadRepo()
	ctx := context.Background()

	// Unfiltered page (large limit) contains all six seed leads, newest first.
	all, err := repo.ListPage(ctx, domain.LeadFilter{Limit: 100})
	if err != nil {
		t.Fatalf("ListPage: %v", err)
	}
	got := idsOf(all)
	for _, id := range []string{f.leadNew, f.leadQualAsgn, f.leadOffer, f.leadWon, f.leadPallet, f.leadHandoff} {
		if got[id] == nil {
			t.Fatalf("seed lead %s missing from list", id)
		}
	}
	// Order: created_at DESC. The handoff lead was inserted last, so it must come
	// before the new lead among our seed rows.
	posHandoff, posNew := -1, -1
	for i, it := range all {
		if it.ID == f.leadHandoff {
			posHandoff = i
		}
		if it.ID == f.leadNew {
			posNew = i
		}
	}
	if posHandoff < 0 || posNew < 0 || posHandoff > posNew {
		t.Fatalf("ordering not desc: handoff@%d new@%d", posHandoff, posNew)
	}

	// status filter.
	q, _ := repo.ListPage(ctx, domain.LeadFilter{Status: "won", Limit: 100})
	if m := idsOf(q); m[f.leadWon] == nil {
		t.Fatal("status=won did not include the won lead")
	} else if m[f.leadNew] != nil {
		t.Fatal("status=won leaked a non-won lead")
	}

	// vertical filter.
	pc, _ := repo.ListPage(ctx, domain.LeadFilter{Vertical: "palletclearance", Limit: 100})
	if m := idsOf(pc); m[f.leadPallet] == nil || m[f.leadNew] != nil {
		t.Fatalf("vertical filter wrong: %v", idsList(pc))
	}

	// assigned_to filter.
	asg, _ := repo.ListPage(ctx, domain.LeadFilter{AssignedTo: &f.staffID, Limit: 100})
	if m := idsOf(asg); m[f.leadQualAsgn] == nil || m[f.leadNew] != nil {
		t.Fatalf("assigned_to filter wrong: %v", idsList(asg))
	}

	// q filter (ILIKE on product) — only the qualified lead has a sourcing_request.
	pq, _ := repo.ListPage(ctx, domain.LeadFilter{Query: "ciment", Limit: 100})
	if m := idsOf(pq); m[f.leadQualAsgn] == nil {
		t.Fatalf("q filter missed the matching lead: %v", idsList(pq))
	}
}

func idsList(items []*domain.LeadSummary) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.ID)
	}
	return out
}

func TestLeadRepo_ListPage_KeysetStable(t *testing.T) {
	requireDB(t)
	f := seedDashboard(t)
	repo := NewLeadRepo()
	ctx := context.Background()
	_ = f

	// Page through the angrosist vertical two at a time and assert no overlap and
	// full coverage of the five angrosist seed leads.
	seen := map[string]bool{}
	var cursor *domain.Cursor
	for i := 0; i < 10; i++ {
		page, err := repo.ListPage(ctx, domain.LeadFilter{Vertical: "angrosist", Limit: 2, After: cursor})
		if err != nil {
			t.Fatalf("page %d: %v", i, err)
		}
		if len(page) == 0 {
			break
		}
		hasNext := len(page) > 2
		if hasNext {
			page = page[:2]
		}
		for _, it := range page {
			if seen[it.ID] {
				t.Fatalf("keyset returned duplicate %s", it.ID)
			}
			seen[it.ID] = true
		}
		if !hasNext {
			break
		}
		last := page[len(page)-1]
		cursor = &domain.Cursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	for _, id := range []string{f.leadNew, f.leadQualAsgn, f.leadOffer, f.leadWon, f.leadHandoff} {
		if !seen[id] {
			t.Fatalf("keyset paging missed angrosist lead %s", id)
		}
	}
}

func TestLeadRepo_GetByID_Facets(t *testing.T) {
	requireDB(t)
	f := seedDashboard(t)
	repo := NewLeadRepo()
	ctx := context.Background()

	d, err := repo.GetByID(ctx, f.leadQualAsgn)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if d.Status != "qualified" || d.Vertical != "angrosist" {
		t.Fatalf("pipeline fields wrong: %+v", d.LeadSummary)
	}
	if d.SourcingRequest == nil || d.SourcingRequest.Product != "Ciment Portland" || !d.SourcingRequest.Recurring {
		t.Fatalf("sourcing_request facet wrong: %+v", d.SourcingRequest)
	}
	if d.Company == nil || len(d.Company.Roles) == 0 {
		t.Fatalf("company facet/roles missing: %+v", d.Company)
	}
	if d.Company.Verification == nil || d.Company.Verification.VATStatus != "active" {
		t.Fatalf("company verification facet missing: %+v", d.Company)
	}
	if d.Contact == nil || d.Contact.Email != "buyer@seed.test" {
		t.Fatalf("contact facet missing: %+v", d.Contact)
	}

	// Transcript present on the handoff lead.
	dh, err := repo.GetByID(ctx, f.leadHandoff)
	if err != nil {
		t.Fatalf("GetByID handoff: %v", err)
	}
	if len(dh.Transcript) != 2 {
		t.Fatalf("expected 2 transcript messages, got %d", len(dh.Transcript))
	}

	// 404 on missing.
	if _, err := repo.GetByID(ctx, "00000000-0000-0000-0000-000000000000"); err == nil {
		t.Fatal("expected error for missing lead id")
	}
}

func TestLeadRepo_UpdateOfferAndActivity(t *testing.T) {
	requireDB(t)
	f := seedDashboard(t)
	repo := NewLeadRepo()
	act := NewActivityLogRepo()
	ctx := context.Background()

	ok, err := repo.StatusExists(ctx, "offer_sent")
	if err != nil || !ok {
		t.Fatalf("StatusExists(offer_sent) = %v,%v", ok, err)
	}
	if bad, _ := repo.StatusExists(ctx, "definitely_not_a_status"); bad {
		t.Fatal("StatusExists returned true for invalid status")
	}

	v := 3333.0
	note := "Quoted via phone"
	st := "offer_sent"
	got, err := repo.UpdateOffer(ctx, f.leadNew, domain.OfferUpdate{Status: &st, Value: &v, Note: &note})
	if err != nil {
		t.Fatalf("UpdateOffer: %v", err)
	}
	if got.Status != "offer_sent" || got.OfferValue == nil || *got.OfferValue != v || got.OfferNote != note {
		t.Fatalf("offer not persisted: %+v", got)
	}

	if err := act.Append(ctx, domain.ActivityLog{
		ActorType: "staff", ActorID: f.staffID, Action: "offer.update",
		EntityType: "lead", EntityID: f.leadNew,
		Meta: map[string]any{"new": map[string]any{"status": "offer_sent"}},
	}); err != nil {
		t.Fatalf("Append activity: %v", err)
	}
	var n int
	if err := GetPool().QueryRow(ctx,
		`SELECT count(*) FROM activity_logs WHERE entity_id = $1::uuid AND action = 'offer.update'`,
		f.leadNew).Scan(&n); err != nil {
		t.Fatalf("count activity: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 activity row, got %d", n)
	}

	// Missing lead -> ErrNotFound.
	if _, err := repo.UpdateOffer(ctx, "00000000-0000-0000-0000-000000000000",
		domain.OfferUpdate{Status: &st}); err != ports.ErrNotFound {
		t.Fatalf("UpdateOffer missing = %v, want ErrNotFound", err)
	}
}

func TestLeadRepo_Assign(t *testing.T) {
	requireDB(t)
	f := seedDashboard(t)
	repo := NewLeadRepo()
	ctx := context.Background()

	got, err := repo.Assign(ctx, f.leadNew, &f.staffID)
	if err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if got.AssignedTo == nil || *got.AssignedTo != f.staffID {
		t.Fatalf("assigned_to not set: %+v", got.AssignedTo)
	}

	un, err := repo.Assign(ctx, f.leadNew, nil)
	if err != nil {
		t.Fatalf("unassign: %v", err)
	}
	if un.AssignedTo != nil {
		t.Fatalf("expected unassigned, got %v", *un.AssignedTo)
	}

	if _, err := repo.Assign(ctx, "00000000-0000-0000-0000-000000000000", &f.staffID); err != ports.ErrNotFound {
		t.Fatalf("Assign missing = %v, want ErrNotFound", err)
	}
}

func TestLeadRepo_Handoffs(t *testing.T) {
	requireDB(t)
	f := seedDashboard(t)
	repo := NewLeadRepo()
	ctx := context.Background()

	items, err := repo.Handoffs(ctx, domain.LeadFilter{Limit: 100})
	if err != nil {
		t.Fatalf("Handoffs: %v", err)
	}
	var found *domain.HandoffItem
	for _, it := range items {
		if it.ID == f.leadHandoff {
			found = it
		}
		// Every returned item must be a needs_human lead — assert none of our
		// non-handoff seed leads leaked in.
		if it.ID == f.leadNew || it.ID == f.leadWon {
			t.Fatalf("handoff queue leaked non-handoff lead %s", it.ID)
		}
	}
	if found == nil {
		t.Fatal("handoff lead missing from queue")
	}
	if found.LastMessage == "" {
		t.Fatal("handoff item missing last-message snippet")
	}
}

func TestLeadRepo_KPIs(t *testing.T) {
	requireDB(t)
	f := seedDashboard(t)
	repo := NewLeadRepo()
	ctx := context.Background()
	_ = f

	k, err := repo.KPIs(ctx)
	if err != nil {
		t.Fatalf("KPIs: %v", err)
	}
	// The seed adds: offer_sent(+offer 2500), won(+offer 2500), qualified, plus
	// new/needs_human/pallet. Assert the seed's contribution is reflected (counts
	// are table-global, so assert lower bounds the seed guarantees).
	if k.OffersSent < 2 {
		t.Fatalf("offers_sent = %d, want >= 2 (offer_sent + won)", k.OffersSent)
	}
	if k.Won < 1 {
		t.Fatalf("won = %d, want >= 1", k.Won)
	}
	if k.Qualified < 3 {
		t.Fatalf("qualified = %d, want >= 3 (qualified+offer_sent+won)", k.Qualified)
	}
	// pipeline_value sums offer_value over open statuses: offer_sent(2500) counts,
	// won(2500) is terminal and excluded. So the seed contributes >= 2500.
	if k.PipelineValue < 2500 {
		t.Fatalf("pipeline_value = %v, want >= 2500 (open offer_sent)", k.PipelineValue)
	}
	if k.ConversionRate <= 0 || k.ConversionRate > 1 {
		t.Fatalf("conversion_rate out of range: %v", k.ConversionRate)
	}
}

func TestCompanyRepo_ListAndDetail(t *testing.T) {
	requireDB(t)
	f := seedDashboard(t)
	repo := NewCompanyRepo()
	ctx := context.Background()

	// role filter (array containment) — company A has 'distributor'.
	byRole, err := repo.ListPage(ctx, domain.CompanyFilter{Role: "distributor", Limit: 100})
	if err != nil {
		t.Fatalf("ListPage role: %v", err)
	}
	if !containsCompany(byRole, f.companyAID) || containsCompany(byRole, f.companyBID) {
		t.Fatalf("role filter wrong: A present=%v B present=%v", containsCompany(byRole, f.companyAID), containsCompany(byRole, f.companyBID))
	}

	// country filter — B is BG.
	byCountry, _ := repo.ListPage(ctx, domain.CompanyFilter{Country: "BG", Limit: 100})
	if !containsCompany(byCountry, f.companyBID) || containsCompany(byCountry, f.companyAID) {
		t.Fatal("country filter wrong")
	}

	// q filter — name ILIKE.
	byName, _ := repo.ListPage(ctx, domain.CompanyFilter{Query: "Acme Distributie Seed", Limit: 100})
	if !containsCompany(byName, f.companyAID) {
		t.Fatal("q filter missed company A by name")
	}

	// detail with roles + latest verification.
	d, err := repo.Detail(ctx, f.companyAID)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if len(d.Roles) == 0 {
		t.Fatal("detail roles missing")
	}
	if d.Verification == nil || d.Verification.VATStatus != "active" {
		t.Fatalf("detail verification missing: %+v", d.Verification)
	}

	if _, err := repo.Detail(ctx, "00000000-0000-0000-0000-000000000000"); err != ports.ErrNotFound {
		t.Fatalf("Detail missing = %v, want ErrNotFound", err)
	}
}

func containsCompany(items []*domain.CompanySummary, id string) bool {
	for _, it := range items {
		if it.ID == id {
			return true
		}
	}
	return false
}
