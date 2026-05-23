package repository

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/marco-spagn/pcmi/internal/model"
)

func sessionMemoryRows() *pgxmock.Rows {
	return pgxmock.NewRows([]string{
		"id", "tenant_id", "path", "content", "metadata", "tags", "embedding_model",
		"embedding_space", "version", "valid_from", "valid_to", "source_agent_id",
		"created_at", "importance", "access_count", "last_accessed_at",
	})
}

func TestSessionRepository_Create_success(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenantID := uuid.New().String()
	sessionID := uuid.New().String()
	started := time.Unix(1700000000, 0).UTC()
	meta := []byte(`{"purpose":"test"}`)
	rows := pgxmock.NewRows([]string{
		"id", "tenant_id", "agent_id", "metadata", "started_at", "ended_at",
	}).AddRow(sessionID, tenantID, nil, meta, started, nil)

	mock.ExpectQuery(`INSERT INTO agent_sessions`).
		WithArgs(tenantID, pgxmock.AnyArg(), meta).
		WillReturnRows(rows)

	repo := NewSessionRepositoryFromDB(mock, mock)
	got, err := repo.Create(context.Background(), tenantID, model.CreateSessionRequest{
		Metadata: map[string]any{"purpose": "test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != sessionID || got.Status != "active" {
		t.Fatalf("unexpected %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSessionRepository_Get_notFound(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	mock.ExpectQuery(`FROM agent_sessions`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnError(pgx.ErrNoRows)

	repo := NewSessionRepositoryFromDB(mock, mock)
	_, err = repo.Get(context.Background(), uuid.New().String(), uuid.New().String())
	if err == nil || err.Error() != "session not found" {
		t.Fatalf("got err=%v", err)
	}
}

func TestSessionRepository_End_success(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenantID := uuid.New().String()
	sessionID := uuid.New().String()
	ended := time.Unix(1700000001, 0).UTC()
	meta := []byte(`{}`)
	rows := pgxmock.NewRows([]string{
		"id", "tenant_id", "agent_id", "metadata", "started_at", "ended_at",
	}).AddRow(sessionID, tenantID, nil, meta, ended.Add(-time.Hour), &ended)

	mock.ExpectQuery(`UPDATE agent_sessions`).
		WithArgs(sessionID, tenantID).
		WillReturnRows(rows)

	repo := NewSessionRepositoryFromDB(mock, mock)
	got, err := repo.End(context.Background(), tenantID, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "ended" {
		t.Fatalf("status=%q", got.Status)
	}
}

func TestSessionRepository_ListSessionMemories_workingOnly(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenantID := uuid.New().String()
	sessionID := uuid.New().String()
	now := time.Unix(1700000002, 0).UTC()
	memRows := sessionMemoryRows().AddRow(
		int64(1), tenantID, "sessions.x.note", "hi", []byte(`{"session_id":"`+sessionID+`"}`),
		[]string{}, "unspecified", "default", 1, now, nil, nil, now, 0.5, 0, nil,
	)
	mock.ExpectQuery(`metadata->>'session_id'`).
		WithArgs(tenantID, sessionID, 50).
		WillReturnRows(memRows)

	repo := NewSessionRepositoryFromDB(mock, mock)
	got, err := repo.ListSessionMemories(context.Background(), tenantID, sessionID, "", 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Content != "hi" {
		t.Fatalf("unexpected %+v", got)
	}
}

func TestSessionRepository_ListSessionMemories_withLongTerm(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenantID := uuid.New().String()
	sessionID := uuid.New().String()
	now := time.Now().UTC()
	mock.ExpectQuery(`metadata->>'session_id'`).
		WithArgs(tenantID, sessionID, 10).
		WillReturnRows(sessionMemoryRows())

	lt := sessionMemoryRows().AddRow(
		int64(2), tenantID, "root.lt", "lt", []byte(`{}`),
		[]string{}, "unspecified", "default", 1, now, nil, nil, now, 0.5, 0, nil,
	)
	mock.ExpectQuery(`metadata->>'session_id' IS NULL`).
		WithArgs(tenantID, "root", 10).
		WillReturnRows(lt)

	repo := NewSessionRepositoryFromDB(mock, mock)
	got, err := repo.ListSessionMemories(context.Background(), tenantID, sessionID, "", 10, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Path != "root.lt" {
		t.Fatalf("unexpected %+v", got)
	}
}

func TestSessionRepository_Promote_success(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	tenantID := uuid.New().String()
	sessionID := uuid.New().String()
	meta, _ := json.Marshal(map[string]any{
		sessionMetadataKey: sessionID,
		sessionScopeKey:    sessionScopeWorking,
	})

	mock.ExpectBegin()
	selectRows := pgxmock.NewRows([]string{"id", "path", "metadata"}).
		AddRow(int64(9), "sessions."+sessionID+".note", meta)
	mock.ExpectQuery(`FOR UPDATE`).
		WithArgs(tenantID, sessionID).
		WillReturnRows(selectRows)
	mock.ExpectExec(`UPDATE memory_entries`).
		WithArgs(int64(9), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectCommit()

	repo := NewSessionRepositoryFromDB(mock, mock)
	n, err := repo.Promote(context.Background(), tenantID, sessionID, "root")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("promoted=%d", n)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSessionRepository_Promote_beginError(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	mock.ExpectBegin().WillReturnError(errors.New("begin failed"))

	repo := NewSessionRepositoryFromDB(mock, mock)
	_, err = repo.Promote(context.Background(), uuid.New().String(), uuid.New().String(), "root")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNewSessionRepository_nilReadPool(t *testing.T) {
	t.Parallel()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mock.Close)
	repo := NewSessionRepositoryFromDB(mock, nil)
	if repo.r == nil {
		t.Fatal("read pool should default to write")
	}
}
