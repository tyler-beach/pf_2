package domain

import "strings"

// NormalizeCustomerName is a pure domain helper with no infrastructure dependencies.
func NormalizeCustomerName(name string) string {
	return strings.TrimSpace(name)
}
