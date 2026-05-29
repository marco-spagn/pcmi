package grpcserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"net"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/marco-spagn/pcmi/internal/config"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/marco-spagn/pcmi/internal/event"
	pcmiv1 "github.com/marco-spagn/pcmi/internal/grpc/pcmiv1"
	"github.com/marco-spagn/pcmi/internal/metrics"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/repository"
	"github.com/marco-spagn/pcmi/internal/service"
	"github.com/marco-spagn/pcmi/internal/version"
)

type memoryServer struct {
	pcmiv1.UnimplementedMemoryServiceServer
	svc         *service.MemoryService
	db          *pgxpool.Pool
	readDB      *pgxpool.Pool
	eventSvc    *service.EventService
	linksRepo   *repository.LinksRepository
	statsRepo   *repository.StatsRepository
	lineageRepo *repository.LineageRepository
	auditRepo   *repository.AuditRepository
	summarize   *service.SummarizeService
	memRepo     *repository.MemoryRepository
}

func (s *memoryServer) resolveTenantAndRole(ctx context.Context, apiKey string) (tenantID string, role string, err error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if vals := md.Get("x-api-key"); len(vals) > 0 {
				apiKey = vals[0]
			}
		}
	}
	if apiKey == "" {
		return "", "", status.Error(codes.Unauthenticated, "missing API key")
	}
	hash := sha256.Sum256([]byte(apiKey))
	keyHash := hex.EncodeToString(hash[:])
	var active bool
	err = s.db.QueryRow(ctx, `
		SELECT tenant_id::text, role, is_active FROM api_keys
		WHERE key_hash = $1
		  AND is_active = true
		  AND (expires_at IS NULL OR expires_at > NOW())
		  AND (
		        rotated_to IS NULL
		        OR (rotation_grace_ends_at IS NOT NULL AND rotation_grace_ends_at > NOW())
		      )`,
		keyHash).Scan(&tenantID, &role, &active)
	if err != nil || !active {
		return "", "", status.Error(codes.Unauthenticated, "invalid API key")
	}
	// BUG-FIX-7: gRPC path had the same silent-discard bug as the HTTP middleware
	// (BUG-FIX-5). A failed set_tenant_context leaves RLS inactive on a pooled
	// connection, potentially exposing another tenant's data.
	if _, err := s.db.Exec(ctx, "SELECT set_tenant_context($1::uuid)", tenantID); err != nil {
		return "", "", status.Errorf(codes.Unavailable, "tenant context unavailable: %v", err)
	}
	return tenantID, role, nil
}

func requireWriteRole(role string) error {
	if role == "readonly" {
		return status.Error(codes.PermissionDenied, "read-only API key cannot perform write operations")
	}
	return nil
}

func mapSvcValidationErr(context string, err error) error {
	msg := err.Error()
	if strings.Contains(msg, "required") || strings.Contains(msg, "maximum") {
		return status.Error(codes.InvalidArgument, msg)
	}
	return status.Errorf(codes.Internal, "%s: %v", context, err)
}

func (s *memoryServer) Store(ctx context.Context, req *pcmiv1.StoreRequest) (*pcmiv1.StoreResponse, error) {
	tenantID, role, err := s.resolveTenantAndRole(ctx, req.GetApiKey())
	if err != nil {
		return nil, err
	}
	if err := requireWriteRole(role); err != nil {
		return nil, err
	}
	sr, err := storeProtoToModel(req)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	result, err := s.svc.Store(ctx, &sr, tenantID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "store: %v", err)
	}
	metrics.IncStore()
	return storeResultToProto(result.Entry.ID, result.Version, result.SupersededID), nil
}

func (s *memoryServer) BatchStore(ctx context.Context, req *pcmiv1.BatchStoreRequest) (*pcmiv1.BatchStoreResponse, error) {
	tenantID, role, err := s.resolveTenantAndRole(ctx, req.GetApiKey())
	if err != nil {
		return nil, err
	}
	if err := requireWriteRole(role); err != nil {
		return nil, err
	}
	items, err := batchStoreProtoToModel(req.GetItems())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	batch := &model.BatchStoreRequest{Items: items}
	result, err := s.svc.BatchStore(ctx, batch, tenantID)
	if err != nil {
		return nil, mapSvcValidationErr("batch store", err)
	}
	return batchStoreModelToProto(result), nil
}

