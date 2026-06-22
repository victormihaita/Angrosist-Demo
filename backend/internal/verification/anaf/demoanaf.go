package anaf

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/verification/caen"
)

// demoANAFDefaultBaseURL is the public DemoANAF REST base. Overridable via
// DEMOANAF_BASE_URL; never hardcoded into a caller.
const demoANAFDefaultBaseURL = "https://demoanaf.ro/api"

// DemoANAFClient is the richer company-verification adapter. It calls
// `${base}/company/{cui}` on DemoANAF.ro and maps the envelope into a
// domain.Company (ONRC reg number, administrators, CAEN, VAT status, derived
// roles, full raw payload). It satisfies ports.CompanyDataProvider.
type DemoANAFClient struct {
	baseURL         string
	httpClient      *http.Client
	demoMode        bool
	fetchFinancials bool
}

// NewDemoANAFClient builds the DemoANAF adapter from the environment. Base URL
// from DEMOANAF_BASE_URL (default https://demoanaf.ro/api); ANAF_DEMO_MODE=true
// short-circuits to deterministic demo data with no network call;
// ANAF_FETCH_FINANCIALS (default true) gates the best-effort /financials call.
func NewDemoANAFClient() *DemoANAFClient {
	baseURL := strings.TrimSpace(os.Getenv("DEMOANAF_BASE_URL"))
	if baseURL == "" {
		baseURL = demoANAFDefaultBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &DemoANAFClient{
		baseURL:         baseURL,
		httpClient:      &http.Client{Timeout: 10 * time.Second},
		demoMode:        strings.EqualFold(strings.TrimSpace(os.Getenv("ANAF_DEMO_MODE")), "true"),
		fetchFinancials: financialsEnabled(),
	}
}

// financialsEnabled reads ANAF_FETCH_FINANCIALS; it defaults to true (enabled)
// when the variable is unset or empty, and is disabled only by an explicit
// "false"/"0"/"no". Never hardcodes behavior; config is the single source.
func financialsEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("ANAF_FETCH_FINANCIALS"))) {
	case "false", "0", "no", "off":
		return false
	default:
		return true
	}
}

// Verify looks up a CUI on DemoANAF and returns a populated domain.Company.
// Invalid CUI -> ErrNotFound; missing company -> ErrNotFound; network/non-2xx
// -> ErrUnavailable. Mirrors the raw-ANAF client's error contract so the agent
// behavior is unchanged.
func (c *DemoANAFClient) Verify(ctx context.Context, cui string) (*domain.Company, error) {
	cuiInt, ok := parseCUI(cui)
	if !ok {
		return nil, ErrNotFound
	}
	if c.demoMode {
		return demoCompany(cuiInt), nil
	}

	cuiStr := fmt.Sprintf("%d", cuiInt)
	company, err := c.callDemoANAF(ctx, cuiStr)
	if err != nil {
		return nil, err
	}

	// Financials are best-effort enrichment: a failure (or 404) must NOT fail
	// Verify. Log and return the company without financials.
	if c.fetchFinancials {
		fins, ferr := c.callFinancials(ctx, cuiStr)
		if ferr != nil {
			slog.WarnContext(ctx, "demoanaf financials fetch failed; continuing without financials",
				slog.String("cui", cuiStr), slog.String("error", ferr.Error()))
		} else {
			company.Financials = fins
		}
	}

	return company, nil
}

// ---- DemoANAF envelope types ----------------------------------------------

type demoANAFEnvelope struct {
	Success bool             `json:"success"`
	Data    *demoANAFCompany `json:"data"`
	Meta    json.RawMessage  `json:"meta"`
}

type demoANAFCompany struct {
	CUI                 json.Number          `json:"cui"`
	Name                string               `json:"name"`
	RegistrationNumber  string               `json:"registrationNumber"`
	RegistrationDate    string               `json:"registrationDate"`
	LegalForm           string               `json:"legalForm"`
	EFacturaRegistered  bool                 `json:"eFacturaRegistered"`
	VATRegistered       bool                 `json:"vatRegistered"`
	VATStatus           string               `json:"vatStatus"`
	Inactive            bool                 `json:"inactive"`
	CAENCode            string               `json:"caenCode"`
	AuthorizedCAENCodes []string             `json:"authorizedCaenCodes"`
	Address             string               `json:"address"`
	HeadquartersAddress *demoANAFAddress     `json:"headquartersAddress"`
	Administrators      []demoANAFAdminEntry `json:"administrators"`
}

type demoANAFAddress struct {
	Street     string `json:"street"`
	Number     string `json:"number"`
	Locality   string `json:"locality"`
	County     string `json:"county"`
	Country    string `json:"country"`
	PostalCode string `json:"postalCode"`
}

