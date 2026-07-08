package store

import "strings"

// isUniqueViolation reports whether err is a SQLite UNIQUE constraint failure.
// We match on the message text so we don't have to depend on the driver's
// concrete error type.
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
