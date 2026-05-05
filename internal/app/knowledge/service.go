package knowledge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/xiaozhao/xiaozhao/internal/app/rbac"
	"github.com/xiaozhao/xiaozhao/internal/domain"
	"github.com/xiaozhao/xiaozhao/internal/infra/embedding"
	"github.com/xiaozhao/xiaozhao/internal/infra/parser"
	"github.com/xiaozhao/xiaozhao/internal/infra/storage"
	"github.com/xiaozhao/xiaozhao/internal/pkg/errcode"
	"github.com/xiaozhao/xiaozhao/internal/pkg/id"
)

const maxDocumentAttempts = 3

type Service struct {
	kbs      domain.KnowledgeBaseRepository
	docs     domain.DocumentRepository
	chunks   domain.DocumentChunkRepository
	files    domain.FileRepository
	objects  domain.FileObjectRepository
	projects domain.ProjectRepository
	audit    domain.AuditLogRepository
	usage    domain.UsageRepository
	rbac     *rbac.Checker
	store    storage.Store
	parser   parser.Parser
	embedder embedding.Provider
	chunker  *Chunker
}

type Options struct {
	ChunkSize    int
	ChunkOverlap int
}

func NewService(
	kbs domain.KnowledgeBaseRepository,
	docs domain.DocumentRepository,
	chunks domain.DocumentChunkRepository,
	files domain.FileRepository,
	objects domain.FileObjectRepository,
	projects domain.ProjectRepository,
	audit domain.AuditLogRepository,
	usage domain.UsageRepository,
	rbacChecker *rbac.Checker,
	store storage.Store,
	docParser parser.Parser,
	embedder embedding.Provider,
	opts Options,
) *Service {
	if docParser == nil {
		panic("knowledge: parser is required")
	}
	if embedder == nil {
		panic("knowledge: embedding provider is required")
	}
	return &Service{
		kbs: kbs, docs: docs, chunks: chunks, files: files, objects: objects, projects: projects,
		audit: audit, usage: usage, rbac: rbacChecker, store: store, parser: docParser, embedder: embedder,
		chunker: NewChunker(opts.ChunkSize, opts.ChunkOverlap),
	}
}

type CreateKnowledgeBaseInput struct {
	OrgID       string
	ProjectID   string
	UserID      string
	Name        string
	Description string
}

type KnowledgeBaseListResult struct {
	Items      []domain.KnowledgeBase
	NextCursor string
}

type DocumentListResult struct {
	Items      []domain.Document
	NextCursor string
}

func (s *Service) CreateKnowledgeBase(ctx context.Context, in CreateKnowledgeBaseInput) (*domain.KnowledgeBase, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, errcode.New(errcode.CodeInvalidArgument, "name is required")
	}
	if err := s.authorizeProject(ctx, in.OrgID, in.ProjectID, in.UserID, domain.RoleAdmin); err != nil {
		return nil, err
	}
	kb := &domain.KnowledgeBase{
		ID:              id.New(id.PrefixKnowledge),
		OrgID:           in.OrgID,
		ProjectID:       in.ProjectID,
		Name:            strings.TrimSpace(in.Name),
		Description:     strings.TrimSpace(in.Description),
		Visibility:      domain.KnowledgeBaseVisibilityProject,
		AccessPolicy:    map[string]any{},
		EmbeddingModel:  s.embedder.Model(),
		EmbeddingDim:    s.embedder.Dim(),
		ChunkConfig:     map[string]any{"chunk_size_chars": s.chunker.Size, "chunk_overlap_chars": s.chunker.Overlap},
		RetrievalConfig: map[string]any{"top_k": 8, "score_threshold": 0.2, "strategy": "hybrid"},
		Status:          domain.KnowledgeBaseStatusActive,
		CreatedBy:       in.UserID,
		CreatedAt:       time.Now(),
	}
	if err := s.kbs.Create(ctx, kb); err != nil {
		return nil, errcode.Wrap(err, errcode.CodeInternal, "create knowledge base")
	}
	s.auditLog(in.OrgID, in.ProjectID, in.UserID, "knowledge_base.created", "knowledge_base", kb.ID, nil)
	return kb, nil
}

