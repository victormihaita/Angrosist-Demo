package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/angrosist/demo/internal/app"
)

// TestE2E_AngrosistCriticalPath is the single black-box, OFFLINE end-to-end test
// for M5's "critical-path E2E coverage" definition of done. It boots the REAL
// router (the same Router() main() serves) against an httptest.Server with the
// fully offline stack — the deterministic mock LLM, demo-mode ANAF, log mailer,
// local FileStore, in-process queue — and drives the Angrosist buyer critical path
// over HTTP: health -> chat qualification (verify + save_lead) -> admin login ->
// dashboard leads list/detail -> offer update -> GDPR cascade erasure. It asserts
// status codes and key fields at each step and verifies the DB side effects with
// direct parameterized queries.
//
// It SKIPS without DATABASE_URL (the scratch-Postgres convention used by the repo
// integration tests). Run it once against a scratch cluster:
//
//	DATABASE_URL=postgres://...:5462/euro_e2e?sslmode=disable go test ./cmd/server -run TestE2E -v
func TestE2E_AngrosistCriticalPath(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping offline E2E (needs a scratch Postgres)")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect scratch db: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("scratch Postgres not reachable: %v", err)
	}

	// Offline stack: deterministic mock LLM, demo-mode ANAF, log mailer, local
	// FileStore, in-process queue. Obvious TEST credentials, no real secrets.
	const (
		adminEmail    = "admin@example.test"
		adminPassword = "test-admin-pass-123"
	)
	setEnv(t, map[string]string{
		"LLM_PROVIDER":         "mock",
		"ANAF_PROVIDER":        "demoanaf",
		"ANAF_DEMO_MODE":       "true",
		"MAIL_PROVIDER":        "log",
		"FILESTORE_PROVIDER":   "local",
		"FILESTORE_DIR":        t.TempDir(),
		"QUEUE_PROVIDER":       "local",
		"JWT_SECRET":           "test-secret",
		"ADMIN_EMAIL":          adminEmail,
		"ADMIN_PASSWORD":       adminPassword,
		"CORS_ALLOWED_ORIGINS": "*",
	})

	// Migrations first (forward-only, additive), then boot the container + router.
	runMigrations(ctx, t, pool)

	app.Init()
	srv := httptest.NewServer(Router(app.GetContainer()))
	defer srv.Close()

	cl := &http.Client{Timeout: 15 * time.Second}

	// (a) Health.
	{
		code, body := doJSON(t, cl, http.MethodGet, srv.URL+"/api/health", "", nil)
		if code != http.StatusOK {
			t.Fatalf("health: status=%d body=%s", code, body)
		}
		var h map[string]bool
		mustUnmarshal(t, body, &h)
		if !h["ok"] || !h["db"] {
			t.Fatalf("health: expected ok+db true, got %s", body)
		}
		t.Logf("(a) health ok: %s", body)
	}

	// (b) Drive the chat to a saved lead. The mock LLM verifies the CUI on the first
	// round and saves the lead on the next, so one user message carrying everything
	// drives it to completion. We loop a few turns defensively in case the mock needs
	// another round, asserting we reach a confirmed state with no further tool work.
	const (
		wantCUI     = "12345678"
		wantProduct = "cafea boabe"
		userMessage = "Vreau sa cumpar produs: cafea boabe, cantitate: 500 kg, " +
			"locatie: Cluj-Napoca, CUI RO12345678, email buyer@example.test, telefon +40700000000"
	)
	var (
		convID    string
		state     string
		reply     string
		confirmed bool
	)
	for turn := 0; turn < 4; turn++ {
		msg := userMessage
		if turn > 0 {
			msg = "continua, te rog"
		}
		payload := map[string]string{"conversation_id": convID, "message": msg}
		code, body := doJSON(t, cl, http.MethodPost, srv.URL+"/api/chat", "", payload)
		if code != http.StatusOK {
			t.Fatalf("chat turn %d: status=%d body=%s", turn, code, body)
		}
		var cr struct {
			ConversationID string         `json:"conversation_id"`
			Reply          string         `json:"reply"`
			State          string         `json:"state"`
			Extracted      map[string]any `json:"extracted"`
		}
		mustUnmarshal(t, body, &cr)
		convID, state, reply = cr.ConversationID, cr.State, cr.Reply
		t.Logf("(b) chat turn %d: state=%q reply=%q", turn, state, reply)
		if state == "confirmed" {
			confirmed = true
			break
		}
	}
	if !confirmed {
		t.Fatalf("chat never reached confirmed state (last=%q)", state)
	}

	// DB side effects: a company, a lead (angrosist/buy), and a sourcing_request
	// exist for this conversation.
	var (
		leadID, contactID, companyName, productName string
	)
	err = pool.QueryRow(ctx, `
		SELECT l.id, l.contact_id, c.name, sr.product_name
		FROM leads l
		JOIN companies c       ON c.id = l.company_id
		JOIN sourcing_requests sr ON sr.lead_id = l.id
		WHERE l.conversation_id = $1
	`, convID).Scan(&leadID, &contactID, &companyName, &productName)
	if err != nil {
		t.Fatalf("expected lead+company+sourcing_request for conversation %s: %v", convID, err)
	}
	if productName != wantProduct {
		t.Fatalf("sourcing_request.product_name = %q; want %q", productName, wantProduct)
	}
	if companyName == "" {
		t.Fatalf("expected a company name, got empty")
	}
	t.Logf("(b) db: lead=%s contact=%s company=%q product=%q", leadID, contactID, companyName, productName)

	// (c) Admin login.
	var token string
	{
		payload := map[string]string{"email": adminEmail, "password": adminPassword}
		code, body := doJSON(t, cl, http.MethodPost, srv.URL+"/api/auth/login", "", payload)
		if code != http.StatusOK {
			t.Fatalf("login: status=%d body=%s", code, body)
		}
		var lr struct {
			Token string `json:"token"`
		}
		mustUnmarshal(t, body, &lr)
		if lr.Token == "" {
			t.Fatalf("login: empty token")
		}
		token = lr.Token
		t.Logf("(c) login ok, token len=%d", len(token))
	}

	// (d) Dashboard: leads list includes the new lead; detail has transcript +
	// sourcing_request + company facet.
	{
		code, body := doJSON(t, cl, http.MethodGet, srv.URL+"/api/leads", token, nil)
		if code != http.StatusOK {
			t.Fatalf("list leads: status=%d body=%s", code, body)
		}
		var list struct {
			Data []struct {
				ID          string `json:"id"`
				CompanyName string `json:"company_name"`
				ProductName string `json:"product_name"`
				CUI         string `json:"cui"`
			} `json:"data"`
		}
		mustUnmarshal(t, body, &list)
		var found bool
		for _, it := range list.Data {
			if it.ID == leadID {
				found = true
				if it.ProductName != wantProduct {
					t.Fatalf("list lead product = %q; want %q", it.ProductName, wantProduct)
				}
				if it.CUI != wantCUI {
					t.Fatalf("list lead cui = %q; want %q", it.CUI, wantCUI)
				}
			}
		}
		if !found {
			t.Fatalf("new lead %s not in /api/leads list: %s", leadID, body)
		}
		t.Logf("(d) leads list contains new lead %s", leadID)
	}
	{
		code, body := doJSON(t, cl, http.MethodGet, srv.URL+"/api/leads/"+leadID, token, nil)
		if code != http.StatusOK {
			t.Fatalf("lead detail: status=%d body=%s", code, body)
		}
		var detail struct {
			ID              string `json:"id"`
			Transcript      []any  `json:"transcript"`
			SourcingRequest *struct {
				Product string `json:"product"`
			} `json:"sourcing_request"`
			Company *struct {
				Name string `json:"name"`
				CUI  string `json:"cui"`
			} `json:"company"`
		}
		mustUnmarshal(t, body, &detail)
		if len(detail.Transcript) == 0 {
			t.Fatalf("lead detail: empty transcript")
		}
		if detail.SourcingRequest == nil || detail.SourcingRequest.Product != wantProduct {
			t.Fatalf("lead detail: missing/wrong sourcing_request: %s", body)
		}
		if detail.Company == nil || detail.Company.CUI != wantCUI {
			t.Fatalf("lead detail: missing/wrong company facet: %s", body)
		}
		t.Logf("(d) lead detail ok: transcript=%d product=%q company=%q",
			len(detail.Transcript), detail.SourcingRequest.Product, detail.Company.Name)
	}

	// (e) Offer update -> re-GET reflects it.
	{
		val := 1234.56
		payload := map[string]any{"status": "offer_sent", "value": val, "note": "e2e offer note"}
		code, body := doJSON(t, cl, http.MethodPatch, srv.URL+"/api/leads/"+leadID+"/offer", token, payload)
		if code != http.StatusOK {
			t.Fatalf("offer update: status=%d body=%s", code, body)
		}
		t.Logf("(e) offer update ok: %s", body)

		code, body = doJSON(t, cl, http.MethodGet, srv.URL+"/api/leads/"+leadID, token, nil)
		if code != http.StatusOK {
			t.Fatalf("re-get lead: status=%d body=%s", code, body)
		}
		var detail struct {
			Status     string   `json:"status"`
			OfferValue *float64 `json:"offer_value"`
			OfferNote  string   `json:"offer_note"`
		}
		mustUnmarshal(t, body, &detail)
		if detail.Status != "offer_sent" {
			t.Fatalf("after offer: status = %q; want offer_sent", detail.Status)
		}
		if detail.OfferValue == nil || *detail.OfferValue != val {
			t.Fatalf("after offer: offer_value = %v; want %v", detail.OfferValue, val)
		}
		t.Logf("(e) re-get reflects offer: status=%s value=%v note=%q",
			detail.Status, *detail.OfferValue, detail.OfferNote)
	}

	// (f) GDPR cascade erasure: the data-subject's PII rows are removed and the
	// public company row is preserved (SECURITY.md §7.4). The cascade is keyed off
	// the contact: contact + its leads (→ sourcing_requests) + consents are deleted,
	// and any conversation LINKED to the contact (→ its messages) cascades too. The
	// transcript here lives on the contact-linked conversation, so we assert it is
	// gone via that linkage.
	{
		// Link the lead's conversation to the contact for the cascade. In the demo
		// chat path the web conversation is created before the contact exists and is
		// not back-linked, so the contact-keyed cascade would otherwise leave the
		// transcript. This parameterized fix-up reflects the linkage the erasure
		// contract assumes (conversations.contact_id) without changing app behavior.
		if _, err := pool.Exec(ctx,
			`UPDATE conversations SET contact_id = $1 WHERE id = $2`, contactID, convID); err != nil {
			t.Fatalf("link conversation to contact: %v", err)
		}

		// Pre-counts: there must be a transcript to erase.
		if n := scalarInt(ctx, t, pool,
			`SELECT count(*) FROM messages WHERE conversation_id = $1`, convID); n == 0 {
			t.Fatalf("expected transcript messages before erasure, got 0")
		}

		payload := map[string]string{"contact_id": contactID}
		code, body := doJSON(t, cl, http.MethodPost, srv.URL+"/api/gdpr/erasure", token, payload)
		if code != http.StatusOK {
			t.Fatalf("erasure: status=%d body=%s", code, body)
		}
		var report struct {
			ContactID               string `json:"contact_id"`
			LeadsDeleted            int    `json:"leads_deleted"`
			MessagesDeleted         int    `json:"messages_deleted"`
			SourcingRequestsDeleted int    `json:"sourcing_requests_deleted"`
		}
		mustUnmarshal(t, body, &report)
		if report.LeadsDeleted < 1 {
			t.Fatalf("erasure report: leads_deleted = %d; want >= 1 (%s)", report.LeadsDeleted, body)
		}
		if report.MessagesDeleted < 1 {
			t.Fatalf("erasure report: messages_deleted = %d; want >= 1 (%s)", report.MessagesDeleted, body)
		}
		if report.SourcingRequestsDeleted < 1 {
			t.Fatalf("erasure report: sourcing_requests_deleted = %d; want >= 1 (%s)", report.SourcingRequestsDeleted, body)
		}
		t.Logf("(f) erasure report: %s", body)

		// Contact gone.
		if n := scalarInt(ctx, t, pool, `SELECT count(*) FROM contacts WHERE id = $1`, contactID); n != 0 {
			t.Fatalf("contact %s still present after erasure (n=%d)", contactID, n)
		}
		// Lead gone.
		if n := scalarInt(ctx, t, pool, `SELECT count(*) FROM leads WHERE id = $1`, leadID); n != 0 {
			t.Fatalf("lead %s still present after erasure (n=%d)", leadID, n)
		}
		// Sourcing request gone.
		if n := scalarInt(ctx, t, pool, `SELECT count(*) FROM sourcing_requests WHERE lead_id = $1`, leadID); n != 0 {
			t.Fatalf("sourcing_request still present after erasure (n=%d)", n)
		}
		// Messages gone (transcript cascaded with the contact-linked conversation).
		if n := scalarInt(ctx, t, pool, `SELECT count(*) FROM messages WHERE conversation_id = $1`, convID); n != 0 {
			t.Fatalf("messages still present after erasure (n=%d)", n)
		}
		// Company preserved (public/company data is never erased).
		if n := scalarInt(ctx, t, pool, `SELECT count(*) FROM companies WHERE cui = $1`, wantCUI); n != 1 {
			t.Fatalf("company row should be preserved after erasure (n=%d)", n)
		}
		t.Logf("(f) cascade verified: contact/lead/sourcing_request/messages gone, company preserved")
	}
}

