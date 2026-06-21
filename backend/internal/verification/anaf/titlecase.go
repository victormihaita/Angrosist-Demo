package anaf

import (
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var titleCaser = cases.Title(language.Romanian)

// toTitle converts an ALL-CAPS Romanian string to Title Case.
// Uses golang.org/x/text so diacritics (Ș Ț Ă Î Â) are handled correctly.
func toTitle(s string) string {
	return titleCaser.String(s)
}