func (s *Service) ListKnowledgeBases(ctx context.Context, orgID, projectID, userID string, limit int, cursor string) (KnowledgeBaseListResult, error) {
	if err := s.authorizeProject(ctx, orgID, projectID, userID, domain.RoleViewer); err != nil {
		return KnowledgeBaseListResult{}, err
	}
	limit = normalizeListLimit(limit)
	out, err := s.kbs.ListByProject(ctx, orgID, projectID, limit+1, cursor)
	if err != nil {
		return KnowledgeBaseListResult{}, errcode.Wrap(err, errcode.CodeInternal, "list knowledge bases")
	}
	items, next := trimPage(out, limit)
	return KnowledgeBaseListResult{Items: items, NextCursor: next}, nil
}

func (s *Service) GetKnowledgeBase(ctx context.Context, orgID, userID, kbID string) (*domain.KnowledgeBase, error) {
	kb, err := s.loadKnowledgeBase(ctx, orgID, userID, kbID, domain.RoleViewer)
	if err != nil {
		return nil, err
	}
	return kb, nil
}

func (s *Service) DeleteKnowledgeBase(ctx context.Context, orgID, userID, kbID string) error {
	kb, err := s.loadKnowledgeBase(ctx, orgID, userID, kbID, domain.RoleAdmin)
	if err != nil {
		return err
	}
	kb.Status = domain.KnowledgeBaseStatusDeleted
	if err := s.kbs.Update(ctx, kb); err != nil {
		return errcode.Wrap(err, errcode.CodeInternal, "delete knowledge base")
	}
	s.auditLog(orgID, kb.ProjectID, userID, "knowledge_base.deleted", "knowledge_base", kb.ID, nil)
	return nil
}

func (s *Service) AddDocument(ctx context.Context, orgID, userID, kbID, fileID string) (*domain.Document, error) {
	kb, err := s.loadKnowledgeBase(ctx, orgID, userID, kbID, domain.RoleAdmin)
	if err != nil {
		return nil, err
	}
	f, err := s.files.GetByID(ctx, fileID)
	if err != nil {
		return nil, errcode.New(errcode.CodeNotFound, "file not found")
	}
	if f.OrgID != orgID || f.ProjectID != kb.ProjectID || f.Status != domain.FileStatusUploaded {
		return nil, errcode.New(errcode.CodeNotFound, "file not found")
	}
	if f.Purpose != domain.FilePurposeKnowledgeBase {
		return nil, errcode.New(errcode.CodeInvalidArgument, "file purpose must be knowledge_base")
	}
	doc := &domain.Document{
		ID:              id.New(id.PrefixDocument),
		OrgID:           orgID,
		ProjectID:       kb.ProjectID,
		KnowledgeBaseID: kb.ID,
		FileID:          f.ID,
		Filename:        f.Filename,
		MimeType:        f.MimeType,
		Status:          domain.DocumentStatusPending,
		Attempts:        0,
		CreatedBy:       userID,
		CreatedAt:       time.Now(),
	}
	if err := s.docs.Create(ctx, doc); err != nil {
		return nil, errcode.Wrap(err, errcode.CodeInternal, "create document")
	}
	s.auditLog(orgID, kb.ProjectID, userID, "document.created", "document", doc.ID, map[string]any{"file_id": f.ID, "knowledge_base_id": kb.ID})
	return doc, nil
}

func (s *Service) ListDocuments(ctx context.Context, orgID, userID, kbID string, limit int, cursor string) (DocumentListResult, error) {
	kb, err := s.loadKnowledgeBase(ctx, orgID, userID, kbID, domain.RoleViewer)
	if err != nil {
		return DocumentListResult{}, err
	}
	limit = normalizeListLimit(limit)
	out, err := s.docs.ListByKnowledgeBase(ctx, orgID, kb.ID, limit+1, cursor)
	if err != nil {
		return DocumentListResult{}, errcode.Wrap(err, errcode.CodeInternal, "list documents")
	}
	items, next := trimPage(out, limit)
	return DocumentListResult{Items: items, NextCursor: next}, nil
}

