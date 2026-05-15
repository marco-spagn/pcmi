package repository

// temporalClause restricts rows to the current slice or a point-in-time view.
func temporalClause(asOfParam string) string {
	return `(
		(` + asOfParam + ` IS NULL AND valid_to IS NULL)
		OR
		(` + asOfParam + ` IS NOT NULL AND valid_from <= ` + asOfParam + ` AND (valid_to IS NULL OR valid_to > ` + asOfParam + `))
	)`
}
