package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

type LeadRepo struct{}

func NewLeadRepo() *LeadRepo { return &LeadRepo{} }

func (r *LeadRepo) Create(ctx context.Context, lead *domain.Lead) error {
	row := GetPool().QueryRow(ctx, `
		INSERT INTO leads (conversation_id, company_id, contact_id, status)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at
	`, lead.ConversationID, nullStr(lead.CompanyID), nullStr(lead.ContactID), lead.Status)
	return row.Scan(&lead.ID, &lead.CreatedAt)
}

func (r *LeadRepo) GetByConversationID(ctx context.Context, convID string) (*domain.Lead, error) {
	row := GetPool().QueryRow(ctx, `
		SELECT id::text, conversation_id::text,
		       COALESCE(company_id::text, ''),
		       COALESCE(contact_id::text, ''),
		       status, created_at
		FROM leads WHERE conversation_id = $1::uuid
	`, convID)
	var l domain.Lead
	if err := row.Scan(&l.ID, &l.ConversationID, &l.CompanyID, &l.ContactID, &l.Status, &l.CreatedAt); err != nil {
		return nil, err
	}
	return &l, nil
}

func (r *LeadRepo) UpdateCompanyContact(ctx context.Context, leadID, companyID, contactID string) error {
	_, err := GetPool().Exec(ctx, `
		UPDATE leads SET company_id = $2, contact_id = $3 WHERE id = $1
	`, leadID, nullStr(companyID), nullStr(contactID))
	return err
}