func (s *memoryServer) Retrieve(ctx context.Context, req *pcmiv1.RetrieveRequest) (*pcmiv1.RetrieveResponse, error) {
	tenantID, _, err := s.resolveTenantAndRole(ctx, req.GetApiKey())
	if err != nil {
		return nil, err
	}
	mr, err := retrieveProtoToModel(req)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	result, err := s.svc.Retrieve(ctx, &mr, tenantID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "retrieve: %v", err)
	}
	out := &pcmiv1.RetrieveResponse{Total: int32(result.Total)}
	for i := range result.Entries {
		out.Entries = append(out.Entries, memoryEntryToProtoRetrieve(&result.Entries[i]))
	}
	metrics.IncRetrieve()
	return out, nil
}

func (s *memoryServer) BatchRetrieve(ctx context.Context, req *pcmiv1.BatchRetrieveRequest) (*pcmiv1.BatchRetrieveResponse, error) {
	tenantID, _, err := s.resolveTenantAndRole(ctx, req.GetApiKey())
	if err != nil {
		return nil, err
	}
	queries, err := batchQueriesProtoToModel(req.GetQueries())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	batch := &model.BatchRetrieveRequest{Queries: queries}
	result, err := s.svc.BatchRetrieve(ctx, batch, tenantID)
	if err != nil {
		return nil, mapSvcValidationErr("batch retrieve", err)
	}
	return batchRetrieveModelToProto(result), nil
}

func (s *memoryServer) RetrieveStream(req *pcmiv1.RetrieveRequest, stream pcmiv1.MemoryService_RetrieveStreamServer) error {
	ctx := stream.Context()
	tenantID, _, err := s.resolveTenantAndRole(ctx, req.GetApiKey())
	if err != nil {
		return err
	}
	mr, err := retrieveProtoToModel(req)
	if err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	result, err := s.svc.Retrieve(ctx, &mr, tenantID)
	if err != nil {
		return status.Errorf(codes.Internal, "retrieve: %v", err)
	}
	if err := stream.Send(&pcmiv1.RetrieveStreamMsg{
		Msg: &pcmiv1.RetrieveStreamMsg_Header{
			Header: &pcmiv1.RetrieveStreamHeader{Total: int32(result.Total)},
		},
	}); err != nil {
		return err
	}
	for i := range result.Entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		entry := memoryEntryToProtoRetrieve(&result.Entries[i])
		if err := stream.Send(&pcmiv1.RetrieveStreamMsg{
			Msg: &pcmiv1.RetrieveStreamMsg_Entry{Entry: entry},
		}); err != nil {
			return err
		}
	}
	metrics.IncRetrieve()
	return nil
}

func (s *memoryServer) GetMemory(ctx context.Context, req *pcmiv1.GetMemoryRequest) (*pcmiv1.GetMemoryResponse, error) {
	tenantID, _, err := s.resolveTenantAndRole(ctx, req.GetApiKey())
	if err != nil {
		return nil, err
	}
	path, ver, asOf, err := getMemoryProtoToParams(req)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	entry, err := s.svc.GetByPath(ctx, tenantID, path, ver, asOf)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Errorf(codes.Internal, "get memory: %v", err)
	}
	return &pcmiv1.GetMemoryResponse{Entry: memoryEntryToProtoRetrieve(entry)}, nil
}

func (s *memoryServer) Compact(ctx context.Context, req *pcmiv1.CompactRequest) (*pcmiv1.CompactResponse, error) {
	tenantID, role, err := s.resolveTenantAndRole(ctx, req.GetApiKey())
	if err != nil {
		return nil, err
	}
	if err := requireWriteRole(role); err != nil {
		return nil, err
	}
	cr, err := compactProtoToModel(req)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	out, err := s.svc.Compact(ctx, tenantID, &cr)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "compact: %v", err)
	}
	return compactModelToProto(out), nil
}

func (s *memoryServer) Health(context.Context, *pcmiv1.HealthRequest) (*pcmiv1.HealthResponse, error) {
	return &pcmiv1.HealthResponse{Status: "ok", Version: version.Tag}, nil
}

