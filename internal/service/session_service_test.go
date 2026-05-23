package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/repository"
)

type stubMemoryRepo struct {
	storeFn func(req model.StoreRequest) (int64, int, *int64, error)
}

func (s *stubMemoryRepo) Store(_ context.Context, req model.StoreRequest, _ string) (int64, int, *int64, error) {
	if s.storeFn != nil {
		return s.storeFn(req)
	}
	return 1, 1, nil, nil
}

func (s *stubMemoryRepo) Retrieve(context.Context, model.RetrieveRequest, string, []float32) ([]model.MemoryEntry, error) {
	return nil, nil
}
func (s *stubMemoryRepo) GetByPath(context.Context, string, string, *int, *time.Time) (*model.MemoryEntry, error) {
	return nil, errors.New("not implemented")
}
func (s *stubMemoryRepo) GetHistoricalVersion(context.Context, string, string, *int, *time.Time) (*model.MemoryEntry, error) {
	return nil, errors.New("not implemented")
}
func (s *stubMemoryRepo) ExportMemories(context.Context, string, string, int, bool) ([]model.MemoryEntry, error) {
	return nil, nil
}
func (s *stubMemoryRepo) CompactPathHistory(context.Context, string, string, int) (int, error) {
	return 0, nil
}
func (s *stubMemoryRepo) UpdateImportance(context.Context, string, string, float64) error {
	return nil
}

func sessionRepoWithActiveSession(t *testing.T, mock pgxmock.PgxPoolIface, tenantID, sessionID string) *repository.SessionRepository {
	t.Helper()
	started := time.Now().UTC()
	meta := []byte(`{}`)
	rows := pgxmock.NewRows([]string{
		"id", "tenant_id", "agent_id", "metadata", "started_at", "ended_at",
	}).AddRow(sessionID, tenantID, nil, meta, started, nil)
	mock.ExpectQuery(`FROM agent_sessions`).
		WithArgs(sessionID, tenantID).
		WillReturnRows(rows)
	return repository.NewSessionRepositoryFromDB(mock, mock)
}

func TestSessionService_Create_nilRequest(t *testing.T) {
	t.Parallel()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mock.Close)

	tenantID := uuid.New().String()
	sessionID := uuid.New().String()
	started := time.Now().UTC()
	meta := []byte(`{}`)
	mock.ExpectQuery(`INSERT INTO agent_sessions`).
		WithArgs(tenantID, pgxmock.AnyArg(), meta).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "tenant_id", "agent_id", "metadata", "started_at", "ended_at",
		}).AddRow(sessionID, tenantID, nil, meta, started, nil))

	svc := NewSessionService(repository.NewSessionRepositoryFromDB(mock, mock), NewMemoryService(&stubMemoryRepo{}, nil))
	got, err := svc.Create(context.Background(), tenantID, nil)
	if err != nil || got.ID != sessionID {
		t.Fatalf("err=%v got=%+v", err, got)
	}
}

func TestSessionService_StoreMemory_sessionEnded(t *testing.T) {
	t.Parallel()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mock.Close)

	tenantID := uuid.New().String()
	sessionID := uuid.New().String()
	ended := time.Now().UTC()
	meta := []byte(`{}`)
	mock.ExpectQuery(`FROM agent_sessions`).
		WithArgs(sessionID, tenantID).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "tenant_id", "agent_id", "metadata", "started_at", "ended_at",
		}).AddRow(sessionID, tenantID, nil, meta, ended.Add(-time.Hour), &ended))

	svc := NewSessionService(repository.NewSessionRepositoryFromDB(mock, mock), NewMemoryService(&stubMemoryRepo{}, nil))
	_, err = svc.StoreMemory(context.Background(), tenantID, sessionID, &model.SessionStoreMemoryRequest{
		Path: "note", Content: "x",
	})
	if err == nil || err.Error() != "session ended" {
		t.Fatalf("got err=%v", err)
	}
}