func (s *Service) DeleteDocument(ctx context.Context, orgID, userID, kbID, docID string) error {
	kb, err := s.loadKnowledgeBase(ctx, orgID, userID, kbID, domain.RoleAdmin)
	if err != nil {
		return err
	}
	doc, err := s.docs.GetByID(ctx, docID)
	if err != nil || doc.OrgID != orgID || doc.KnowledgeBaseID != kb.ID {
		return errcode.New(errcode.CodeNotFound, "document not found")
	}
	doc.Status = domain.DocumentStatusDeleted
	if err := s.docs.Update(ctx, doc); err != nil {
		return errcode.Wrap(err, errcode.CodeInternal, "delete document")
	}
	if err := s.docs.DeleteChunks(ctx, doc.ID); err != nil {
		return errcode.Wrap(err, errcode.CodeInternal, "delete document chunks")
	}
	s.auditLog(orgID, kb.ProjectID, userID, "document.deleted", "document", doc.ID, nil)
	return nil
}

type SearchInput struct {
	OrgID            string
	ProjectID        string
	UserID           string
	KnowledgeBaseIDs []string
	DocumentIDs      []string
	Query            string
	TopK             int
	ScoreThreshold   float64
}

func (s *Service) Search(ctx context.Context, in SearchInput) ([]domain.KnowledgeSearchResult, error) {
	if strings.TrimSpace(in.Query) == "" {
		return nil, errcode.New(errcode.CodeInvalidArgument, "query is required")
	}
	if err := s.authorizeProject(ctx, in.OrgID, in.ProjectID, in.UserID, domain.RoleViewer); err != nil {
		return nil, err
	}
	kbIDs := cleanIDs(in.KnowledgeBaseIDs)
	if len(kbIDs) == 0 {
		kbs, err := s.kbs.ListByProject(ctx, in.OrgID, in.ProjectID, 100, "")
		if err != nil {
			return nil, errcode.Wrap(err, errcode.CodeInternal, "list knowledge bases")
		}
		for _, kb := range kbs {
			kbIDs = append(kbIDs, kb.ID)
		}
	} else {
		for _, kbID := range kbIDs {
			kb, err := s.kbs.GetByID(ctx, kbID)
			if err != nil || kb.OrgID != in.OrgID || kb.ProjectID != in.ProjectID || kb.Status == domain.KnowledgeBaseStatusDeleted {
				return nil, errcode.New(errcode.CodeNotFound, "knowledge base not found")
			}
		}
	}
	if len(kbIDs) == 0 {
		return []domain.KnowledgeSearchResult{}, nil
	}
	vecs, usage, err := s.embedder.Embed(ctx, []string{in.Query})
	if err != nil {
		return nil, err
	}
	if len(vecs) != 1 || len(vecs[0]) != s.embedder.Dim() {
		return nil, errcode.New(errcode.CodeInvalidArgument, "embedding dimension mismatch")
	}
	if in.TopK <= 0 {
		in.TopK = 8
	}
	if in.TopK > 50 {
		in.TopK = 50
	}
	if in.ScoreThreshold <= 0 {
		in.ScoreThreshold = 0.2
	}
	out, err := s.chunks.Search(ctx, domain.ChunkSearchInput{
		OrgID: in.OrgID, ProjectID: in.ProjectID, KnowledgeBaseIDs: kbIDs,
		DocumentIDs: cleanIDs(in.DocumentIDs), Query: in.Query, Embedding: vecs[0],
		TopK: in.TopK, ScoreThreshold: in.ScoreThreshold,
	})
	if err != nil {
		return nil, errcode.Wrap(err, errcode.CodeInternal, "search knowledge")
	}
	s.recordUsage(in.OrgID, in.ProjectID, in.UserID, "embedding", "query", float64(usage.InputTokens), "token")
	return out, nil
}