func (r *LeadRepo) List(ctx context.Context) ([]*domain.LeadSummary, error) {
	rows, err := GetPool().Query(ctx, `
		SELECT
			l.id,
			l.status,
			COALESCE(c.name, '') AS company_name,
			COALESCE(c.cui, '') AS cui,
			COALESCE(sr.product_name, '') AS product_name,
			sr.quantity,
			COALESCE(sr.unit, '') AS unit,
			COALESCE(sr.delivery_location, '') AS delivery_location,
			l.created_at
		FROM leads l
		LEFT JOIN companies c ON c.id = l.company_id
		LEFT JOIN sourcing_requests sr ON sr.lead_id = l.id
		ORDER BY l.created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var leads []*domain.LeadSummary
	for rows.Next() {
		var s domain.LeadSummary
		if err := rows.Scan(
			&s.ID, &s.Status, &s.CompanyName, &s.CUI,
			&s.ProductName, &s.Quantity, &s.Unit, &s.DeliveryLocation,
			&s.CreatedAt,
		); err != nil {
			return nil, err
		}
		leads = append(leads, &s)
	}
	return leads, rows.Err()
}

func (r *LeadRepo) GetByID(ctx context.Context, id string) (*domain.LeadDetail, error) {
	row := GetPool().QueryRow(ctx, `
		SELECT
			l.id,
			l.status,
			COALESCE(c.name, '') AS company_name,
			COALESCE(c.cui, '') AS cui,
			COALESCE(sr.product_name, '') AS product_name,
			sr.quantity,
			COALESCE(sr.unit, '') AS unit,
			COALESCE(sr.delivery_location, '') AS delivery_location,
			l.created_at,
			COALESCE(c.address, '') AS address,
			COALESCE(c.county, '') AS county,
			COALESCE(ct.phone, '') AS phone,
			COALESCE(ct.email, '') AS email,
			l.conversation_id
		FROM leads l
		LEFT JOIN companies c ON c.id = l.company_id
		LEFT JOIN contacts ct ON ct.id = l.contact_id
		LEFT JOIN sourcing_requests sr ON sr.lead_id = l.id
		WHERE l.id = $1
	`, id)

	var d domain.LeadDetail
	var convID string
	err := row.Scan(
		&d.ID, &d.Status, &d.CompanyName, &d.CUI,
		&d.ProductName, &d.Quantity, &d.Unit, &d.DeliveryLocation,
		&d.CreatedAt, &d.Address, &d.County, &d.Phone, &d.Email, &convID,
	)
	if err != nil {
		return nil, err
	}

	msgRepo := NewMessageRepo()
	msgs, err := msgRepo.ListByConversation(ctx, convID)
	if err != nil {
		return nil, err
	}
	for _, m := range msgs {
		d.Transcript = append(d.Transcript, *m)
	}

	if err := r.loadLeadDetailFacets(ctx, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// leadSummaryColumns is the shared SELECT projection for the paginated pipeline
// list (migration 016/025 fields). It joins the company and the typed sourcing
// request; all are LEFT joins so a thin lead still lists.
const leadSummaryColumns = `
	l.id::text,
	l.status,
	COALESCE(c.name, '') AS company_name,
	COALESCE(c.cui, '') AS cui,
	COALESCE(sr.product, sr.product_name, '') AS product_name,
	sr.quantity,
	COALESCE(sr.unit, '') AS unit,
	COALESCE(sr.delivery_location, '') AS delivery_location,
	l.created_at,
	l.vertical,
	l.assigned_to::text,
	l.needs_human,
	l.offer_value,
	COALESCE(l.offer_note, '')`

// scanLeadSummary maps a row in leadSummaryColumns order into a LeadSummary.
func scanLeadSummary(row pgx.Row) (*domain.LeadSummary, error) {
	var s domain.LeadSummary
	if err := row.Scan(
		&s.ID, &s.Status, &s.CompanyName, &s.CUI,
		&s.ProductName, &s.Quantity, &s.Unit, &s.DeliveryLocation,
		&s.CreatedAt, &s.Vertical, &s.AssignedTo, &s.NeedsHuman,
		&s.OfferValue, &s.OfferNote,
	); err != nil {
		return nil, err
	}
	return &s, nil
}

// ListPage returns one keyset page of the leads pipeline. The keyset predicate
// (created_at, id) < (after.created_at, after.id) plus the `created_at DESC, id
// DESC` order is stable under concurrent inserts and uses the migration-016
// recency/pipeline indexes. It fetches Limit+1 rows so the caller detects a next
// page. All filter values are bound parameters (no SQL built from input).
func (r *LeadRepo) ListPage(ctx context.Context, f domain.LeadFilter) ([]*domain.LeadSummary, error) {
	where := []string{"1=1"}
	args := []any{}
	add := func(clause string, val any) {
		args = append(args, val)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}

	if f.Status != "" {
		add("l.status = $%d", f.Status)
	}
	if f.Vertical != "" {
		add("l.vertical = $%d", f.Vertical)
	}
	if f.AssignedTo != nil {
		if *f.AssignedTo == "" {
			where = append(where, "l.assigned_to IS NULL")
		} else {
			add("l.assigned_to = $%d::uuid", *f.AssignedTo)
		}
	}
	if f.NeedsHuman != nil {
		add("l.needs_human = $%d", *f.NeedsHuman)
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		args = append(args, "%"+q+"%")
		where = append(where, fmt.Sprintf(
			"(c.name ILIKE $%d OR COALESCE(sr.product, sr.product_name) ILIKE $%d)",
			len(args), len(args)))
	}
	if f.After != nil {
		args = append(args, f.After.CreatedAt)
		ca := len(args)
		args = append(args, f.After.ID)
		id := len(args)
		where = append(where, fmt.Sprintf("(l.created_at, l.id) < ($%d, $%d::uuid)", ca, id))
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 25
	}
	args = append(args, limit+1)

	query := `
		SELECT ` + leadSummaryColumns + `
		FROM leads l
		LEFT JOIN companies c ON c.id = l.company_id
		LEFT JOIN sourcing_requests sr ON sr.lead_id = l.id
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY l.created_at DESC, l.id DESC
		LIMIT $` + fmt.Sprint(len(args))

	rows, err := GetPool().Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list leads page: %w", err)
	}
	defer rows.Close()

	var out []*domain.LeadSummary
	for rows.Next() {
		s, err := scanLeadSummary(rows)
		if err != nil {
			return nil, fmt.Errorf("scan lead summary: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Handoffs returns one keyset page of leads needing a human, newest first, each
// with a short snippet of the conversation's last message. It relies on the
// partial index leads_needs_human_idx (migration 016).
func (r *LeadRepo) Handoffs(ctx context.Context, f domain.LeadFilter) ([]*domain.HandoffItem, error) {
	where := []string{"l.needs_human = true"}
	args := []any{}
	if f.After != nil {
		args = append(args, f.After.CreatedAt, f.After.ID)
		where = append(where, "(l.created_at, l.id) < ($1, $2::uuid)")
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 25
	}
	args = append(args, limit+1)

	query := `
		SELECT
			l.id::text,
			l.status,
			l.vertical,
			COALESCE(c.name, '') AS company_name,
			COALESCE(sr.product, sr.product_name, '') AS product_name,
			l.assigned_to::text,
			COALESCE((
				SELECT m.content FROM messages m
				WHERE m.conversation_id = l.conversation_id
				ORDER BY m.created_at DESC LIMIT 1
			), '') AS last_message,
			l.created_at
		FROM leads l
		LEFT JOIN companies c ON c.id = l.company_id
		LEFT JOIN sourcing_requests sr ON sr.lead_id = l.id
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY l.created_at DESC, l.id DESC
		LIMIT $` + fmt.Sprint(len(args))

	rows, err := GetPool().Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list handoffs: %w", err)
	}
	defer rows.Close()

	var out []*domain.HandoffItem
	for rows.Next() {
		var h domain.HandoffItem
		if err := rows.Scan(
			&h.ID, &h.Status, &h.Vertical, &h.CompanyName, &h.ProductName,
			&h.AssignedTo, &h.LastMessage, &h.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan handoff: %w", err)
		}
		h.LastMessage = snippet(h.LastMessage, 140)
		out = append(out, &h)
	}
	return out, rows.Err()
}

// snippet truncates s to at most max runes, appending an ellipsis when cut.
func snippet(s string, max int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return strings.TrimSpace(string(r[:max])) + "…"
}

// StatusExists reports whether status is a valid lead_statuses code (010).
func (r *LeadRepo) StatusExists(ctx context.Context, status string) (bool, error) {
	var exists bool
	err := GetPool().QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM lead_statuses WHERE code = $1)`, status).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("status exists: %w", err)
	}
	return exists, nil
}

// UpdateOffer applies a manual offer change. COALESCE keeps each column unchanged
// when its parameter is NULL, so a partial PATCH only touches the supplied fields.
// It returns the refreshed summary, or ports.ErrNotFound.
func (r *LeadRepo) UpdateOffer(ctx context.Context, leadID string, upd domain.OfferUpdate) (*domain.LeadSummary, error) {
	tag, err := GetPool().Exec(ctx, `
		UPDATE leads SET
			status      = COALESCE($2, status),
			offer_value = COALESCE($3, offer_value),
			offer_note  = COALESCE($4, offer_note),
			updated_at  = now()
		WHERE id = $1::uuid
	`, leadID, upd.Status, upd.Value, upd.Note)
	if err != nil {
		return nil, fmt.Errorf("update offer: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ports.ErrNotFound
	}
	return r.summaryByID(ctx, leadID)
}

// Assign sets leads.assigned_to (nil unassigns). It returns ports.ErrNotFound
// when no such lead exists.
func (r *LeadRepo) Assign(ctx context.Context, leadID string, userID *string) (*domain.LeadSummary, error) {
	var arg any
	if userID != nil {
		arg = *userID
	}
	tag, err := GetPool().Exec(ctx, `
		UPDATE leads SET assigned_to = $2::uuid, updated_at = now()
		WHERE id = $1::uuid
	`, leadID, arg)
	if err != nil {
		return nil, fmt.Errorf("assign lead: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ports.ErrNotFound
	}
	return r.summaryByID(ctx, leadID)
}

// summaryByID re-reads a single lead summary after a mutation.
func (r *LeadRepo) summaryByID(ctx context.Context, leadID string) (*domain.LeadSummary, error) {
	row := GetPool().QueryRow(ctx, `
		SELECT `+leadSummaryColumns+`
		FROM leads l
		LEFT JOIN companies c ON c.id = l.company_id
		LEFT JOIN sourcing_requests sr ON sr.lead_id = l.id
		WHERE l.id = $1::uuid
	`, leadID)
	s, err := scanLeadSummary(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ports.ErrNotFound
		}
		return nil, fmt.Errorf("read lead summary: %w", err)
	}
	return s, nil
}

// KPIs computes the dashboard aggregates in one indexed pass. Definitions:
//   - offers_sent     = leads with status in (offer_sent, negotiation, won)
//   - won             = leads with status = won
//   - qualified       = leads that reached qualification or beyond (the conversion
//     denominator): qualified, offer_requested, offer_sent, negotiation, won, lost
//   - conversion_rate = won / qualified (0 when qualified = 0)
//   - pipeline_value  = sum(offer_value) over open (non-terminal) statuses
func (r *LeadRepo) KPIs(ctx context.Context) (*domain.KPIs, error) {
	const offersSent = "('offer_sent','negotiation','won')"
	const qualifiedPlus = "('qualified','offer_requested','offer_sent','negotiation','won','lost')"

	var k domain.KPIs
	err := GetPool().QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE status IN `+offersSent+`)        AS offers_sent,
			count(*) FILTER (WHERE status = 'won')                  AS won,
			count(*) FILTER (WHERE status IN `+qualifiedPlus+`)     AS qualified,
			COALESCE(sum(offer_value) FILTER (
				WHERE offer_value IS NOT NULL
				  AND status NOT IN ('won','lost')
			), 0)                                                   AS pipeline_value
		FROM leads
	`).Scan(&k.OffersSent, &k.Won, &k.Qualified, &k.PipelineValue)
	if err != nil {
		return nil, fmt.Errorf("compute kpis: %w", err)
	}
	if k.Qualified > 0 {
		k.ConversionRate = float64(k.Won) / float64(k.Qualified)
	}
	return &k, nil
}

// loadLeadDetailFacets enriches a LeadDetail with the pipeline fields, the typed
// sourcing request, the company facet (incl. latest verification + roles), and
// the contact facet — each from its own scoped query keyed on the lead id.
func (r *LeadRepo) loadLeadDetailFacets(ctx context.Context, d *domain.LeadDetail) error {
	// Pipeline fields + the typed sourcing request, in one row.
	var (
		srID, srProduct, srUnit, srLoc *string
		srQty, srBudget                *float64
		srRecurring                    *bool
		srCreatedAt                    *time.Time
	)
	row := GetPool().QueryRow(ctx, `
		SELECT
			l.vertical,
			l.assigned_to::text,
			l.needs_human,
			l.offer_value,
			COALESCE(l.offer_note, ''),
			sr.id::text,
			COALESCE(sr.product, sr.product_name),
			sr.quantity,
			sr.unit,
			sr.delivery_location,
			sr.recurring,
			sr.budget,
			sr.created_at
		FROM leads l
		LEFT JOIN sourcing_requests sr ON sr.lead_id = l.id
		WHERE l.id = $1::uuid
	`, d.ID)

	if err := row.Scan(
		&d.Vertical, &d.AssignedTo, &d.NeedsHuman, &d.OfferValue, &d.OfferNote,
		&srID, &srProduct, &srQty, &srUnit, &srLoc, &srRecurring, &srBudget, &srCreatedAt,
	); err != nil {
		return fmt.Errorf("load lead facets: %w", err)
	}
	if srID != nil {
		sr := &domain.SourcingRequestView{ID: *srID, Quantity: srQty, Budget: srBudget}
		if srProduct != nil {
			sr.Product = *srProduct
		}
		if srUnit != nil {
			sr.Unit = *srUnit
		}
		if srLoc != nil {
			sr.DeliveryLocation = *srLoc
		}
		if srRecurring != nil {
			sr.Recurring = *srRecurring
		}
		if srCreatedAt != nil {
			sr.CreatedAt = *srCreatedAt
		}
		d.SourcingRequest = sr
	}

	if err := r.loadCompanyFacet(ctx, d); err != nil {
		return err
	}
	return r.loadContactFacet(ctx, d)
}

// loadCompanyFacet populates d.Company (with roles[] and the latest verification)
// when the lead is linked to a company.
func (r *LeadRepo) loadCompanyFacet(ctx context.Context, d *domain.LeadDetail) error {
	row := GetPool().QueryRow(ctx, `
		SELECT
			c.id::text,
			COALESCE(c.name, ''),
			COALESCE(c.cui, ''),
			COALESCE(c.country, 'RO'),
			COALESCE(c.reg_no, ''),
			COALESCE(c.caen, ''),
			COALESCE(c.vat_status, ''),
			c.roles,
			cv.source, cv.vat_status, cv.administrators, cv.checked_at
		FROM leads l
		JOIN companies c ON c.id = l.company_id
		LEFT JOIN LATERAL (
			SELECT source, vat_status, administrators, checked_at
			FROM company_verifications
			WHERE company_id = c.id
			ORDER BY checked_at DESC LIMIT 1
		) cv ON true
		WHERE l.id = $1::uuid
	`, d.ID)

	var cv domain.LeadCompanyView
	var vSource, vVAT *string
	var vAdmins []byte
	var vChecked *time.Time
	if err := row.Scan(
		&cv.ID, &cv.Name, &cv.CUI, &cv.Country, &cv.RegNo, &cv.CAEN, &cv.VATStatus,
		&cv.Roles, &vSource, &vVAT, &vAdmins, &vChecked,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // thin lead with no company
		}
		return fmt.Errorf("load company facet: %w", err)
	}
	if cv.Roles == nil {
		cv.Roles = []string{}
	}
	if vSource != nil {
		ver := &domain.CompanyVerificationView{Source: *vSource}
		if vVAT != nil {
			ver.VATStatus = *vVAT
		}
		if len(vAdmins) > 0 {
			ver.Administrators = append([]byte(nil), vAdmins...)
		}
		if vChecked != nil {
			ver.CheckedAt = *vChecked
		}
		cv.Verification = ver
	}
	d.Company = &cv
	return nil
}

// loadContactFacet populates d.Contact when the lead is linked to a contact.
func (r *LeadRepo) loadContactFacet(ctx context.Context, d *domain.LeadDetail) error {
	row := GetPool().QueryRow(ctx, `
		SELECT ct.id::text, COALESCE(ct.name, ''), COALESCE(ct.phone, ''), COALESCE(ct.email, '')
		FROM leads l
		JOIN contacts ct ON ct.id = l.contact_id
		WHERE l.id = $1::uuid
	`, d.ID)
	var c domain.LeadContactView
	if err := row.Scan(&c.ID, &c.Name, &c.Phone, &c.Email); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("load contact facet: %w", err)
	}
	d.Contact = &c
	return nil
}

var _ ports.LeadRepo = (*LeadRepo)(nil)
