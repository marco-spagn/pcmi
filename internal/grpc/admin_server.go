package grpcserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	pcmiv1 "github.com/marco-spagn/pcmi/internal/grpc/pcmiv1"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/repository"
	"github.com/marco-spagn/pcmi/internal/service"
)

type adminServer struct {
	pcmiv1.UnimplementedAdminServiceServer
	admin *service.AdminService
	db    *pgxpool.Pool
}

func newAdminServer(db *pgxpool.Pool) *adminServer {
	repo := repository.NewAdminRepository(db)
	return &adminServer{
		admin: service.NewAdminService(repo),
		db:    db,
	}
}

func (s *adminServer) requireAdmin(ctx context.Context) error {
	_, role, err := (&memoryServer{db: s.db}).resolveTenantAndRole(ctx, "")
	if err != nil {
		return err
	}
	if role != "admin" {
		return status.Error(codes.PermissionDenied, "admin role required")
	}
	return nil
}

func clampAdminLimit(l int32) int {
	if l <= 0 {
		return 50
	}
	if int(l) > 200 {
		return 200
	}
	return int(l)
}

func (s *adminServer) CreateTenant(ctx context.Context, req *pcmiv1.CreateTenantRequest) (*pcmiv1.TenantResponse, error) {
	if err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	settings := map[string]interface{}{}
	if js := strings.TrimSpace(req.GetSettingsJson()); js != "" && js != "{}" {
		if err := json.Unmarshal([]byte(js), &settings); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "settings_json: %v", err)
		}
	}
	t, err := s.admin.CreateTenant(ctx, &model.TenantCreateRequest{
		Slug:     req.GetSlug(),
		Name:     req.GetName(),
		Settings: settings,
	})
	if err != nil {
		if strings.Contains(err.Error(), "required") {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Errorf(codes.Internal, "create tenant: %v", err)
	}
	return tenantToProto(t), nil
}

func (s *adminServer) ListTenants(ctx context.Context, req *pcmiv1.ListTenantsRequest) (*pcmiv1.ListTenantsResponse, error) {
	if err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	limit := clampAdminLimit(req.GetLimit())
	var cur model.Cursor
	if strings.TrimSpace(req.GetCursor()) != "" {
		decoded, err := model.DecodeCursor(req.GetCursor())
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid cursor: "+err.Error())
		}
		cur = decoded
	}
	tenants, pageResp, err := s.admin.ListTenants(ctx, model.PageRequest{Cursor: cur, Limit: limit})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list tenants: %v", err)
	}
	out := &pcmiv1.ListTenantsResponse{
		NextCursor: pageResp.NextCursor,
		HasMore:    pageResp.HasMore,
	}
	for i := range tenants {
		out.Tenants = append(out.Tenants, tenantToProto(&tenants[i]))
	}
	return out, nil
}

func (s *adminServer) CreateAPIKey(ctx context.Context, req *pcmiv1.CreateAPIKeyRequest) (*pcmiv1.APIKeyResponse, error) {
	if err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	var exp *time.Time
	if s := strings.TrimSpace(req.GetExpiresAt()); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "expires_at: %v", err)
		}
		exp = &t
	}
	role := strings.TrimSpace(req.GetRole())
	if role == "" {
		role = "user"
	}
	resp, err := s.admin.CreateAPIKey(ctx, &model.APIKeyCreateRequest{
		TenantID:  req.GetTenantId(),
		Name:      req.GetName(),
		Role:      role,
		ExpiresAt: exp,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create api key: %v", err)
	}
	return apiKeyToProto(resp), nil
}

func (s *adminServer) RotateAPIKey(ctx context.Context, req *pcmiv1.RotateAPIKeyRequest) (*pcmiv1.APIKeyResponse, error) {
	if err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	resp, err := s.admin.RotateAPIKey(ctx, req.GetId(), req.GetName(), "/grpc/AdminService/RotateAPIKey", "POST", "")
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Errorf(codes.Internal, "rotate api key: %v", err)
	}
	return apiKeyToProto(resp), nil
}

func (s *adminServer) ListAPIKeys(ctx context.Context, req *pcmiv1.ListAPIKeysRequest) (*pcmiv1.ListAPIKeysResponse, error) {
	if err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.GetTenantId()) == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	limit := clampAdminLimit(req.GetLimit())
	var cur model.Cursor
	if strings.TrimSpace(req.GetCursor()) != "" {
		decoded, err := model.DecodeCursor(req.GetCursor())
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid cursor: "+err.Error())
		}
		cur = decoded
	}
	keys, pageResp, err := s.admin.ListAPIKeys(ctx, req.GetTenantId(), model.PageRequest{Cursor: cur, Limit: limit})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list api keys: %v", err)
	}
	out := &pcmiv1.ListAPIKeysResponse{
		NextCursor: pageResp.NextCursor,
		HasMore:    pageResp.HasMore,
	}
	for _, row := range keys {
		sum, mapErr := apiKeySummaryFromMap(row)
		if mapErr != nil {
			return nil, status.Errorf(codes.Internal, "list api keys: %v", mapErr)
		}
		if sum.TenantId == "" {
			sum.TenantId = req.GetTenantId()
		}
		out.Keys = append(out.Keys, sum)
	}
	return out, nil
}

func tenantToProto(t *model.TenantResponse) *pcmiv1.TenantResponse {
	if t == nil {
		return &pcmiv1.TenantResponse{}
	}
	out := &pcmiv1.TenantResponse{
		Id:   t.ID,
		Slug: t.Slug,
		Name: t.Name,
	}
	if !t.CreatedAt.IsZero() {
		out.CreatedAt = timestamppb.New(t.CreatedAt)
	}
	return out
}

func apiKeyToProto(r *model.APIKeyRotateResponse) *pcmiv1.APIKeyResponse {
	if r == nil {
		return &pcmiv1.APIKeyResponse{}
	}
	out := &pcmiv1.APIKeyResponse{
		Id:       r.ID,
		TenantId: r.TenantID,
		Name:     r.Name,
		Role:     r.Role,
		ApiKey:   r.APIKey,
	}
	if r.ExpiresAt != nil {
		out.ExpiresAt = r.ExpiresAt.Format(time.RFC3339)
	}
	return out
}

func apiKeySummaryFromMap(row map[string]interface{}) (*pcmiv1.APIKeySummary, error) {
	id, _ := row["id"].(string)
	if id == "" {
		id = fmt.Sprint(row["id"])
	}
	name, _ := row["name"].(string)
	role, _ := row["role"].(string)
	active, _ := row["is_active"].(bool)
	tenantID, _ := row["tenant_id"].(string)

	sum := &pcmiv1.APIKeySummary{
		Id:       id,
		TenantId: tenantID,
		Name:     name,
		Role:     role,
		IsActive: active,
	}
	if ts, ok := row["created_at"].(time.Time); ok && !ts.IsZero() {
		sum.CreatedAt = timestamppb.New(ts)
	}
	if exp := row["expires_at"]; exp != nil {
		switch v := exp.(type) {
		case time.Time:
			if !v.IsZero() {
				sum.ExpiresAt = v.Format(time.RFC3339)
			}
		case string:
			sum.ExpiresAt = v
		}
	}
	return sum, nil
}
