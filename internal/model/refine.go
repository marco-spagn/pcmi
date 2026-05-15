package model

// RefineRequest queues asynchronous distillation for a path prefix.
type RefineRequest struct {
	PathPrefix string `json:"path_prefix"`
}

// RefineResponse acknowledges a refine job was queued.
type RefineResponse struct {
	Status     string `json:"status"`
	PathPrefix string `json:"path_prefix"`
}
