package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/repository"
)

// SessionService manages agent sessions and working memory.
type SessionService struct {
	sessions *repository.SessionRepository
	memories *MemoryService
}

func NewSessionService(sessions *repository.SessionRepository, memories *MemoryService) *SessionService {
	return &SessionService{sessions: sessions, memories: memories}
}

func (s *SessionService) Create(ctx context.Context, tenantID string, req *model.CreateSessionRequest) (*model.AgentSession, error) {
	if req == nil {
		req = &model.CreateSessionRequest{}
	}
	return s.sessions.Create(ctx, tenantID, *req)
}

func (s *SessionService) Get(ctx context.Context, tenantID, sessionID string) (*model.AgentSession, error) {
	return s.sessions.Get(ctx, tenantID, sessionID)
}

func (s *SessionService) End(ctx context.Context, tenantID, sessionID string) (*model.AgentSession, error) {
	return s.sessions.End(ctx, tenantID, sessionID)
}

func (s *SessionService) StoreMemory(ctx context.Context, tenantID, sessionID string, req *model.SessionStoreMemoryRequest) (*StoreResult, error) {
	sess, err := s.sessions.Get(ctx, tenantID, sessionID)
	if err != nil {
		return nil, err
	}
	if sess.EndedAt != nil {
		return nil, fmt.Errorf("session ended")
	}
	storeReq := repository.SessionScopedStoreRequest(sessionID, *req)
	return s.memories.Store(ctx, &storeReq, tenantID)
}

func (s *SessionService) ListMemories(ctx context.Context, tenantID, sessionID, pathPrefix string, limit int, includeLongTerm bool) (*model.SessionMemoriesResponse, error) {
	if _, err := s.sessions.Get(ctx, tenantID, sessionID); err != nil {
		return nil, err
	}
	entries, err := s.sessions.ListSessionMemories(ctx, tenantID, sessionID, pathPrefix, limit, includeLongTerm)
	if err != nil {
		return nil, err
	}
	return &model.SessionMemoriesResponse{
		SessionID: sessionID,
		Entries:   entries,
		Total:     len(entries),
	}, nil
}

func (s *SessionService) Promote(ctx context.Context, tenantID, sessionID string, req *model.PromoteSessionRequest) (*model.PromoteSessionResponse, error) {
	sess, err := s.sessions.Get(ctx, tenantID, sessionID)
	if err != nil {
		return nil, err
	}
	if sess.EndedAt != nil {
		return nil, fmt.Errorf("session ended")
	}
	target := "root"
	if req != nil && strings.TrimSpace(req.TargetPrefix) != "" {
		target = strings.TrimSpace(req.TargetPrefix)
	}
	n, err := s.sessions.Promote(ctx, tenantID, sessionID, target)
	if err != nil {
		return nil, err
	}
	return &model.PromoteSessionResponse{
		SessionID:    sessionID,
		Promoted:     n,
		TargetPrefix: target,
		Status:       "promoted",
	}, nil
}