func (s *Service) ProcessPending(ctx context.Context, limit int) error {
	docs, err := s.docs.ListPending(ctx, limit)
	if err != nil {
		return err
	}
	for _, doc := range docs {
		if err := s.ProcessDocument(ctx, doc.ID); err != nil {
			continue
		}
	}
	return nil
}

func (s *Service) StartWorker(ctx context.Context, interval time.Duration, limit int) func() {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		_ = s.ProcessPending(ctx, limit)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = s.ProcessPending(ctx, limit)
			}
		}
	}()
	return wg.Wait
}

func (s *Service) ProcessDocument(ctx context.Context, documentID string) (err error) {
	doc, err := s.docs.GetByID(ctx, documentID)
	if err != nil {
		return err
	}
	if doc.Status == domain.DocumentStatusReady || doc.Status == domain.DocumentStatusDeleted {
		return nil
	}
	if doc.Attempts >= maxDocumentAttempts {
		doc.Status = domain.DocumentStatusFailed
		doc.ParseError = "document processing attempts exceeded"
		return s.docs.Update(ctx, doc)
	}
	doc.Status = domain.DocumentStatusProcessing
	doc.ParseError = ""
	doc.Attempts++
	doc.ProcessingUpdatedAt = time.Now()
	if err := s.docs.Update(ctx, doc); err != nil {
		return err
	}
	fail := func(e error) error {
		doc.Status = domain.DocumentStatusFailed
		doc.ParseError = truncate(e.Error(), 2000)
		_ = s.docs.Update(context.Background(), doc)
		return e
	}
	defer func() {
		if r := recover(); r != nil {
			err = fail(fmt.Errorf("document processing panic: %v", r))
		}
	}()
	obj, err := s.objects.GetByFileID(ctx, doc.FileID)
	if err != nil {
		return fail(err)
	}
	got, err := s.store.GetObject(ctx, storage.GetObjectInput{Bucket: obj.Bucket, Key: obj.ObjectKey})
	if err != nil {
		return fail(err)
	}
	defer got.Body.Close()
	parsed, err := s.parser.Parse(ctx, parser.Input{Filename: doc.Filename, MimeType: doc.MimeType, Reader: got.Body})
	if err != nil {
		return fail(err)
	}
	chunks := s.chunker.Split(ChunkInput{
		OrgID: doc.OrgID, ProjectID: doc.ProjectID, KnowledgeBaseID: doc.KnowledgeBaseID,
		DocumentID: doc.ID, FileID: doc.FileID, Parsed: parsed,
	})
	if len(chunks) == 0 {
		return fail(errors.New("document produced no chunks"))
	}
	texts := make([]string, 0, len(chunks))
	charCount := 0
	for _, ch := range chunks {
		texts = append(texts, ch.Content)
		charCount += len([]rune(ch.Content))
	}
	vecs, usage, err := s.embedder.Embed(ctx, texts)
	if err != nil {
		return fail(err)
	}
	if len(vecs) != len(chunks) {
		return fail(fmt.Errorf("embedding count mismatch: got %d want %d", len(vecs), len(chunks)))
	}
	for i := range chunks {
		if len(vecs[i]) != s.embedder.Dim() {
			return fail(fmt.Errorf("embedding dimension mismatch: got %d want %d", len(vecs[i]), s.embedder.Dim()))
		}
		chunks[i].Embedding = vecs[i]
		chunks[i].CreatedAt = time.Now()
	}
	// 旧 chunk 删除和新 chunk 写入必须在同一个事务里完成，避免重建索引期间出现检索空窗。
	if err := s.chunks.ReplaceByDocument(ctx, doc.ID, chunks); err != nil {
		return fail(err)
	}
	doc.Status = domain.DocumentStatusReady
	doc.ParseError = ""
	doc.ProcessingUpdatedAt = time.Time{}
	doc.ChunkCount = len(chunks)
	doc.CharCount = charCount
	doc.TokenCount = usage.InputTokens
	if err := s.docs.Update(ctx, doc); err != nil {
		return fail(err)
	}
	s.recordUsage(doc.OrgID, doc.ProjectID, doc.CreatedBy, "embedding", doc.ID, float64(usage.InputTokens), "token")
	s.auditLog(doc.OrgID, doc.ProjectID, doc.CreatedBy, "document.indexed", "document", doc.ID, map[string]any{"chunk_count": len(chunks)})
	return nil
}

