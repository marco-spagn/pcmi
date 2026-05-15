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

// scopeFilters adds optional cross-agent and embedding-space predicates.
// Empty string parameters mean "no filter" (avoids untyped NULL uuid bind issues).
func scopeFilters(agentParam, spaceParam string) string {
	return `($` + agentParam + ` = '' OR source_agent_id = $` + agentParam + `::uuid)
		  AND ($` + spaceParam + ` = '' OR embedding_space = $` + spaceParam + `)`
}

// tagFilters restricts rows by tag overlap (any) or containment (all).
// tagsParam is a text[] placeholder; matchParam is '' | 'any' | 'all'.
func tagFilters(tagsParam, matchParam string) string {
	return `(
		cardinality($` + tagsParam + `::text[]) = 0
		OR (
			($` + matchParam + ` IN ('', 'any') AND tags && $` + tagsParam + `::text[])
			OR ($` + matchParam + ` = 'all' AND tags @> $` + tagsParam + `::text[])
		)
	)`
}
