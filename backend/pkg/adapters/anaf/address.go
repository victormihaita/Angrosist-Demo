package anaf

import "strings"

// ParsedAddress holds the components extracted from an ANAF flat address string.
type ParsedAddress struct {
	City   string
	County string
	Street string
}

// cityPrefixes are stripped from the first address segment before title-casing.
var cityPrefixes = []string{
	"MUNICIPIUL ", "ORAȘ ", "ORAŞ ", "COMUNĂ ", "COMUNA ", "SAT ", "SECTOR ",
}

// countyPrefixes are stripped from the second address segment before title-casing.
var countyPrefixes = []string{
	"JUDEȚUL ", "JUDETUL ", "JUDEȚ ", "JUDET ", "JUD. ", "JUD ",
}

// ParseAddress splits an ANAF "adresa" field into city, county, and street.
// ANAF format: "MUNICIPIUL CLUJ-NAPOCA, JUD. CLUJ, STR. XYZ, NR. 1, AP. 2"
// Bucharest sectors omit the JUD segment: "MUNICIPIUL BUCUREȘTI, SECTOR 1, STR. XYZ, NR. 1"
func ParseAddress(raw string) ParsedAddress {
	// Split into at most 3 chunks so the street preserves internal commas.
	parts := strings.SplitN(raw, ", ", 3)

	var city, county, street string

	if len(parts) > 0 {
		city = toTitle(stripAnyPrefix(strings.TrimSpace(parts[0]), cityPrefixes))
	}
	if len(parts) > 1 {
		seg := strings.TrimSpace(parts[1])
		stripped := stripAnyPrefix(seg, countyPrefixes)
		if stripped != seg {
			// It was a county segment — title-case it.
			county = toTitle(stripped)
		} else {
			// No county prefix (e.g. Bucharest "SECTOR 1") — treat as street prefix.
			street = toTitle(seg)
		}
	}
	if len(parts) > 2 {
		rest := toTitle(strings.TrimSpace(parts[2]))
		if street != "" {
			street = street + ", " + rest
		} else {
			street = rest
		}
	}

	return ParsedAddress{City: city, County: county, Street: street}
}

func stripAnyPrefix(s string, prefixes []string) string {
	upper := strings.ToUpper(s)
	for _, p := range prefixes {
		if strings.HasPrefix(upper, p) {
			return s[len(p):]
		}
	}
	return s
}