func (s *Service) loadKnowledgeBase(ctx context.Context, orgID, userID, kbID string, min domain.Role) (*domain.KnowledgeBase, error) {
	kb, err := s.kbs.GetByID(ctx, kbID)
	if err != nil {
		return nil, errcode.New(errcode.CodeNotFound, "knowledge base not found")
	}
	if kb.OrgID != orgID || kb.Status == domain.KnowledgeBaseStatusDeleted {
		return nil, errcode.New(errcode.CodeNotFound, "knowledge base not found")
	}
	if err := s.authorizeProject(ctx, orgID, kb.ProjectID, userID, min); err != nil {
		return nil, errcode.New(errcode.CodeNotFound, "knowledge base not found")
	}
	return kb, nil
}

func (s *Service) authorizeProject(ctx context.Context, orgID, projectID, userID string, min domain.Role) error {
	if projectID == "" {
		return errcode.New(errcode.CodeInvalidArgument, "project_id is required")
	}
	if _, err := s.rbac.Require(ctx, orgID, userID, min); err != nil {
		return err
	}
	p, err := s.projects.GetByID(ctx, projectID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return errcode.New(errcode.CodeProjectNotFound, "project not found")
		}
		return errcode.Wrap(err, errcode.CodeInternal, "get project")
	}
	if p.OrgID != orgID {
		return errcode.New(errcode.CodeProjectNotFound, "project not found")
	}
	return nil
}

func (s *Service) auditLog(orgID, projectID, userID, action, typ, resourceID string, metadata map[string]any) {
	if s.audit == nil {
		return
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	_ = s.audit.Create(context.Background(), &domain.AuditLog{
		ID: id.New(id.PrefixAudit), OrgID: orgID, ProjectID: projectID, ActorUserID: userID,
		Action: action, ResourceType: typ, ResourceID: resourceID, Metadata: metadata, CreatedAt: time.Now(),
	})
}

func (s *Service) recordUsage(orgID, projectID, userID, sourceType, sourceID string, quantity float64, unit string) {
	if s.usage == nil || quantity <= 0 {
		return
	}
	_ = s.usage.Create(context.Background(), &domain.UsageRecord{
		ID: id.New(id.PrefixEvent), OrgID: orgID, ProjectID: projectID, UserID: userID,
		SourceType: sourceType, SourceID: sourceID, Quantity: quantity, Unit: unit, CreatedAt: time.Now(),
	})
}

func cleanIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, v := range ids {
		if strings.TrimSpace(v) != "" {
			out = append(out, strings.TrimSpace(v))
		}
	}
	return out
}

func normalizeListLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	if limit > 200 {
		return 200
	}
	return limit
}

func trimPage[T any](items []T, limit int) ([]T, string) {
	if len(items) <= limit {
		return items, ""
	}
	page := items[:limit]
	return page, pageCursor(page[len(page)-1])
}

func pageCursor(item any) string {
	switch v := item.(type) {
	case domain.KnowledgeBase:
		return v.CreatedAt.Format(time.RFC3339Nano)
	case domain.Document:
		return v.CreatedAt.Format(time.RFC3339Nano)
	default:
		return ""
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func ReadAllAndClose(rc io.ReadCloser) ([]byte, error) {
	defer rc.Close()
	return io.ReadAll(rc)
}
