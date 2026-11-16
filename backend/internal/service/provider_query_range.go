package service

import "time"

// ValidateFinanceRange bounds read-only supplier and procurement queries.
func ValidateFinanceRange(from, until time.Time) bool {
	return !from.IsZero() && until.After(from) && until.Sub(from) <= 31*24*time.Hour
}