func (s *memoryServer) Ready(ctx context.Context, _ *pcmiv1.ReadyRequest) (*pcmiv1.ReadyResponse, error) {
	dbOK := s.db.Ping(ctx) == nil
	redisOK := event.RedisClient != nil && event.RedisClient.Ping(ctx).Err() == nil
	st := "not_ready"
	if dbOK && redisOK {
		st = "ready"
	}
	return &pcmiv1.ReadyResponse{
		Status:     st,
		DatabaseOk: dbOK,
		RedisOk:    redisOK,
		Version:    version.Tag,
	}, nil
}

func (s *memoryServer) setTenant(ctx context.Context, tenantID string) error {
	_, err := s.db.Exec(ctx, "SELECT set_tenant_context($1::uuid)", tenantID)
	return err
}

// ResolveGRPCPort returns the TCP port for the gRPC server from cfg (default 50051).
func ResolveGRPCPort(cfg *config.Config) string {
	port := "50051"
	if cfg != nil && strings.TrimSpace(cfg.GRPCPort) != "" {
		port = strings.TrimSpace(cfg.GRPCPort)
	}
	return port
}

// BuildServerOptions assembles the grpc.ServerOption list for the PCMI gRPC
// server. When cfg.TLSCertFile and cfg.TLSKeyFile are both set and readable,
// grpc.Creds(tls) is appended → the listener serves TLS in-process, matching
// the existing optional TLS behaviour of the Fiber HTTP server.
//
// If only one of the two is set, or either file is unreadable, the function
// logs a warning and falls back to plain TCP so a misconfigured deployment
// doesn't deadlock the gRPC plane. Operators should validate cert+key paths
// at deploy time.
//
// PR #3 — closes the "gRPC in-process TLS" tech-debt item.
func BuildServerOptions(cfg *config.Config) []grpc.ServerOption {
	opts := []grpc.ServerOption{
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
	}
	if cfg == nil {
		return opts
	}
	cert := strings.TrimSpace(cfg.TLSCertFile)
	key := strings.TrimSpace(cfg.TLSKeyFile)
	if cert == "" && key == "" {
		return opts
	}
	if cert == "" || key == "" {
		log.Printf("gRPC TLS partially configured (cert=%q, key=%q) — falling back to plain TCP", cert, key)
		return opts
	}
	creds, err := credentials.NewServerTLSFromFile(cert, key)
	if err != nil {
		log.Printf("gRPC TLS load failed (%v) — falling back to plain TCP", err)
		return opts
	}
	log.Printf("gRPC TLS enabled (cert=%s)", cert)
	return append(opts, grpc.Creds(creds))
}

// Start launches the gRPC server on cfg.GRPCPort (default 50051).
func Start(dbWrite, dbRead *pgxpool.Pool, memSvc *service.MemoryService, cfg *config.Config) {
	if dbRead == nil {
		dbRead = dbWrite
	}
	memRepo := repository.NewMemoryRepository(dbWrite, dbRead)
	port := ResolveGRPCPort(cfg)
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Printf("gRPC listen failed: %v", err)
		return
	}
	srv := grpc.NewServer(BuildServerOptions(cfg)...)
	memSrv := &memoryServer{
		svc:         memSvc,
		db:          dbWrite,
		readDB:      dbRead,
		eventSvc:    service.NewEventService(repository.NewEventRepository(dbWrite)),
		linksRepo:   repository.NewLinksRepository(dbWrite, dbRead),
		statsRepo:   repository.NewStatsRepository(dbWrite, dbRead),
		lineageRepo: repository.NewLineageRepository(dbWrite, dbRead),
		auditRepo:   repository.NewAuditRepository(dbWrite),
		summarize:   service.NewSummarizeService(memRepo, cfg),
		memRepo:     memRepo,
	}
	pcmiv1.RegisterMemoryServiceServer(srv, memSrv)
	pcmiv1.RegisterAdminServiceServer(srv, newAdminServer(dbWrite))
	pcmiv1.RegisterMetricsServiceServer(srv, newMetricsServer(dbWrite))
	go func() {
		log.Printf("PCMI gRPC server on :%s (MemoryService + AdminService + MetricsService)", port)
		if err := srv.Serve(lis); err != nil {
			log.Printf("gRPC serve: %v", err)
		}
	}()
}

// ResolveTenantForTest exposes tenant resolution for integration tests.
func ResolveTenantForTest(ctx context.Context, db *pgxpool.Pool, apiKey string) (tenantID string, role string, err error) {
	s := &memoryServer{db: db}
	return s.resolveTenantAndRole(ctx, apiKey)
}
