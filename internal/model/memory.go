package model

type MemoryEntry struct {
	ID        int64     `json:"id"`
	Path      string    `json:"path"`
	Content   string    `json:"content"`
	Metadata  any       `json:"metadata"`
	Embedding []float32 `json:"embedding"`
	ValidFrom string    `json:"valid_from"`
	ValidTo   *string   `json:"valid_to"`
	Version   int       `json:"version"`
	TenantID  string    `json:"tenant_id"`
}

type RetrieveQuery struct {
	PathPrefix string `json:"path_prefix"`
	Query      string `json:"query"`
	Limit      int    `json:"limit"`
}
