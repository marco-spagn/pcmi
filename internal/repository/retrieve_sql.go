package repository

// temporalClause restricts rows to the current slice or a point-in-time view.
// asOfParam is a query placeholder such as "$3"; a timestamptz cast is applied for NULL inference.
func temporalClause(asOfParam string) string {
	asOf := asOfParam + "::timestamptz"
	return `(
		(` + asOf + ` IS NULL AND valid_to IS NULL)
		OR
		(` + asOf + ` IS NOT NULL AND valid_from <= ` + asOf + ` AND (valid_to IS NULL OR valid_to > ` + asOf + `))
	)`
}