type demoANAFAdminEntry struct {
	Name     string `json:"name"`
	Role     string `json:"role"`
	PersonID string `json:"personId"`
}

// ---- HTTP call ------------------------------------------------------------

func (c *DemoANAFClient) callDemoANAF(ctx context.Context, cui string) (*domain.Company, error) {
	url := c.baseURL + "/company/" + cui

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: build request: %v", ErrUnavailable, err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: HTTP %d", ErrUnavailable, resp.StatusCode)
	}

	var env demoANAFEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, fmt.Errorf("%w: decode: %v", ErrUnavailable, err)
	}
	if !env.Success || env.Data == nil {
		return nil, ErrNotFound
	}

	return mapDemoANAFCompany(cui, env.Data)
}

// ---- Mapping --------------------------------------------------------------

// mapDemoANAFCompany maps a DemoANAF `data` object onto a domain.Company per
// docs/specs/ANAF_API.md. reg_no is canonicalized to the CUI (the (country,
// reg_no) dedup key); the ONRC J-number is preserved in RegistrationNumber.
func mapDemoANAFCompany(cui string, d *demoANAFCompany) (*domain.Company, error) {
	// Re-marshal the data object so RawData / RawVerification hold the exact
	// DemoANAF payload (cache + audit), independent of our struct shape.
	raw, err := json.Marshal(d)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal raw: %v", ErrUnavailable, err)
	}

	// Prefer the company's own CUI when present; fall back to the requested one.
	regNo := strings.TrimSpace(d.CUI.String())
	if regNo == "" || regNo == "0" {
		regNo = cui
	}

	caenCode := strings.TrimSpace(d.CAENCode)
	authorized := trimAll(d.AuthorizedCAENCodes)

	var address, county string
	if d.HeadquartersAddress != nil {
		address, county = formatHeadquarters(d.HeadquartersAddress)
	}
	if address == "" {
		// Fall back to the flat address string parsed like raw ANAF.
		pa := ParseAddress(strings.TrimSpace(d.Address))
		address = pa.Street
		if county == "" {
			county = pa.County
		}
	}

	vatStatus := normalizeVATStatus(d.VATStatus, d.VATRegistered)

	admins := make([]string, 0, len(d.Administrators))
	for _, a := range d.Administrators {
		if n := strings.TrimSpace(a.Name); n != "" {
			admins = append(admins, toTitle(n))
		}
	}

	return &domain.Company{
		CUI:                regNo,
		Country:            "RO",
		RegNo:              regNo,
		RegistrationNumber: strings.TrimSpace(d.RegistrationNumber),
		RegistrationDate:   parseANAFDate(d.RegistrationDate),
		LegalForm:          strings.TrimSpace(d.LegalForm),
		EFactura:           d.EFacturaRegistered,
		Name:               toTitle(strings.TrimSpace(d.Name)),
		Address:            strings.TrimSpace(address),
		County:             strings.TrimSpace(county),
		IsActive:           !d.Inactive,
		VATStatus:          vatStatus,
		CAEN:               caenCode,
		AuthorizedCAEN:     authorized,
		Roles:              caen.RolesForCAEN(caenCode, authorized...),
		Administrators:     admins,
		RawData:            raw,
		RawVerification:    json.RawMessage(raw),
	}, nil
}

// parseANAFDate parses a DemoANAF date. It accepts the common ISO date and full
// RFC3339 timestamp shapes and returns nil for empty/unparseable values so a
// missing incorporation date is preserved as NULL rather than a zero time.
func parseANAFDate(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	for _, layout := range []string{"2006-01-02", time.RFC3339, "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return &t
		}
	}
	return nil
}

// formatHeadquarters renders a human-readable street line and the county from
// the structured headquarters address. Title-cases ALL-CAPS ANAF values.
func formatHeadquarters(a *demoANAFAddress) (street, county string) {
	parts := make([]string, 0, 3)
	if s := strings.TrimSpace(a.Street); s != "" {
		parts = append(parts, toTitle(s))
	}
	if n := strings.TrimSpace(a.Number); n != "" {
		parts = append(parts, "Nr. "+n)
	}
	if l := strings.TrimSpace(a.Locality); l != "" {
		parts = append(parts, toTitle(l))
	}
	return strings.Join(parts, ", "), toTitle(strings.TrimSpace(a.County))
}

// ---- Financials -----------------------------------------------------------

// DemoANAF financial-indicator codes we map (see docs/specs/ANAF_API.md).
const (
	indicatorNetTurnover = "I13" // net turnover
	indicatorProfit      = "I18" // net profit
	indicatorLoss        = "I19" // net loss
	indicatorEmployees   = "I20" // average number of employees
)

