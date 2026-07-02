package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/marco-spagn/pcmi/internal/config"
	"github.com/marco-spagn/pcmi/internal/extraction"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/repository"
)

// LLMCompleter is the minimal surface needed for attribute extraction.
type LLMCompleter interface {
	Complete(ctx context.Context, systemPrompt string, userMessages []string) (string, error)
	IsConfigured() bool
}

// ExtractionService runs profile matching, LLM extraction, and metadata merge.
type ExtractionService struct {
	profiles *repository.ExtractionRepository
	memories repository.MemoryRepo
	llm      LLMCompleter
	model    string
	enabled  bool
}

func NewExtractionService(profiles *repository.ExtractionRepository, memories repository.MemoryRepo, llm LLMCompleter, cfg *config.Config) *ExtractionService {
	s := &ExtractionService{
		profiles: profiles,
		memories: memories,
		llm:      llm,
		model:    "gpt-4o-mini",
	}
	if cfg != nil {
		s.enabled = cfg.ExtractionEnabled
		if m := strings.TrimSpace(cfg.ExtractionModel); m != "" {
			s.model = m
		} else if m := strings.TrimSpace(cfg.DistillationModel); m != "" {
			s.model = m
		}
	}
	return s
}

func (s *ExtractionService) Enabled() bool {
	return s != nil && s.enabled
}

// ListProfiles returns tenant extraction profiles.
func (s *ExtractionService) ListProfiles(ctx context.Context, tenantID string) ([]repository.ExtractionProfileRow, error) {
	return s.profiles.ListProfiles(ctx, tenantID)
}

// UpsertProfile validates and stores a profile for the tenant.
func (s *ExtractionService) UpsertProfile(ctx context.Context, tenantID, profileID, pathPrefix string, profile *extraction.Profile, enabled bool) (*repository.ExtractionProfileRow, error) {
	return s.profiles.UpsertProfile(ctx, tenantID, profileID, pathPrefix, profile, enabled)
}

// DeleteProfile removes a profile.
func (s *ExtractionService) DeleteProfile(ctx context.Context, tenantID, profileID string) (bool, error) {
	return s.profiles.DeleteProfile(ctx, tenantID, profileID)
}

// GetExtraction reads pcmi_extract from the current memory row.
func (s *ExtractionService) GetExtraction(ctx context.Context, tenantID string, memoryID int64) (*extraction.Record, *model.MemoryEntry, error) {
	entry, err := s.profiles.GetCurrentMemoryByID(ctx, tenantID, memoryID)
	if err != nil {
		return nil, nil, err
	}
	if entry == nil {
		return nil, nil, fmt.Errorf("memory not found")
	}
	rec, _ := extraction.RecordFromMetadata(metadataAsMap(entry.Metadata))
	return rec, entry, nil
}

// ExtractMemory runs (or re-runs) LLM extraction for a memory id.
func (s *ExtractionService) ExtractMemory(ctx context.Context, tenantID string, memoryID int64) (*extraction.Record, error) {
	if s == nil || s.profiles == nil || s.memories == nil {
		return nil, fmt.Errorf("extraction service not configured")
	}
	if !s.enabled {
		return nil, fmt.Errorf("extraction is disabled")
	}
	entry, err := s.profiles.GetCurrentMemoryByID(ctx, tenantID, memoryID)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, fmt.Errorf("memory not found")
	}
	return s.extractEntry(ctx, tenantID, entry, true)
}

// ExtractPath triggers extraction for the current memory at path (worker helper).
func (s *ExtractionService) ExtractPath(ctx context.Context, tenantID, path string, memoryID int64, version int) error {
	if !s.enabled {
		return nil
	}
	path = strings.TrimSpace(path)
	if extraction.ShouldSkipPath(path) {
		return nil
	}
	entry, err := s.profiles.GetCurrentMemoryByID(ctx, tenantID, memoryID)
	if err != nil {
		return err
	}
	if entry == nil {
		return nil
	}
	if entry.Path != path {
		return nil
	}
	if entry.Version != version && version > 0 {
		return nil
	}
	if _, ok := existingOkExtraction(entry); ok {
		return nil
	}
	_, err = s.extractEntry(ctx, tenantID, entry, false)
	return err
}

