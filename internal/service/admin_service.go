package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/repository"
)

type AdminService struct {
	repo *repository.AdminRepository
}

func NewAdminService(repo *repository.AdminRepository) *AdminService {
	return &AdminService{repo: repo}
}

func (s *AdminService) CreateTenant(ctx context.Context, req *model.TenantCreateRequest) (*model.TenantResponse, error) {
	slug := strings.TrimSpace(req.Slug)
	name := strings.TrimSpace(req.Name)
	if slug == "" || name == "" {
		return nil, fmt.Errorf("slug and name are required")
	}
	return s.repo.CreateTenant(ctx, slug, name, req.Settings)
}

func (s *AdminService) ListTenants(ctx context.Context, limit int) ([]model.TenantResponse, error) {
	return s.repo.ListTenants(ctx, limit)
}

func (s *AdminService) ListAPIKeys(ctx context.Context, tenantID string, limit int) ([]map[string]interface{}, error) {
	return s.repo.ListAPIKeys(ctx, tenantID, limit)
}

func (s *AdminService) RotateAPIKey(ctx context.Context, keyID, name string) (*model.APIKeyRotateResponse, error) {
	raw, hash, err := generateAPIKey()
	if err != nil {
		return nil, err
	}
	resp, err := s.repo.RotateAPIKey(ctx, keyID, hash, name)
	if err != nil {
		return nil, err
	}
	resp.APIKey = raw
	return resp, nil
}

func (s *AdminService) CreateAPIKey(ctx context.Context, req *model.APIKeyCreateRequest) (*model.APIKeyRotateResponse, error) {
	raw, hash, err := generateAPIKey()
	if err != nil {
		return nil, err
	}
	var exp *string
	resp, err := s.repo.CreateAPIKey(ctx, req.TenantID, hash, req.Name, req.Role, exp)
	if err != nil {
		return nil, err
	}
	resp.APIKey = raw
	return resp, nil
}

func generateAPIKey() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	raw = "pcmi_" + hex.EncodeToString(b)
	h := sha256.Sum256([]byte(raw))
	hash = hex.EncodeToString(h[:])
	return raw, hash, nil
}