type demoANAFFinancialsEnvelope struct {
	Success bool                    `json:"success"`
	Data    []demoANAFFinancialYear `json:"data"`
	Meta    json.RawMessage         `json:"meta"`
}

type demoANAFFinancialYear struct {
	Year            int                 `json:"year"`
	CAENCode        string              `json:"caenCode"`
	CAENDescription string              `json:"caenDescription"`
	Indicators      []demoANAFIndicator `json:"indicators"`
}

type demoANAFIndicator struct {
	Code  string      `json:"code"`
	Value json.Number `json:"value"`
}

// callFinancials fetches `${base}/company/{cui}/financials` and maps the per-year
// indicator set into domain.CompanyFinancialYear (newest year first). It reuses
// the shared HTTP client/timeout. A 404 yields an empty slice (no financials on
// file is not an error); other non-2xx / decode failures return an error which
// the caller treats as best-effort (logged, not fatal to Verify).
func (c *DemoANAFClient) callFinancials(ctx context.Context, cui string) ([]domain.CompanyFinancialYear, error) {
	url := c.baseURL + "/company/" + cui + "/financials"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: build financials request: %v", ErrUnavailable, err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // no financials on file — not an error
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: HTTP %d", ErrUnavailable, resp.StatusCode)
	}

	var env demoANAFFinancialsEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, fmt.Errorf("%w: decode financials: %v", ErrUnavailable, err)
	}
	if !env.Success {
		return nil, nil
	}
	return mapFinancials(env.Data), nil
}

// mapFinancials converts DemoANAF financial-year records into the domain shape,
// picking out net turnover (I13), net profit (I18 - I19) and average employees
// (I20). Years are returned newest first; years with no recognized indicators are
// still emitted (caller may show the year/CAEN even without figures).
func mapFinancials(years []demoANAFFinancialYear) []domain.CompanyFinancialYear {
	if len(years) == 0 {
		return nil
	}
	out := make([]domain.CompanyFinancialYear, 0, len(years))
	for _, y := range years {
		if y.Year <= 0 {
			continue
		}
		fy := domain.CompanyFinancialYear{
			Year:            y.Year,
			CAENDescription: strings.TrimSpace(y.CAENDescription),
		}
		var profit, loss *float64
		for _, ind := range y.Indicators {
			switch strings.ToUpper(strings.TrimSpace(ind.Code)) {
			case indicatorNetTurnover:
				fy.Turnover = numberToFloat(ind.Value)
			case indicatorProfit:
				profit = numberToFloat(ind.Value)
			case indicatorLoss:
				loss = numberToFloat(ind.Value)
			case indicatorEmployees:
				fy.Employees = numberToInt(ind.Value)
			}
		}
		if net := netProfit(profit, loss); net != nil {
			fy.NetProfit = net
		}
		out = append(out, fy)
	}
	sortYearsDesc(out)
	return out
}

// netProfit derives net profit from the reported profit (I18) and loss (I19)
// indicators. DemoANAF reports them as separate non-negative figures; net =
// profit - loss. Returns nil only when neither indicator was present.
func netProfit(profit, loss *float64) *float64 {
	if profit == nil && loss == nil {
		return nil
	}
	var v float64
	if profit != nil {
		v += *profit
	}
	if loss != nil {
		v -= *loss
	}
	return &v
}

func numberToFloat(n json.Number) *float64 {
	s := strings.TrimSpace(n.String())
	if s == "" {
		return nil
	}
	f, err := n.Float64()
	if err != nil {
		return nil
	}
	return &f
}

func numberToInt(n json.Number) *int {
	f := numberToFloat(n)
	if f == nil {
		return nil
	}
	i := int(*f)
	return &i
}

// sortYearsDesc orders the financial years newest first (stable, small slices).
func sortYearsDesc(in []domain.CompanyFinancialYear) {
	for i := 1; i < len(in); i++ {
		for j := i; j > 0 && in[j].Year > in[j-1].Year; j-- {
			in[j], in[j-1] = in[j-1], in[j]
		}
	}
}

// normalizeVATStatus maps the provider's VAT fields onto our documented set:
// active | inactive | not_registered | unknown.
func normalizeVATStatus(status string, registered bool) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "active":
		return "active"
	case "inactive":
		return "inactive"
	case "not_registered", "notregistered", "neinregistrat":
		return "not_registered"
	case "":
		if registered {
			return "active"
		}
		return "not_registered"
	default:
		return "unknown"
	}
}

func trimAll(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, t)
		}
	}
	return out
}
