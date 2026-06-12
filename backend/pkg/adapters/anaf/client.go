package anaf

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/angrosist/demo/pkg/domain"
)

// ErrNotFound is returned when the CUI does not exist in ANAF.
var ErrNotFound = errors.New("anaf: company not found")

// ErrUnavailable is returned on network failure or non-2xx HTTP status.
var ErrUnavailable = errors.New("anaf: service unavailable")

const defaultBaseURL = "https://webservicesp.anaf.ro/api/PlatitorTvaRest/v9/tva"

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient() *Client {
	baseURL := os.Getenv("DEMOANAF_BASE_URL")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Verify looks up a CUI in ANAF and returns a populated Company.
// Non-numeric or non-positive CUI returns ErrNotFound (not an error worth retrying).
// Network failures / non-2xx return ErrUnavailable — caller surfaces this to the agent.
func (c *Client) Verify(ctx context.Context, cui string) (*domain.Company, error) {
	cuiInt, ok := parseCUI(cui)
	if !ok {
		return nil, ErrNotFound
	}
	return c.callANAF(ctx, cuiInt)
}

// ---- ANAF v9 types --------------------------------------------------------

type anafRequest struct {
	CUI  int    `json:"cui"`
	Data string `json:"data"`
}

type anafResponse struct {
	Found    []anafFoundItem `json:"found"`
	NotFound []int           `json:"notFound"`
}

type anafFoundItem struct {
	DateGenerale      anafDateGenerale `json:"date_generale"`
	ScpTva            bool             `json:"scpTva"`
	StatusTvaInactivi bool             `json:"statusTvaInactivi"`
}

type anafDateGenerale struct {
	Denumire         string `json:"denumire"`
	Adresa           string `json:"adresa"`
	NrRegCom         string `json:"nrRegCom"`
	Telefon          string `json:"telefon"`
	CodCAEN          string `json:"cod_CAEN"`
	DataInregistrare string `json:"data_inregistrare"`
}

// ---- HTTP call ------------------------------------------------------------

func (c *Client) callANAF(ctx context.Context, cui int) (*domain.Company, error) {
	loc, _ := time.LoadLocation("Europe/Bucharest")
	today := time.Now().In(loc).Format("2006-01-02")

	body, _ := json.Marshal([]anafRequest{{CUI: cui, Data: today}})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: HTTP %d", ErrUnavailable, resp.StatusCode)
	}

	var ar anafResponse
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
		return nil, fmt.Errorf("%w: decode: %v", ErrUnavailable, err)
	}

	if len(ar.Found) == 0 {
		return nil, ErrNotFound
	}

	return mapCompany(cui, ar.Found[0]), nil
}

// ---- Mapping --------------------------------------------------------------

func mapCompany(cui int, item anafFoundItem) *domain.Company {
	dg := item.DateGenerale
	addr := ParseAddress(dg.Adresa)

	raw, _ := json.Marshal(item)
	return &domain.Company{
		CUI:      fmt.Sprintf("%d", cui),
		Name:     toTitle(strings.TrimSpace(dg.Denumire)),
		Address:  strings.TrimSpace(addr.Street),
		County:   strings.TrimSpace(addr.County),
		IsActive: !item.StatusTvaInactivi,
		RawData:  raw,
	}
}

// ---- Helpers --------------------------------------------------------------

// parseCUI strips a leading "RO" prefix and parses to a positive integer.
func parseCUI(raw string) (int, bool) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(strings.ToUpper(s), "RO")
	s = strings.TrimSpace(s)
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}
