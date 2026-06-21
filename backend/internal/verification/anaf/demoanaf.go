package anaf

import (
	"context"
	"encoding/json"
	"fmt"
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
	baseURL    string
	httpClient *http.Client
	demoMode   bool
}

// NewDemoANAFClient builds the DemoANAF adapter from the environment. Base URL
// from DEMOANAF_BASE_URL (default https://demoanaf.ro/api); ANAF_DEMO_MODE=true
// short-circuits to deterministic demo data with no network call.
func NewDemoANAFClient() *DemoANAFClient {
	baseURL := strings.TrimSpace(os.Getenv("DEMOANAF_BASE_URL"))
	if baseURL == "" {
		baseURL = demoANAFDefaultBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &DemoANAFClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		demoMode:   strings.EqualFold(strings.TrimSpace(os.Getenv("ANAF_DEMO_MODE")), "true"),
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
	return c.callDemoANAF(ctx, fmt.Sprintf("%d", cuiInt))
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