func (s *ExtractionService) extractEntry(ctx context.Context, tenantID string, entry *model.MemoryEntry, force bool) (*extraction.Record, error) {
	if !force {
		if rec, ok := existingOkExtraction(entry); ok {
			return rec, nil
		}
	}
	if extraction.ShouldSkipPath(entry.Path) {
		return nil, fmt.Errorf("path not eligible for extraction")
	}
	content := strings.TrimSpace(entry.Content)
	if content == "" {
		return nil, fmt.Errorf("empty content")
	}

	profile, _, err := s.profiles.MatchProfile(ctx, tenantID, entry.Path)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, fmt.Errorf("no matching extraction profile")
	}

	rec := &extraction.Record{
		ProfileID:      profile.ProfileID,
		ProfileVersion: profile.Version,
		MemoryID:       entry.ID,
		MemoryVersion:  entry.Version,
		ExtractedAt:    time.Now().UTC().Format(time.RFC3339),
		Model:          s.model,
		Status:         "llm_failed",
	}

	if s.llm == nil || !s.llm.IsConfigured() {
		rec.Error = "LLM not configured"
		_ = s.persistFailureRecord(ctx, tenantID, entry, rec, force)
		return rec, fmt.Errorf("LLM not configured")
	}

	systemPrompt := extraction.BuildSystemPrompt(profile)
	userMsg := extraction.BuildUserMessage(content, metadataAsMap(entry.Metadata))
	raw, err := s.llm.Complete(ctx, systemPrompt, []string{userMsg})
	if err != nil {
		rec.Error = err.Error()
		_ = s.persistFailureRecord(ctx, tenantID, entry, rec, force)
		return rec, fmt.Errorf("llm extract: %w", err)
	}

	parsed, err := extraction.ParseLLMResponse(raw, profile)
	if err != nil {
		rec.Status = "validation_failed"
		rec.Error = err.Error()
		_ = s.persistFailureRecord(ctx, tenantID, entry, rec, force)
		return rec, fmt.Errorf("validate extraction: %w", err)
	}

	parsed.MemoryID = entry.ID
	parsed.MemoryVersion = entry.Version
	parsed.ExtractedAt = rec.ExtractedAt
	parsed.Model = rec.Model
	parsed.ProfileID = profile.ProfileID
	parsed.ProfileVersion = profile.Version
	parsed.Status = "ok"

	if err := s.persistRecord(ctx, tenantID, entry.Path, parsed); err != nil {
		return parsed, err
	}
	return parsed, nil
}

func (s *ExtractionService) persistRecord(ctx context.Context, tenantID, path string, rec *extraction.Record) error {
	meta := extraction.RecordToMetadataMap(rec)
	_, err := s.memories.MergeCurrentMetadata(ctx, tenantID, path, meta, nil)
	return err
}

// persistFailureRecord writes failed extractions unless a successful one already
// exists for this memory version (async worker must not clobber sync POST / ok).
func (s *ExtractionService) persistFailureRecord(ctx context.Context, tenantID string, entry *model.MemoryEntry, rec *extraction.Record, force bool) error {
	if !force {
		if _, ok := existingOkExtraction(entry); ok {
			return nil
		}
	}
	return s.persistRecord(ctx, tenantID, entry.Path, rec)
}

func existingOkExtraction(entry *model.MemoryEntry) (*extraction.Record, bool) {
	rec, ok := extraction.RecordFromMetadata(metadataAsMap(entry.Metadata))
	if !ok || rec == nil || rec.Status != "ok" {
		return nil, false
	}
	if rec.MemoryID != entry.ID || rec.MemoryVersion != entry.Version {
		return nil, false
	}
	return rec, true
}

func metadataAsMap(raw any) map[string]interface{} {
	if raw == nil {
		return nil
	}
	if m, ok := raw.(map[string]interface{}); ok {
		return m
	}
	return nil
}