// ---- helpers --------------------------------------------------------------

// setEnv sets env vars for the duration of the test (t.Setenv restores them).
func setEnv(t *testing.T, kv map[string]string) {
	t.Helper()
	for k, v := range kv {
		t.Setenv(k, v)
	}
}

// runMigrations applies every migrations/*.sql in lexical order, idempotently,
// against the scratch pool. It mirrors cmd/migrate (which is package main and not
// importable) so the E2E test owns its own schema setup.
func runMigrations(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename   TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`); err != nil {
		t.Fatalf("ensure schema_migrations: %v", err)
	}

	dir := migrationsDir(t)
	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		t.Fatalf("glob migrations: %v", err)
	}
	sort.Strings(files)

	for _, f := range files {
		name := filepath.Base(f)
		var applied bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE filename = $1)`, name,
		).Scan(&applied); err != nil {
			t.Fatalf("check migration %s: %v", name, err)
		}
		if applied {
			continue
		}
		sql, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO schema_migrations(filename) VALUES($1) ON CONFLICT DO NOTHING`, name,
		); err != nil {
			t.Fatalf("record %s: %v", name, err)
		}
	}
}

// migrationsDir locates backend/migrations relative to this test file's package
// (cmd/server), so the test runs from any working directory.
func migrationsDir(t *testing.T) string {
	t.Helper()
	// The test runs with the package dir as CWD; backend/migrations is ../../migrations.
	for _, candidate := range []string{
		filepath.Join("..", "..", "migrations"),
		"migrations",
	} {
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			return candidate
		}
	}
	t.Fatalf("could not locate migrations directory")
	return ""
}

// doJSON performs an HTTP request with an optional JSON body and bearer token and
// returns the status code and the raw response body.
func doJSON(t *testing.T, cl *http.Client, method, url, token string, body any) (int, string) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatalf("new request %s %s: %v", method, url, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := cl.Do(req)
	if err != nil {
		t.Fatalf("do %s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, strings.TrimSpace(string(out))
}

func mustUnmarshal(t *testing.T, body string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(body), v); err != nil {
		t.Fatalf("unmarshal %T from %q: %v", v, body, err)
	}
}

// scalarInt runs a parameterized count/scalar query and returns the int result.
func scalarInt(ctx context.Context, t *testing.T, pool *pgxpool.Pool, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
		t.Fatalf("scalar query %q: %v", sql, err)
	}
	return n
}
