package embedding

import (
	"fmt"
	"strings"
)

// knownModelDimensions maps well-known model names to their native output
// dimensions. Used to validate EMBEDDING_MODEL at startup against the
// pgvector index dimension (always 1536 in current migrations).
var knownModelDimensions = map[string]int{
	"text-embedding-3-small":  1536,
	"text-embedding-3-large":  3072,
	"text-embedding-ada-002":  1536,
	"text-embedding-ada-001":  1024,
	// llama.cpp / local models that expose dimension in the name
}

// DBVectorDimension is the dimension of the VECTOR(N) column in memory_entries.
// Must match migration 001_init.sql / 006_fts_hybrid.sql.
const DBVectorDimension = 1536

// ValidateModelDimension returns an error when the configured model's known
// native dimension does not match DBVectorDimension.
//
// FIX-8: without this check, using text-embedding-3-large (3072d) causes a
// silent pgvector dimension mismatch error at store time. The error message
// "expected 1536 dimensions, not 3072" is opaque and only surfaces at
// runtime during the first embedding write, not at startup.
//
// Models not in knownModelDimensions are allowed through (unknown custom
// models) with a logged warning only.
func ValidateModelDimension(model string) error {
	model = strings.TrimSpace(model)
	dim, known := knownModelDimensions[model]
	if !known {
		// Unknown model — can't validate; caller logs a warning.
		return nil
	}
	if dim != DBVectorDimension {
		return fmt.Errorf(
			"embedding model %q has %d dimensions but the database "+
				"vector column is VECTOR(%d); either change EMBEDDING_MODEL "+
				"to a %d-dimensional model (e.g. text-embedding-3-small) or "+
				"recreate the pgvector index with the correct dimension",
			model, dim, DBVectorDimension, DBVectorDimension,
		)
	}
	return nil
}
