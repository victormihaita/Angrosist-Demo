package caen

import (
	"reflect"
	"testing"
)

func TestRolesForCAEN(t *testing.T) {
	tests := []struct {
		name       string
		primary    string
		authorized []string
		want       []string
	}{
		{
			name:    "wholesale 46 -> distributor+wholesaler+buyer",
			primary: "4690",
			want:    []string{"buyer", "distributor", "wholesaler"},
		},
		{
			name:    "retail 47 -> retailer+buyer",
			primary: "4711",
			want:    []string{"buyer", "retailer"},
		},
		{
			name:    "food manufacturing 10 -> processor+producer+buyer",
			primary: "1071",
			want:    []string{"buyer", "processor", "producer"},
		},
		{
			name:    "restaurant 56 -> horeca+buyer",
			primary: "5610",
			want:    []string{"buyer", "horeca"},
		},
		{
			name:    "transport 49 -> distributor+buyer",
			primary: "4941",
			want:    []string{"buyer", "distributor"},
		},
		{
			name:       "primary plus authorized union",
			primary:    "4711", // retailer
			authorized: []string{"4690", "5610"},
			want:       []string{"buyer", "distributor", "horeca", "retailer", "wholesaler"},
		},
		{
			name:    "unknown division -> buyer only",
			primary: "9999",
			want:    []string{"buyer"},
		},
		{
			name:    "empty primary -> buyer only",
			primary: "",
			want:    []string{"buyer"},
		},
		{
			name:    "RO-prefixed / messy code still divides",
			primary: " 47.11 ",
			want:    []string{"buyer", "retailer"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RolesForCAEN(tt.primary, tt.authorized...)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("RolesForCAEN(%q, %v) = %v; want %v", tt.primary, tt.authorized, got, tt.want)
			}
		})
	}
}

func TestDivision(t *testing.T) {
	cases := map[string]string{
		"4690":  "46",
		"47":    "47",
		"4":     "",
		"":      "",
		"46.90": "46",
		" 10x":  "10",
		"abc":   "",
	}
	for in, want := range cases {
		if got := division(in); got != want {
			t.Errorf("division(%q) = %q; want %q", in, got, want)
		}
	}
}
