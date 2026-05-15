package model

import "time"

// RollbackRequest restores a path to a prior version (by version number or point in time).
type RollbackRequest struct {
	Path    string     `json:"path"`
	Version *int       `json:"version,omitempty"`
	AsOf    *time.Time `json:"as_of,omitempty"`
}

// RollbackResponse is returned after a successful temporal rollback.
type RollbackResponse struct {
	ID                 int64  `json:"id"`
	Status             string `json:"status"`
	Version            int    `json:"version"`
	RestoredFromVersion int    `json:"restored_from_version"`
	SupersededID       *int64 `json:"superseded_id,omitempty"`
}
