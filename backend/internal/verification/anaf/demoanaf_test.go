package anaf

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// demoANAFBody is a representative DemoANAF /company/:cui response envelope.
const demoANAFBody = `{
  "success": true,
  "data": {
    "cui": 14399840,
    "name": "DANTE INTERNATIONAL SA",
    "registrationNumber": "J40/372/2002",
    "vatRegistered": true,
    "vatStatus": "active",
    "inactive": false,
    "caenCode": "4791",
    "authorizedCaenCodes": ["4791", "4690", "5610"],
    "address": "MUNICIPIUL BUCUREȘTI, SECTOR 6, STR. VIRTUTII, NR. 148",
    "headquartersAddress": {
      "street": "STR. VIRTUTII",
      "number": "148",
      "locality": "BUCUREȘTI",
      "county": "BUCUREȘTI",
      "country": "RO",
      "postalCode": "060787"
    },
    "administrators": [
      {"name": "ION POPESCU", "role": "Administrator", "personId": "p1"},
      {"name": "MARIA IONESCU", "role": "Administrator", "personId": "p2"}
    ]
  },
  "meta": {"cached": true, "source": "anaf", "cachedAt": "2026-06-21T10:00:00Z"}
}`

func TestCallDemoANAF_mapsRichResponse(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(demoANAFBody))
	}))
	defer srv.Close()

	c := &DemoANAFClient{baseURL: srv.URL, httpClient: srv.Client()}
	company, err := c.Verify(t.Context(), "14399840")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotPath != "/company/14399840" {
		t.Errorf("request path = %q; want /company/14399840", gotPath)
	}
	if company.Country != "RO" {
		t.Errorf("Country = %q; want RO", company.Country)
	}
	if company.RegNo != "14399840" || company.CUI != "14399840" {
		t.Errorf("RegNo/CUI = %q/%q; want 14399840", company.RegNo, company.CUI)
	}
	if company.RegistrationNumber != "J40/372/2002" {
		t.Errorf("RegistrationNumber = %q; want J40/372/2002", company.RegistrationNumber)
	}
	if company.Name != "Dante International Sa" {
		t.Errorf("Name = %q; want title-cased", company.Name)
	}
	if company.County != "București" {
		t.Errorf("County = %q; want București", company.County)
	}
	if company.Address == "" {
		t.Error("Address should be derived from headquartersAddress")
	}
	if !company.IsActive {
		t.Error("IsActive should be true (inactive=false)")
	}
	if company.VATStatus != "active" {
		t.Errorf("VATStatus = %q; want active", company.VATStatus)
	}
	if company.CAEN != "4791" {
		t.Errorf("CAEN = %q; want 4791", company.CAEN)
	}
	wantAdmins := []string{"Ion Popescu", "Maria Ionescu"}
	if !reflect.DeepEqual(company.Administrators, wantAdmins) {
		t.Errorf("Administrators = %v; want %v", company.Administrators, wantAdmins)
	}
	// 4791 (retail) + authorized 4690 (wholesale) + 5610 (horeca) -> derived roles.
	wantRoles := []string{"buyer", "distributor", "horeca", "retailer", "wholesaler"}
	if !reflect.DeepEqual(company.Roles, wantRoles) {
		t.Errorf("Roles = %v; want %v", company.Roles, wantRoles)
	}
	if len(company.RawVerification) == 0 {
		t.Error("RawVerification should hold the provider payload")
	}
}

