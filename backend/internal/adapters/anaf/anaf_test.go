package anaf

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ---- titlecase tests -------------------------------------------------------

func TestToTitle(t *testing.T) {
	cases := []struct{ in, want string }{
		{"EURO INTERMED SRL", "Euro Intermed Srl"},
		{"MUNICIPIUL BUCUREȘTI", "Municipiul București"},
		{"JUDEȚUL CLUJ", "Județul Cluj"},
		{"SC DEMO COMPANY 123 SRL", "Sc Demo Company 123 Srl"},
	}
	for _, c := range cases {
		got := toTitle(c.in)
		if got != c.want {
			t.Errorf("toTitle(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}

// ---- address parser tests --------------------------------------------------

func TestParseAddress(t *testing.T) {
	tests := []struct {
		raw    string
		city   string
		county string
		street string
	}{
		{
			raw:    "MUNICIPIUL CLUJ-NAPOCA, JUD. CLUJ, STR. EXEMPLU, NR. 5",
			city:   "Cluj-Napoca",
			county: "Cluj",
			street: "Str. Exemplu, Nr. 5",
		},
		{
			raw:    "MUNICIPIUL BUCUREȘTI, SECTOR 1, STR. VICTORIEI, NR. 1, AP. 2",
			city:   "București",
			county: "",           // no JUD. segment for Bucharest
			street: "Sector 1, Str. Victoriei, Nr. 1, Ap. 2",
		},
		{
			raw:    "ORAȘ BRAȘOV, JUDEȚUL BRAȘOV, STR. LUNGĂ, NR. 10",
			city:   "Brașov",
			county: "Brașov",
			street: "Str. Lungă, Nr. 10",
		},
	}
	for _, tt := range tests {
		got := ParseAddress(tt.raw)
		if got.City != tt.city {
			t.Errorf("ParseAddress(%q).City = %q; want %q", tt.raw, got.City, tt.city)
		}
		if got.County != tt.county {
			t.Errorf("ParseAddress(%q).County = %q; want %q", tt.raw, got.County, tt.county)
		}
		if got.Street != tt.street {
			t.Errorf("ParseAddress(%q).Street = %q; want %q", tt.raw, got.Street, tt.street)
		}
	}
}

// ---- parseCUI tests --------------------------------------------------------

func TestParseCUI(t *testing.T) {
	ok := []struct {
		in   string
		want int
	}{
		{"12345678", 12345678},
		{"RO12345678", 12345678},
		{" ro 12345678 ", 12345678},
		{"41651600", 41651600},
	}
	for _, c := range ok {
		n, valid := parseCUI(c.in)
		if !valid || n != c.want {
			t.Errorf("parseCUI(%q) = (%d, %v); want (%d, true)", c.in, n, valid, c.want)
		}
	}

	bad := []string{"", "0", "-1", "abc", "RO", "RO abc"}
	for _, s := range bad {
		_, valid := parseCUI(s)
		if valid {
			t.Errorf("parseCUI(%q) should be invalid", s)
		}
	}
}

// ---- HTTP client tests -----------------------------------------------------

func TestCallANAF_found(t *testing.T) {
	payload := map[string]any{
		"found": []map[string]any{
			{
				"date_generale": map[string]any{
					"denumire":          "EURO INTERMED SRL",
					"adresa":            "MUNICIPIUL CLUJ-NAPOCA, JUD. CLUJ, STR. EXEMPLU, NR. 1",
					"nrRegCom":          "J12/1234/2020",
					"cod_CAEN":          "4611",
					"data_inregistrare": "2020-01-15",
					"telefon":           "0712345678",
				},
				"scpTva":            true,
				"statusTvaInactivi": false,
			},
		},
		"notFound": []int{},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	company, err := c.callANAF(t.Context(), 12345678)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if company.Name != "Euro Intermed Srl" {
		t.Errorf("Name = %q; want %q", company.Name, "Euro Intermed Srl")
	}
	if company.County != "Cluj" {
		t.Errorf("County = %q; want %q", company.County, "Cluj")
	}
	if !company.IsActive {
		t.Error("IsActive should be true")
	}
}

func TestCallANAF_notFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"found": []any{}, "notFound": []int{99}})
	}))
	defer srv.Close()

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	_, err := c.callANAF(t.Context(), 99)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestCallANAF_serverError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	_, err := c.callANAF(t.Context(), 12345678)
	if err == nil {
		t.Error("expected error on 503")
	}
}

func TestVerify_invalidCUI(t *testing.T) {
	c := &Client{}
	_, err := c.Verify(t.Context(), "abc")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for invalid CUI, got %v", err)
	}
}

func TestVerify_demoMode(t *testing.T) {
	c := &Client{demoMode: true}
	company, err := c.Verify(t.Context(), "41651600")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if company.CUI != "41651600" {
		t.Errorf("CUI = %q; want %q", company.CUI, "41651600")
	}
	if !company.IsActive {
		t.Error("demo company should be active")
	}
}
