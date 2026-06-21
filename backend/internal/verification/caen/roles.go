// Package caen maps Romanian CAEN Rev.2 activity codes to the platform's
// company role set. It is pure (no I/O, no infra imports) so it is trivially
// unit-testable and can be reused by the B2B directory and Phase-2 matching.
//
// The mapping is intentionally conservative: a code only contributes a role
// when the CAEN division clearly implies that commercial function. Codes we are
// unsure about contribute nothing rather than mislabel a company. The output
// always includes the generic "buyer" role, since every qualified company in
// the funnel is, at minimum, a prospective buyer.
package caen

import "sort"

// Role values mirror the documented companies.roles[] set in the data model.
const (
	RoleDistributor = "distributor"
	RoleImporter    = "importer"
	RoleWholesaler  = "wholesaler"
	RoleRetailer    = "retailer"
	RoleHoreca      = "horeca"
	RoleProcessor   = "processor"
	RoleProducer    = "producer"
	RoleBuyer       = "buyer"
	RoleSeller      = "seller"
)

// divisionRoles maps a CAEN two-digit division prefix to the roles it implies.
// CAEN Rev.2 structure: SS = section, DD = division (first 2 digits), then
// group/class. We key on the division, which is stable and coarse enough to be
// safe. References (CAEN Rev.2 divisions):
//
//	01-03  Agriculture, forestry, fishing      -> producer
//	10-33  Manufacturing                        -> producer, processor
//	46     Wholesale trade (except vehicles)    -> wholesaler, distributor
//	45     Trade/repair of motor vehicles       -> wholesaler, retailer
//	47     Retail trade (except vehicles)       -> retailer
//	49-53  Transportation & storage             -> distributor
//	55-56  Accommodation & food service         -> horeca
var divisionRoles = map[string][]string{
	// Primary production.
	"01": {RoleProducer}, "02": {RoleProducer}, "03": {RoleProducer},
	// Manufacturing (food & beverage processors are also processors).
	"10": {RoleProducer, RoleProcessor}, "11": {RoleProducer, RoleProcessor},
	"12": {RoleProducer, RoleProcessor},
	"13": {RoleProducer, RoleProcessor}, "14": {RoleProducer, RoleProcessor},
	"15": {RoleProducer, RoleProcessor}, "16": {RoleProducer, RoleProcessor},
	"17": {RoleProducer, RoleProcessor}, "18": {RoleProducer, RoleProcessor},
	"19": {RoleProducer, RoleProcessor}, "20": {RoleProducer, RoleProcessor},
	"21": {RoleProducer, RoleProcessor}, "22": {RoleProducer, RoleProcessor},
	"23": {RoleProducer, RoleProcessor}, "24": {RoleProducer, RoleProcessor},
	"25": {RoleProducer, RoleProcessor}, "26": {RoleProducer, RoleProcessor},
	"27": {RoleProducer, RoleProcessor}, "28": {RoleProducer, RoleProcessor},
	"29": {RoleProducer, RoleProcessor}, "30": {RoleProducer, RoleProcessor},
	"31": {RoleProducer, RoleProcessor}, "32": {RoleProducer, RoleProcessor},
	"33": {RoleProducer, RoleProcessor},
	// Trade.
	"45": {RoleWholesaler, RoleRetailer},
	"46": {RoleWholesaler, RoleDistributor},
	"47": {RoleRetailer},
	// Transport & storage -> distribution function.
	"49": {RoleDistributor}, "50": {RoleDistributor}, "51": {RoleDistributor},
	"52": {RoleDistributor}, "53": {RoleDistributor},
	// HoReCa.
	"55": {RoleHoreca}, "56": {RoleHoreca},
}

// RolesForCAEN derives the role set for a company from its primary CAEN code and
// any authorized CAEN codes. It is conservative and order-stable: the result is
// the de-duplicated, sorted union of the divisions' roles plus the always-on
// "buyer" role. An empty/invalid primary with no authorized codes still yields
// ["buyer"].
func RolesForCAEN(primary string, authorized ...string) []string {
	set := map[string]struct{}{RoleBuyer: {}}

	add := func(code string) {
		div := division(code)
		if div == "" {
			return
		}
		for _, r := range divisionRoles[div] {
			set[r] = struct{}{}
		}
	}

	add(primary)
	for _, c := range authorized {
		add(c)
	}

	out := make([]string, 0, len(set))
	for r := range set {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

// division extracts the leading two-digit CAEN division from a code string,
// tolerating surrounding whitespace and codes shorter/longer than 4 digits.
// Returns "" when the input has fewer than two leading digits.
func division(code string) string {
	digits := make([]rune, 0, 2)
	for _, r := range code {
		if r < '0' || r > '9' {
			if len(digits) == 0 {
				continue // skip leading non-digits (whitespace, letters)
			}
			break
		}
		digits = append(digits, r)
		if len(digits) == 2 {
			break
		}
	}
	if len(digits) < 2 {
		return ""
	}
	return string(digits)
}