func TestCallDemoANAF_notFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := &DemoANAFClient{baseURL: srv.URL, httpClient: srv.Client()}
	if _, err := c.Verify(t.Context(), "99"); err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestCallDemoANAF_successFalse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"success": false, "data": null}`))
	}))
	defer srv.Close()

	c := &DemoANAFClient{baseURL: srv.URL, httpClient: srv.Client()}
	if _, err := c.Verify(t.Context(), "12345678"); err != ErrNotFound {
		t.Errorf("expected ErrNotFound for success=false, got %v", err)
	}
}

func TestCallDemoANAF_serverError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	c := &DemoANAFClient{baseURL: srv.URL, httpClient: srv.Client()}
	if _, err := c.Verify(t.Context(), "12345678"); err == nil {
		t.Error("expected ErrUnavailable on 502")
	}
}

func TestDemoANAF_invalidCUI(t *testing.T) {
	c := &DemoANAFClient{}
	if _, err := c.Verify(t.Context(), "abc"); err != ErrNotFound {
		t.Errorf("expected ErrNotFound for invalid CUI, got %v", err)
	}
}

func TestDemoANAF_demoMode(t *testing.T) {
	c := &DemoANAFClient{demoMode: true}
	company, err := c.Verify(t.Context(), "41651600")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if company.RegNo != "41651600" {
		t.Errorf("RegNo = %q; want 41651600", company.RegNo)
	}
	if len(company.Roles) == 0 {
		t.Error("demo company should carry derived roles")
	}
	// The offline demo must render a complete page: VAT, ONRC identity, financials.
	if company.VATStatus != "active" {
		t.Errorf("demo VATStatus = %q; want active", company.VATStatus)
	}
	if company.RegistrationDate == nil {
		t.Error("demo company should carry a registration date")
	}
	if company.LegalForm != "SRL" {
		t.Errorf("demo LegalForm = %q; want SRL", company.LegalForm)
	}
	if len(company.Administrators) == 0 {
		t.Error("demo company should carry administrators")
	}
	if len(company.Financials) < 2 {
		t.Errorf("demo company should carry multiple financial years, got %d", len(company.Financials))
	} else {
		f := company.Financials[0]
		if f.Turnover == nil || f.NetProfit == nil || f.Employees == nil {
			t.Error("demo financial year should carry turnover/net profit/employees")
		}
	}
}

// demoANAFFinancialsBody is a representative /company/:cui/financials response
// with two years of indicators (net turnover I13, profit I18, loss I19, employees I20).
const demoANAFFinancialsBody = `{
  "success": true,
  "data": [
    {
      "year": 2024,
      "caenCode": "4791",
      "caenDescription": "Comerț cu amănuntul prin internet",
      "indicators": [
        {"code": "I13", "value": "12500000"},
        {"code": "I18", "value": "1450000"},
        {"code": "I19", "value": "0"},
        {"code": "I20", "value": "48"}
      ]
    },
    {
      "year": 2023,
      "caenCode": "4791",
      "caenDescription": "Comerț cu amănuntul prin internet",
      "indicators": [
        {"code": "I13", "value": "10200000"},
        {"code": "I18", "value": "0"},
        {"code": "I19", "value": "300000"},
        {"code": "I20", "value": "41"}
      ]
    }
  ],
  "meta": {"financialsCoveredYears": 2}
}`

// newDemoANAFTestServer serves the core /company/:cui body and, when financials
// is non-empty, the /financials body; otherwise /financials returns finStatus.
func newDemoANAFTestServer(t *testing.T, core, financials string, finStatus int) (*httptest.Server, *int) {
	t.Helper()
	var finCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/financials") {
			finCalls++
			if financials == "" {
				w.WriteHeader(finStatus)
				return
			}
			_, _ = w.Write([]byte(financials))
			return
		}
		_, _ = w.Write([]byte(core))
	}))
	return srv, &finCalls
}

func TestVerify_withFinancials(t *testing.T) {
	srv, finCalls := newDemoANAFTestServer(t, demoANAFBody, demoANAFFinancialsBody, 0)
	defer srv.Close()

	c := &DemoANAFClient{baseURL: srv.URL, httpClient: srv.Client(), fetchFinancials: true}
	company, err := c.Verify(t.Context(), "14399840")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *finCalls != 1 {
		t.Fatalf("expected 1 financials call, got %d", *finCalls)
	}
	if len(company.Financials) != 2 {
		t.Fatalf("Financials = %d years; want 2", len(company.Financials))
	}
	// Newest first.
	y0 := company.Financials[0]
	if y0.Year != 2024 {
		t.Errorf("first year = %d; want 2024 (newest first)", y0.Year)
	}
	if y0.Turnover == nil || *y0.Turnover != 12500000 {
		t.Errorf("2024 turnover = %v; want 12500000", y0.Turnover)
	}
	if y0.NetProfit == nil || *y0.NetProfit != 1450000 {
		t.Errorf("2024 net profit = %v; want 1450000", y0.NetProfit)
	}
	if y0.Employees == nil || *y0.Employees != 48 {
		t.Errorf("2024 employees = %v; want 48", y0.Employees)
	}
	y1 := company.Financials[1]
	if y1.NetProfit == nil || *y1.NetProfit != -300000 {
		t.Errorf("2023 net profit = %v; want -300000 (profit I18=0 minus loss I19=300000)", y1.NetProfit)
	}
	if y1.CAENDescription == "" {
		t.Error("2023 caen description should be populated")
	}
}

func TestVerify_richCoreFields(t *testing.T) {
	const core = `{"success":true,"data":{
		"cui":14399840,"name":"DANTE INTERNATIONAL SA","registrationNumber":"J40/372/2002",
		"registrationDate":"2002-02-15","legalForm":"SA","eFacturaRegistered":true,
		"vatRegistered":true,"vatStatus":"active","inactive":false,"caenCode":"4791"
	}}`
	srv, _ := newDemoANAFTestServer(t, core, "", http.StatusNotFound)
	defer srv.Close()

	c := &DemoANAFClient{baseURL: srv.URL, httpClient: srv.Client(), fetchFinancials: true}
	company, err := c.Verify(t.Context(), "14399840")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if company.LegalForm != "SA" {
		t.Errorf("LegalForm = %q; want SA", company.LegalForm)
	}
	if !company.EFactura {
		t.Error("EFactura should be true")
	}
	if company.RegistrationDate == nil || company.RegistrationDate.Year() != 2002 {
		t.Errorf("RegistrationDate = %v; want 2002-02-15", company.RegistrationDate)
	}
}

func TestVerify_financials404_doesNotFail(t *testing.T) {
	srv, finCalls := newDemoANAFTestServer(t, demoANAFBody, "", http.StatusNotFound)
	defer srv.Close()

	c := &DemoANAFClient{baseURL: srv.URL, httpClient: srv.Client(), fetchFinancials: true}
	company, err := c.Verify(t.Context(), "14399840")
	if err != nil {
		t.Fatalf("Verify must succeed despite financials 404, got %v", err)
	}
	if *finCalls != 1 {
		t.Fatalf("expected 1 financials call, got %d", *finCalls)
	}
	if len(company.Financials) != 0 {
		t.Errorf("Financials should be empty on 404, got %d", len(company.Financials))
	}
}

func TestVerify_financials5xx_doesNotFail(t *testing.T) {
	srv, _ := newDemoANAFTestServer(t, demoANAFBody, "", http.StatusBadGateway)
	defer srv.Close()

	c := &DemoANAFClient{baseURL: srv.URL, httpClient: srv.Client(), fetchFinancials: true}
	company, err := c.Verify(t.Context(), "14399840")
	if err != nil {
		t.Fatalf("Verify must succeed despite financials 5xx, got %v", err)
	}
	if len(company.Financials) != 0 {
		t.Errorf("Financials should be empty on 5xx, got %d", len(company.Financials))
	}
}

func TestVerify_financialsDisabled_noCall(t *testing.T) {
	srv, finCalls := newDemoANAFTestServer(t, demoANAFBody, demoANAFFinancialsBody, 0)
	defer srv.Close()

	c := &DemoANAFClient{baseURL: srv.URL, httpClient: srv.Client(), fetchFinancials: false}
	company, err := c.Verify(t.Context(), "14399840")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *finCalls != 0 {
		t.Fatalf("financials must not be called when disabled, got %d calls", *finCalls)
	}
	if len(company.Financials) != 0 {
		t.Errorf("Financials should be empty when disabled, got %d", len(company.Financials))
	}
}
