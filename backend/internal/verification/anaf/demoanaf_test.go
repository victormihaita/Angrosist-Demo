package anaf

import (
	"net/http"
	"net/http/httptest"
	"reflect"
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
}
