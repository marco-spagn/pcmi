package model

// MemoryLineageResponse lists version history and derived distilled knowledge.
type MemoryLineageResponse struct {
	EntryID   int64                  `json:"entry_id"`
	Path      string                 `json:"path"`
	Versions  []MemoryEntry          `json:"versions"`
	Distilled []DistilledLineageItem `json:"distilled"`
}

// DistilledLineageResponse links distilled knowledge to source memories.
type DistilledLineageResponse struct {
	Distilled DistilledLineageItem `json:"distilled"`
	Sources   []MemoryEntry        `json:"sources"`
}

// DistilledLineageItem is a distilled knowledge row in lineage views.
type DistilledLineageItem struct {
	ID              int64    `json:"id"`
	Path            string   `json:"path"`
	Summary         string   `json:"summary"`
	Version         int      `json:"version"`
	SourceEntryIDs  []int64  `json:"source_entry_ids"`
	ConfidenceScore float64  `json:"confidence_score"`
}
