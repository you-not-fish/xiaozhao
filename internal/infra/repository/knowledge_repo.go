package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/xiaozhao/xiaozhao/internal/domain"
)

type knowledgeBaseRow struct {
	ID              string         `gorm:"column:id;primaryKey;size:64"`
	OrgID           string         `gorm:"column:org_id;size:64;not null"`
	ProjectID       string         `gorm:"column:project_id;size:64;not null"`
	Name            string         `gorm:"column:name;size:128;not null"`
	Description     string         `gorm:"column:description;type:text;not null;default:''"`
	Visibility      string         `gorm:"column:visibility;size:32;not null;default:project"`
	AccessPolicy    datatypes.JSON `gorm:"column:access_policy;type:jsonb;not null"`
	EmbeddingModel  string         `gorm:"column:embedding_model;size:128;not null"`
	EmbeddingDim    int            `gorm:"column:embedding_dim;not null;default:1536"`
	ChunkConfig     datatypes.JSON `gorm:"column:chunk_config;type:jsonb;not null"`
	RetrievalConfig datatypes.JSON `gorm:"column:retrieval_config;type:jsonb;not null"`
	Status          string         `gorm:"column:status;size:32;not null;default:active"`
	CreatedBy       string         `gorm:"column:created_by;size:64;not null"`
	CreatedAt       time.Time      `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt       time.Time      `gorm:"column:updated_at;not null;default:now()"`
}

func (knowledgeBaseRow) TableName() string { return "knowledge_bases" }

type documentRow struct {
	ID                  string     `gorm:"column:id;primaryKey;size:64"`
	OrgID               string     `gorm:"column:org_id;size:64;not null"`
	ProjectID           string     `gorm:"column:project_id;size:64;not null"`
	KnowledgeBaseID     string     `gorm:"column:knowledge_base_id;size:64;not null"`
	FileID              string     `gorm:"column:file_id;size:64;not null"`
	Filename            string     `gorm:"column:filename;size:255;not null"`
	MimeType            string     `gorm:"column:mime_type;size:255;not null"`
	Status              string     `gorm:"column:status;size:32;not null"`
	ParseError          string     `gorm:"column:parse_error;type:text;not null;default:''"`
	Attempts            int        `gorm:"column:attempts;not null;default:0"`
	ProcessingUpdatedAt *time.Time `gorm:"column:processing_updated_at"`
	ChunkCount          int        `gorm:"column:chunk_count;not null;default:0"`
	CharCount           int        `gorm:"column:char_count;not null;default:0"`
	TokenCount          int        `gorm:"column:token_count;not null;default:0"`
	CreatedBy           string     `gorm:"column:created_by;size:64;not null"`
	CreatedAt           time.Time  `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt           time.Time  `gorm:"column:updated_at;not null;default:now()"`
}

func (documentRow) TableName() string { return "documents" }

type DocumentChunkRepo struct{ db *gorm.DB }

type KnowledgeBaseRepo struct{ db *gorm.DB }

type DocumentRepo struct{ db *gorm.DB }

func NewKnowledgeBaseRepo(db *gorm.DB) *KnowledgeBaseRepo { return &KnowledgeBaseRepo{db: db} }
func NewDocumentRepo(db *gorm.DB) *DocumentRepo           { return &DocumentRepo{db: db} }
func NewDocumentChunkRepo(db *gorm.DB) *DocumentChunkRepo { return &DocumentChunkRepo{db: db} }

func (r *KnowledgeBaseRepo) Create(ctx context.Context, kb *domain.KnowledgeBase) error {
	row, err := knowledgeBaseRowFromDomain(kb)
	if err != nil {
		return err
	}
	if row.CreatedAt.IsZero() {
		row.CreatedAt = now()
	}
	row.UpdatedAt = row.CreatedAt
	return r.db.WithContext(ctx).Create(&row).Error
}

