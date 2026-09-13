// Package findingid orders ids like f-001 so that f-999 sorts before f-1000.
package findingid

import (
	"cmp"
	"strings"
)

// Compare orders by prefix, then by numeric suffix; an id without a numeric suffix sorts after every id with one.
// Equal numbers written with different zero padding fall back to string order so the order stays total.
func Compare(a, b string) int {
	pa, na, oka := split(a)
	pb, nb, okb := split(b)
	if c := cmp.Compare(pa, pb); c != 0 {
		return c
	}
	switch {
	case oka && !okb:
		return -1
	case !oka && okb:
		return 1
	case oka && okb:
		if c := cmp.Compare(len(na), len(nb)); c != 0 {
			return c
		}
		if c := cmp.Compare(na, nb); c != 0 {
			return c
		}
	}
	return cmp.Compare(a, b)
}

// split returns the suffix without leading zeros, compared by length then bytes so any number of digits fits.
func split(id string) (prefix, digits string, ok bool) {
	i := strings.LastIndexByte(id, '-')
	prefix, suffix := id[:i+1], id[i+1:]
	if suffix == "" || strings.Trim(suffix, "0123456789") != "" {
		return prefix, "", false
	}
	return prefix, strings.TrimLeft(suffix, "0"), true
}
