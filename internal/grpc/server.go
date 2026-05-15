package grpcserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"net"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pcmiv1 "github.com/marco-spagn/pcmi/internal/grpc/pcmiv1"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/service"
)

type memoryServer struct {
	pcmiv1.UnimplementedMemoryServiceServer
	svc *service.MemoryService
	db  *pgxpool.Pool
}

func (s *memoryServer) resolveTenant(ctx context.Context, apiKey string) (string, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if vals := md.Get("x-api-key"); len(vals) > 0 {
				apiKey = vals[0]
			}
		}
	}
	if apiKey == "" {
		return "", status.Error(codes.Unauthenticated, "missing API key")
	}
	hash := sha256.Sum256([]byte(apiKey))
	keyHash := hex.EncodeToString(hash[:])
	var tenantID string
	var active bool
	err := s.db.QueryRow(ctx, `
		SELECT tenant_id::text, is_active FROM api_keys
		WHERE key_hash = $1 AND (expires_at IS NULL OR expires_at > NOW())`,
		keyHash).Scan(&tenantID, &active)
	if err != nil || !active {
		return "", status.Error(codes.Unauthenticated, "invalid API key")
	}
	_, _ = s.db.Exec(ctx, "SELECT set_tenant_context($1::uuid)", tenantID)
	return tenantID, nil
}

func (s *memoryServer) Store(ctx context.Context, req *pcmiv1.StoreRequest) (*pcmiv1.StoreResponse, error) {
	tenantID, err := s.resolveTenant(ctx, req.GetApiKey())
	if err != nil {
		return nil, err
	}
	meta := map[string]interface{}{}
	if req.GetMetadataJson() != "" {
		_ = json.Unmarshal([]byte(req.GetMetadataJson()), &meta)
	}
	result, err := s.svc.Store(ctx, &model.StoreRequest{
		Path: req.GetPath(), Content: req.GetContent(), Metadata: meta,
	}, tenantID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "store: %v", err)
	}
	return &pcmiv1.StoreResponse{
		Id: result.Entry.ID, Status: "stored", Version: int32(result.Version),
	}, nil
}

func (s *memoryServer) Retrieve(ctx context.Context, req *pcmiv1.RetrieveRequest) (*pcmiv1.RetrieveResponse, error) {
	tenantID, err := s.resolveTenant(ctx, req.GetApiKey())
	if err != nil {
		return nil, err
	}
	limit := int(req.GetLimit())
	if limit <= 0 {
		limit = 10
	}
	result, err := s.svc.Retrieve(ctx, &model.RetrieveRequest{
		PathPrefix: req.GetPathPrefix(), Query: req.GetQuery(), Limit: limit,
	}, tenantID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "retrieve: %v", err)
	}
	out := &pcmiv1.RetrieveResponse{Total: int32(result.Total)}
	for _, e := range result.Entries {
		out.Entries = append(out.Entries, &pcmiv1.RetrieveEntry{
			Id: e.ID, Path: e.Path, Content: e.Content,
			Version: int32(e.Version), RelevanceScore: e.RelevanceScore,
		})
	}
	return out, nil
}

func (s *memoryServer) Health(context.Context, *pcmiv1.HealthRequest) (*pcmiv1.HealthResponse, error) {
	return &pcmiv1.HealthResponse{Status: "ok", Version: "v1.14.0"}, nil
}

// Start launches the gRPC server on GRPC_PORT (default 50051).
func Start(db *pgxpool.Pool, memSvc *service.MemoryService) {
	port := os.Getenv("GRPC_PORT")
	if port == "" {
		port = "50051"
	}
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Printf("gRPC listen failed: %v", err)
		return
	}
	srv := grpc.NewServer()
	pcmiv1.RegisterMemoryServiceServer(srv, &memoryServer{svc: memSvc, db: db})
	go func() {
		log.Printf("✅ PCMI gRPC server on :%s (Store/Retrieve/Health)", port)
		if err := srv.Serve(lis); err != nil {
			log.Printf("gRPC serve: %v", err)
		}
	}()
}

// ResolveTenantForTest exposes tenant resolution for integration tests.
func ResolveTenantForTest(ctx context.Context, db *pgxpool.Pool, apiKey string) (string, error) {
	s := &memoryServer{db: db}
	return s.resolveTenant(ctx, apiKey)
}