func (r *KnowledgeBaseRepo) Update(ctx context.Context, kb *domain.KnowledgeBase) error {
	row, err := knowledgeBaseRowFromDomain(kb)
	if err != nil {
		return err
	}
	row.UpdatedAt = now()
	res := r.db.WithContext(ctx).Model(&knowledgeBaseRow{}).Where("id = ?", kb.ID).Updates(map[string]any{
		"name":             row.Name,
		"description":      row.Description,
		"visibility":       row.Visibility,
		"access_policy":    row.AccessPolicy,
		"embedding_model":  row.EmbeddingModel,
		"embedding_dim":    row.EmbeddingDim,
		"chunk_config":     row.ChunkConfig,
		"retrieval_config": row.RetrievalConfig,
		"status":           row.Status,
		"updated_at":       row.UpdatedAt,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	kb.UpdatedAt = row.UpdatedAt
	return nil
}

func (r *KnowledgeBaseRepo) GetByID(ctx context.Context, id string) (*domain.KnowledgeBase, error) {
	var row knowledgeBaseRow
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return knowledgeBaseRowToDomain(row)
}

func (r *KnowledgeBaseRepo) ListByProject(ctx context.Context, orgID, projectID string, limit int, cursor string) ([]domain.KnowledgeBase, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := r.db.WithContext(ctx).
		Where("org_id = ? AND project_id = ? AND status <> ?", orgID, projectID, domain.KnowledgeBaseStatusDeleted)
	if cursor != "" {
		t, err := time.Parse(time.RFC3339Nano, cursor)
		if err != nil {
			return nil, fmt.Errorf("invalid cursor: %w", err)
		}
		q = q.Where("created_at < ?", t)
	}
	var rows []knowledgeBaseRow
	err := q.
		Order("created_at DESC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.KnowledgeBase, 0, len(rows))
	for _, row := range rows {
		kb, err := knowledgeBaseRowToDomain(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *kb)
	}
	return out, nil
}

func (r *DocumentRepo) Create(ctx context.Context, doc *domain.Document) error {
	row := documentRowFromDomain(doc)
	if row.CreatedAt.IsZero() {
		row.CreatedAt = now()
	}
	row.UpdatedAt = row.CreatedAt
	return r.db.WithContext(ctx).Create(&row).Error
}

func (r *DocumentRepo) Update(ctx context.Context, doc *domain.Document) error {
	row := documentRowFromDomain(doc)
	row.UpdatedAt = now()
	res := r.db.WithContext(ctx).Model(&documentRow{}).Where("id = ?", doc.ID).Updates(map[string]any{
		"filename":              row.Filename,
		"mime_type":             row.MimeType,
		"status":                row.Status,
		"parse_error":           row.ParseError,
		"attempts":              row.Attempts,
		"processing_updated_at": row.ProcessingUpdatedAt,
		"chunk_count":           row.ChunkCount,
		"char_count":            row.CharCount,
		"token_count":           row.TokenCount,
		"updated_at":            row.UpdatedAt,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	doc.UpdatedAt = row.UpdatedAt
	return nil
}

func (r *DocumentRepo) GetByID(ctx context.Context, id string) (*domain.Document, error) {
	var row documentRow
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return documentRowToDomain(row), nil
}

func (r *DocumentRepo) ListByKnowledgeBase(ctx context.Context, orgID, knowledgeBaseID string, limit int, cursor string) ([]domain.Document, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := r.db.WithContext(ctx).
		Where("org_id = ? AND knowledge_base_id = ? AND status <> ?", orgID, knowledgeBaseID, domain.DocumentStatusDeleted)
	if cursor != "" {
		t, err := time.Parse(time.RFC3339Nano, cursor)
		if err != nil {
			return nil, fmt.Errorf("invalid cursor: %w", err)
		}
		q = q.Where("created_at < ?", t)
	}
	var rows []documentRow
	err := q.
		Order("created_at DESC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.Document, 0, len(rows))
	for _, row := range rows {
		out = append(out, *documentRowToDomain(row))
	}
	return out, nil
}

func (r *DocumentRepo) ListPending(ctx context.Context, limit int) ([]domain.Document, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var rows []documentRow
	err := r.db.WithContext(ctx).
		Where("status = ? OR (status = ? AND (processing_updated_at IS NULL OR processing_updated_at < NOW() - INTERVAL '15 minutes'))",
			domain.DocumentStatusPending, domain.DocumentStatusProcessing).
		Order("updated_at ASC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.Document, 0, len(rows))
	for _, row := range rows {
		out = append(out, *documentRowToDomain(row))
	}
	return out, nil
}

func (r *DocumentRepo) DeleteChunks(ctx context.Context, documentID string) error {
	return r.db.WithContext(ctx).Exec("DELETE FROM document_chunks WHERE document_id = ?", documentID).Error
}

func (r *DocumentChunkRepo) CreateBatch(ctx context.Context, chunks []domain.DocumentChunk) error {
	if len(chunks) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return insertChunksTx(tx, chunks) })
}

func (r *DocumentChunkRepo) ReplaceByDocument(ctx context.Context, documentID string, chunks []domain.DocumentChunk) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM document_chunks WHERE document_id = ?", documentID).Error; err != nil {
			return err
		}
		return insertChunksTx(tx, chunks)
	})
}

const knowledgeEmbeddingDim = 1536

func insertChunksTx(tx *gorm.DB, chunks []domain.DocumentChunk) error {
	if len(chunks) == 0 {
		return nil
	}
	const batchSize = 200
	for start := 0; start < len(chunks); start += batchSize {
		end := start + batchSize
		if end > len(chunks) {
			end = len(chunks)
		}
		batch := chunks[start:end]
		values := make([]string, 0, len(batch))
		args := make([]any, 0, len(batch)*12)
		for _, ch := range batch {
			if len(ch.Embedding) != knowledgeEmbeddingDim {
				return fmt.Errorf("document chunk %s embedding dimension = %d, want %d", ch.ID, len(ch.Embedding), knowledgeEmbeddingDim)
			}
			meta, err := marshalSettings(ch.Metadata)
			if err != nil {
				return err
			}
			createdAt := ch.CreatedAt
			if createdAt.IsZero() {
				createdAt = now()
			}
			values = append(values, "(?, ?, ?, ?, ?, ?, ?, ?, ?::jsonb, ?::vector, ?, ?)")
			args = append(args,
				ch.ID, ch.OrgID, ch.ProjectID, ch.KnowledgeBaseID, ch.DocumentID, ch.FileID,
				ch.ChunkIndex, ch.Content, string(meta), formatVector(ch.Embedding), string(ch.Status), createdAt,
			)
		}
		sql := `INSERT INTO document_chunks
(id, org_id, project_id, knowledge_base_id, document_id, file_id, chunk_index, content, metadata, embedding, status, created_at)
VALUES ` + strings.Join(values, ",")
		if err := tx.Exec(sql, args...).Error; err != nil {
			return err
		}
	}
	return nil
}

func (r *DocumentChunkRepo) Search(ctx context.Context, in domain.ChunkSearchInput) ([]domain.KnowledgeSearchResult, error) {
	if in.TopK <= 0 || in.TopK > 50 {
		in.TopK = 8
	}
	kbIDs := normalizeIDs(in.KnowledgeBaseIDs)
	docIDs := normalizeIDs(in.DocumentIDs)
	args := []any{in.OrgID, in.ProjectID}
	conds := []string{"c.org_id = ?", "c.project_id = ?", "c.status = 'ready'", "d.status = 'ready'", "kb.status = 'active'"}
	if len(kbIDs) > 0 {
		args = append(args, kbIDs)
		conds = append(conds, "c.knowledge_base_id IN ?")
	}
	if len(docIDs) > 0 {
		args = append(args, docIDs)
		conds = append(conds, "c.document_id IN ?")
	}

	rows := []knowledgeSearchRow{}
	vectorSQL := `
SELECT c.id AS chunk_id, c.document_id, c.file_id, d.filename, c.knowledge_base_id, c.content,
       c.metadata, GREATEST(0, 1 - (c.embedding <=> ?::vector)) AS score
FROM document_chunks c
JOIN documents d ON d.id = c.document_id
JOIN knowledge_bases kb ON kb.id = c.knowledge_base_id
WHERE ` + strings.Join(conds, " AND ") + `
ORDER BY c.embedding <=> ?::vector
LIMIT ?`
	vectorArgs := append([]any{formatVector(in.Embedding)}, args...)
	vectorArgs = append(vectorArgs, formatVector(in.Embedding), in.TopK)
	if err := r.db.WithContext(ctx).Raw(vectorSQL, vectorArgs...).Scan(&rows).Error; err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	out := make([]domain.KnowledgeSearchResult, 0, len(rows))
	for _, row := range rows {
		if row.Score < in.ScoreThreshold {
			continue
		}
		seen[row.ChunkID] = true
		out = append(out, row.toDomain())
	}

	if len(out) < in.TopK && strings.TrimSpace(in.Query) != "" {
		keywordRows := []knowledgeSearchRow{}
		kwSQL := `
SELECT c.id AS chunk_id, c.document_id, c.file_id, d.filename, c.knowledge_base_id, c.content,
       c.metadata, 0.35 AS score
FROM document_chunks c
JOIN documents d ON d.id = c.document_id
JOIN knowledge_bases kb ON kb.id = c.knowledge_base_id
WHERE ` + strings.Join(append(conds, "(c.keyword @@ plainto_tsquery('simple', ?) OR position(lower(?) in lower(c.content)) > 0)"), " AND ") + `
ORDER BY ts_rank_cd(c.keyword, plainto_tsquery('simple', ?)) DESC, c.created_at DESC
LIMIT ?`
		kwArgs := append([]any{}, args...)
		kwArgs = append(kwArgs, sanitizeQuery(in.Query), sanitizeQuery(in.Query), sanitizeQuery(in.Query), in.TopK)
		if err := r.db.WithContext(ctx).Raw(kwSQL, kwArgs...).Scan(&keywordRows).Error; err != nil {
			return nil, err
		}
		for _, row := range keywordRows {
			if seen[row.ChunkID] {
				continue
			}
			seen[row.ChunkID] = true
			out = append(out, row.toDomain())
			if len(out) >= in.TopK {
				break
			}
		}
	}
	return out, nil
}

type knowledgeSearchRow struct {
	ChunkID         string         `gorm:"column:chunk_id"`
	DocumentID      string         `gorm:"column:document_id"`
	FileID          string         `gorm:"column:file_id"`
	Filename        string         `gorm:"column:filename"`
	KnowledgeBaseID string         `gorm:"column:knowledge_base_id"`
	Content         string         `gorm:"column:content"`
	Metadata        datatypes.JSON `gorm:"column:metadata"`
	Score           float64        `gorm:"column:score"`
}

func (r knowledgeSearchRow) toDomain() domain.KnowledgeSearchResult {
	meta := map[string]any{}
	_ = json.Unmarshal(r.Metadata, &meta)
	return domain.KnowledgeSearchResult{
		ChunkID:         r.ChunkID,
		DocumentID:      r.DocumentID,
		FileID:          r.FileID,
		Filename:        r.Filename,
		KnowledgeBaseID: r.KnowledgeBaseID,
		Content:         r.Content,
		Score:           r.Score,
		Metadata:        meta,
	}
}

func knowledgeBaseRowFromDomain(kb *domain.KnowledgeBase) (knowledgeBaseRow, error) {
	access, err := marshalSettings(kb.AccessPolicy)
	if err != nil {
		return knowledgeBaseRow{}, err
	}
	chunk, err := marshalSettings(kb.ChunkConfig)
	if err != nil {
		return knowledgeBaseRow{}, err
	}
	retrieval, err := marshalSettings(kb.RetrievalConfig)
	if err != nil {
		return knowledgeBaseRow{}, err
	}
	return knowledgeBaseRow{
		ID:              kb.ID,
		OrgID:           kb.OrgID,
		ProjectID:       kb.ProjectID,
		Name:            kb.Name,
		Description:     kb.Description,
		Visibility:      string(kb.Visibility),
		AccessPolicy:    access,
		EmbeddingModel:  kb.EmbeddingModel,
		EmbeddingDim:    kb.EmbeddingDim,
		ChunkConfig:     chunk,
		RetrievalConfig: retrieval,
		Status:          string(kb.Status),
		CreatedBy:       kb.CreatedBy,
		CreatedAt:       kb.CreatedAt,
		UpdatedAt:       kb.UpdatedAt,
	}, nil
}

func knowledgeBaseRowToDomain(row knowledgeBaseRow) (*domain.KnowledgeBase, error) {
	access, err := unmarshalSettings(row.AccessPolicy)
	if err != nil {
		return nil, err
	}
	chunk, err := unmarshalSettings(row.ChunkConfig)
	if err != nil {
		return nil, err
	}
	retrieval, err := unmarshalSettings(row.RetrievalConfig)
	if err != nil {
		return nil, err
	}
	return &domain.KnowledgeBase{
		ID:              row.ID,
		OrgID:           row.OrgID,
		ProjectID:       row.ProjectID,
		Name:            row.Name,
		Description:     row.Description,
		Visibility:      domain.KnowledgeBaseVisibility(row.Visibility),
		AccessPolicy:    access,
		EmbeddingModel:  row.EmbeddingModel,
		EmbeddingDim:    row.EmbeddingDim,
		ChunkConfig:     chunk,
		RetrievalConfig: retrieval,
		Status:          domain.KnowledgeBaseStatus(row.Status),
		CreatedBy:       row.CreatedBy,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}, nil
}

func documentRowFromDomain(doc *domain.Document) documentRow {
	return documentRow{
		ID:                  doc.ID,
		OrgID:               doc.OrgID,
		ProjectID:           doc.ProjectID,
		KnowledgeBaseID:     doc.KnowledgeBaseID,
		FileID:              doc.FileID,
		Filename:            doc.Filename,
		MimeType:            doc.MimeType,
		Status:              string(doc.Status),
		ParseError:          doc.ParseError,
		Attempts:            doc.Attempts,
		ProcessingUpdatedAt: timePtr(doc.ProcessingUpdatedAt),
		ChunkCount:          doc.ChunkCount,
		CharCount:           doc.CharCount,
		TokenCount:          doc.TokenCount,
		CreatedBy:           doc.CreatedBy,
		CreatedAt:           doc.CreatedAt,
		UpdatedAt:           doc.UpdatedAt,
	}
}

func documentRowToDomain(row documentRow) *domain.Document {
	return &domain.Document{
		ID:                  row.ID,
		OrgID:               row.OrgID,
		ProjectID:           row.ProjectID,
		KnowledgeBaseID:     row.KnowledgeBaseID,
		FileID:              row.FileID,
		Filename:            row.Filename,
		MimeType:            row.MimeType,
		Status:              domain.DocumentStatus(row.Status),
		ParseError:          row.ParseError,
		Attempts:            row.Attempts,
		ProcessingUpdatedAt: derefTime(row.ProcessingUpdatedAt),
		ChunkCount:          row.ChunkCount,
		CharCount:           row.CharCount,
		TokenCount:          row.TokenCount,
		CreatedBy:           row.CreatedBy,
		CreatedAt:           row.CreatedAt,
		UpdatedAt:           row.UpdatedAt,
	}
}

func formatVector(v []float32) string {
	parts := make([]string, 0, len(v))
	for _, f := range v {
		if math.IsNaN(float64(f)) || math.IsInf(float64(f), 0) {
			f = 0
		}
		parts = append(parts, strconv.FormatFloat(float64(f), 'f', -1, 32))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func normalizeIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if strings.TrimSpace(id) != "" {
			out = append(out, strings.TrimSpace(id))
		}
	}
	return out
}

func sanitizeQuery(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 80 {
		s = s[:80]
	}
	return s
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func derefTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}