func TestSessionService_StoreMemory_success(t *testing.T) {
	t.Parallel()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mock.Close)

	tenantID := uuid.New().String()
	sessionID := uuid.New().String()
	_ = sessionRepoWithActiveSession(t, mock, tenantID, sessionID)

	mem := &stubMemoryRepo{
		storeFn: func(req model.StoreRequest) (int64, int, *int64, error) {
			if req.Metadata["session_id"] != sessionID {
				t.Fatalf("session_id=%v", req.Metadata["session_id"])
			}
			return 42, 1, nil, nil
		},
	}
	svc := NewSessionService(repository.NewSessionRepositoryFromDB(mock, mock), NewMemoryService(mem, nil))
	res, err := svc.StoreMemory(context.Background(), tenantID, sessionID, &model.SessionStoreMemoryRequest{
		Path: "note", Content: "stored",
	})
	if err != nil || res.Entry.ID != 42 {
		t.Fatalf("err=%v res=%+v", err, res)
	}
}

func TestSessionService_ListMemories(t *testing.T) {
	t.Parallel()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mock.Close)

	tenantID := uuid.New().String()
	sessionID := uuid.New().String()
	_ = sessionRepoWithActiveSession(t, mock, tenantID, sessionID)

	now := time.Now().UTC()
	memRows := pgxmock.NewRows([]string{
		"id", "tenant_id", "path", "content", "metadata", "tags", "embedding_model",
		"embedding_space", "version", "valid_from", "valid_to", "source_agent_id",
		"created_at", "importance", "access_count", "last_accessed_at",
	}).AddRow(int64(1), tenantID, "sessions.x", "c", []byte(`{}`), []string{}, "unspecified", "default",
		1, now, nil, nil, now, 0.5, 0, nil)
	mock.ExpectQuery(`metadata->>'session_id'`).
		WithArgs(tenantID, sessionID, 50).
		WillReturnRows(memRows)

	svc := NewSessionService(repository.NewSessionRepositoryFromDB(mock, mock), NewMemoryService(&stubMemoryRepo{}, nil))
	resp, err := svc.ListMemories(context.Background(), tenantID, sessionID, "", 0, false)
	if err != nil || resp.Total != 1 {
		t.Fatalf("err=%v resp=%+v", err, resp)
	}
}

func TestSessionService_Promote_endedSession(t *testing.T) {
	t.Parallel()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mock.Close)

	tenantID := uuid.New().String()
	sessionID := uuid.New().String()
	ended := time.Now().UTC()
	meta := []byte(`{}`)
	mock.ExpectQuery(`FROM agent_sessions`).
		WithArgs(sessionID, tenantID).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "tenant_id", "agent_id", "metadata", "started_at", "ended_at",
		}).AddRow(sessionID, tenantID, nil, meta, ended.Add(-time.Hour), &ended))

	svc := NewSessionService(repository.NewSessionRepositoryFromDB(mock, mock), NewMemoryService(&stubMemoryRepo{}, nil))
	_, err = svc.Promote(context.Background(), tenantID, sessionID, &model.PromoteSessionRequest{TargetPrefix: "root"})
	if err == nil || err.Error() != "session ended" {
		t.Fatalf("got err=%v", err)
	}
}

func TestSessionService_Promote_success(t *testing.T) {
	t.Parallel()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mock.Close)

	tenantID := uuid.New().String()
	sessionID := uuid.New().String()
	_ = sessionRepoWithActiveSession(t, mock, tenantID, sessionID)

	mock.ExpectBegin()
	mock.ExpectQuery(`FOR UPDATE`).
		WithArgs(tenantID, sessionID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "path", "metadata"}))
	mock.ExpectCommit()

	svc := NewSessionService(repository.NewSessionRepositoryFromDB(mock, mock), NewMemoryService(&stubMemoryRepo{}, nil))
	resp, err := svc.Promote(context.Background(), tenantID, sessionID, nil)
	if err != nil || resp.Promoted != 0 || resp.TargetPrefix != "root" {
		t.Fatalf("err=%v resp=%+v", err, resp)
	}
}
