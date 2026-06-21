package httputil

import "testing"

func TestResolveCORSOrigin(t *testing.T) {
	cases := []struct {
		name      string
		allowed   []string
		origin    string
		wantValue string
		wantVary  bool
	}{
		{
			name:      "wildcard allows any origin",
			allowed:   []string{"*"},
			origin:    "https://anything.example",
			wantValue: "*",
			wantVary:  false,
		},
		{
			name:      "explicit allowlist echoes a matching origin",
			allowed:   []string{"https://app.eurointermed.ro", "https://dash.eurointermed.ro"},
			origin:    "https://dash.eurointermed.ro",
			wantValue: "https://dash.eurointermed.ro",
			wantVary:  true,
		},
		{
			name:      "explicit allowlist blocks an unknown origin",
			allowed:   []string{"https://app.eurointermed.ro"},
			origin:    "https://evil.example",
			wantValue: "",
			wantVary:  true,
		},
		{
			name:      "explicit allowlist with empty origin emits no header",
			allowed:   []string{"https://app.eurointermed.ro"},
			origin:    "",
			wantValue: "",
			wantVary:  true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			value, vary := resolveCORSOrigin(c.allowed, c.origin)
			if value != c.wantValue {
				t.Errorf("value = %q; want %q", value, c.wantValue)
			}
			if vary != c.wantVary {
				t.Errorf("vary = %v; want %v", vary, c.wantVary)
			}
		})
	}
}
