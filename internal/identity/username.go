// Package identity owns the canonical representation used for account names.
//
// OrbitTerm treats usernames as email-style account identifiers. They must
// therefore have one stable representation at every boundary: registration,
// login, administrator provisioning, persistence, and lookup.
package identity

import "strings"

// CanonicalUsername removes surrounding whitespace and folds case. It is
// deliberately small and deterministic so existing account identifiers are
// not subject to locale-dependent transformations.
func CanonicalUsername(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}
