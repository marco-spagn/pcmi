package model

// CompactMemoryRequest trims superseded versions for a single path, keeping the newest N closed rows.
type CompactMemoryRequest struct {
	Path           string `json:"path"`
	KeepSuperseded int    `json:"keep_superseded"`
}

// CompactMemoryResponse reports how many superseded rows were deleted.
type CompactMemoryResponse struct {
	Path           string `json:"path"`
	DeletedCount   int    `json:"deleted_count"`
	KeepSuperseded int    `json:"keep_superseded"`
}
