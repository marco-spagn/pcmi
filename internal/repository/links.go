package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/marco-spagn/pcmi/internal/model"
)

// linksReadQuerier is implemented by *pgxpool.Pool and pgxmock pools (List, Count).
type linksReadQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type LinksRepository struct {
	w *pgxpool.Pool
	r linksReadQuerier
}

func NewLinksRepository(writePool, readPool *pgxpool.Pool) *LinksRepository {
	if readPool == nil {
		readPool = writePool
	}
	return &LinksRepository{w: writePool, r: readPool}
}

func (r *LinksRepository) Create(ctx context.Context, tenantID string, req model.CreateLinkRequest) (*model.MemoryLink, error) {
	from := strings.TrimSpace(req.FromPath)
	to := strings.TrimSpace(req.ToPath)
	if from == "" || to == "" {
		return nil, fmt.Errorf("from_path and to_path are required")
	}
	linkType := strings.TrimSpace(req.LinkType)
	if linkType == "" {
		linkType = "related"
	}
	meta := req.Metadata
	if meta == nil {
		meta = map[string]interface{}{}
	}
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return nil, err
	}

	var link model.MemoryLink
	err = r.w.QueryRow(ctx, `
		INSERT INTO memory_links (tenant_id, from_path, to_path, link_type, metadata)
		VALUES ($1::uuid, $2::ltree, $3::ltree, $4, $5)
		ON CONFLICT (tenant_id, from_path, to_path, link_type)
		DO UPDATE SET metadata = EXCLUDED.metadata
		RETURNING id, from_path::text, to_path::text, link_type, metadata, created_at`,
		tenantID, from, to, linkType, metaJSON,
	).Scan(&link.ID, &link.FromPath, &link.ToPath, &link.LinkType, &link.Metadata, &link.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create link: %w", err)
	}
	return &link, nil
}

func (r *LinksRepository) List(ctx context.Context, tenantID, fromPath, toPath, linkType string, limit int) ([]model.MemoryLink, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	fromPath = strings.TrimSpace(fromPath)
	toPath = strings.TrimSpace(toPath)
	linkType = strings.TrimSpace(linkType)

	q := `SELECT id, from_path::text, to_path::text, link_type, metadata, created_at
	      FROM memory_links WHERE tenant_id = $1::uuid`
	args := []any{tenantID}
	n := 2
	if fromPath != "" {
		q += fmt.Sprintf(" AND from_path = $%d::ltree", n)
		args = append(args, fromPath)
		n++
	}
	if toPath != "" {
		q += fmt.Sprintf(" AND to_path = $%d::ltree", n)
		args = append(args, toPath)
		n++
	}
	if linkType != "" {
		q += fmt.Sprintf(" AND link_type = $%d", n)
		args = append(args, linkType)
		n++
	}
	q += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", n)
	args = append(args, limit)

	rows, err := r.r.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.MemoryLink
	for rows.Next() {
		var l model.MemoryLink
		if err := rows.Scan(&l.ID, &l.FromPath, &l.ToPath, &l.LinkType, &l.Metadata, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (r *LinksRepository) Count(ctx context.Context, tenantID string) (int, error) {
	var n int
	err := r.r.QueryRow(ctx, `SELECT COUNT(*) FROM memory_links WHERE tenant_id = $1::uuid`, tenantID).Scan(&n)
	return n, err
}
