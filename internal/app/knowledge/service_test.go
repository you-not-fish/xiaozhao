package knowledge

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/xiaozhao/xiaozhao/internal/app/rbac"
	"github.com/xiaozhao/xiaozhao/internal/domain"
	"github.com/xiaozhao/xiaozhao/internal/infra/embedding"
	"github.com/xiaozhao/xiaozhao/internal/infra/parser"
	"github.com/xiaozhao/xiaozhao/internal/infra/storage"
	"github.com/xiaozhao/xiaozhao/internal/pkg/errcode"
)

func TestServiceAddAndProcessDocument(t *testing.T) {
	svc, fx := newKnowledgeFixture()
	kb, err := svc.CreateKnowledgeBase(context.Background(), CreateKnowledgeBaseInput{
		OrgID: "org_1", ProjectID: "proj_1", UserID: "user_1", Name: "产品文档",
	})
	if err != nil {
		t.Fatalf("CreateKnowledgeBase() error = %v", err)
	}
	doc, err := svc.AddDocument(context.Background(), "org_1", "user_1", kb.ID, "file_1")
	if err != nil {
		t.Fatalf("AddDocument() error = %v", err)
	}
	if doc.Status != domain.DocumentStatusPending {
		t.Fatalf("status = %s, want pending", doc.Status)
	}
	if err := svc.ProcessDocument(context.Background(), doc.ID); err != nil {
		t.Fatalf("ProcessDocument() error = %v", err)
	}
	updated := fx.docs.docs[doc.ID]
	if updated.Status != domain.DocumentStatusReady {
		t.Fatalf("status = %s, want ready; error=%s", updated.Status, updated.ParseError)
	}
	if updated.ChunkCount == 0 || len(fx.chunks.chunks) == 0 {
		t.Fatalf("chunks not created: doc=%#v chunks=%d", updated, len(fx.chunks.chunks))
	}
}

func TestServiceRejectsCrossTenantDocumentAttach(t *testing.T) {
	svc, _ := newKnowledgeFixture()
	kb, err := svc.CreateKnowledgeBase(context.Background(), CreateKnowledgeBaseInput{
		OrgID: "org_1", ProjectID: "proj_1", UserID: "user_1", Name: "产品文档",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.AddDocument(context.Background(), "org_1", "user_1", kb.ID, "file_other")
	if errcode.CodeOf(err) != errcode.CodeNotFound {
		t.Fatalf("error code = %s, want not_found", errcode.CodeOf(err))
	}
}

func TestServiceSearchUsesAllowedTenantScope(t *testing.T) {
	svc, fx := newKnowledgeFixture()
	fx.kbs.kbs["kb_1"] = &domain.KnowledgeBase{ID: "kb_1", OrgID: "org_1", ProjectID: "proj_1", Status: domain.KnowledgeBaseStatusActive}
	fx.chunks.chunks = append(fx.chunks.chunks, domain.DocumentChunk{
		ID: "chk_1", OrgID: "org_1", ProjectID: "proj_1", KnowledgeBaseID: "kb_1", DocumentID: "doc_1", FileID: "file_1",
		Content: "退款政策支持七天内申请", Status: domain.DocumentChunkStatusReady,
	})

	results, err := svc.Search(context.Background(), SearchInput{
		OrgID: "org_1", ProjectID: "proj_1", UserID: "user_1", KnowledgeBaseIDs: []string{"kb_1"}, Query: "退款",
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 || !strings.Contains(results[0].Content, "退款") {
		t.Fatalf("results = %#v", results)
	}
}

type knowledgeFixture struct {
	kbs     *memoryKBRepo
	docs    *memoryDocRepo
	chunks  *memoryChunkRepo
	files   *memoryKnowledgeFileRepo
	objects *memoryKnowledgeObjectRepo
	store   *memoryKnowledgeStore
}

func newKnowledgeFixture() (*Service, *knowledgeFixture) {
	kbs := &memoryKBRepo{kbs: map[string]*domain.KnowledgeBase{}}
	docs := &memoryDocRepo{docs: map[string]*domain.Document{}}
	chunks := &memoryChunkRepo{}
	files := &memoryKnowledgeFileRepo{files: map[string]*domain.File{
		"file_1":     {ID: "file_1", OrgID: "org_1", ProjectID: "proj_1", UserID: "user_1", Filename: "doc.txt", MimeType: "text/plain", Purpose: domain.FilePurposeKnowledgeBase, Status: domain.FileStatusUploaded},
		"file_other": {ID: "file_other", OrgID: "org_2", ProjectID: "proj_2", UserID: "user_2", Filename: "secret.txt", MimeType: "text/plain", Purpose: domain.FilePurposeKnowledgeBase, Status: domain.FileStatusUploaded},
	}}
	objects := &memoryKnowledgeObjectRepo{objects: map[string]*domain.FileObject{
		"file_1": {ID: "fobj_1", FileID: "file_1", Bucket: "bucket", ObjectKey: "file_1.txt"},
	}}
	projects := &memoryKnowledgeProjectRepo{projects: map[string]*domain.Project{
		"proj_1": {ID: "proj_1", OrgID: "org_1"},
		"proj_2": {ID: "proj_2", OrgID: "org_2"},
	}}
	members := &memoryKnowledgeMemberRepo{members: map[string]*domain.OrgMember{
		"org_1/user_1": {OrgID: "org_1", UserID: "user_1", Role: domain.RoleAdmin, Status: domain.MemberStatusActive},
	}}
	store := &memoryKnowledgeStore{objects: map[string][]byte{"bucket/file_1.txt": []byte("退款政策支持七天内申请。请保留订单号。")}}
	svc := NewService(kbs, docs, chunks, files, objects, projects, memoryKnowledgeAudit{}, memoryKnowledgeUsage{}, rbac.NewChecker(members), store, parser.NewDefaultParser(), embedding.NewMockProvider("mock", 1536), Options{ChunkSize: 20, ChunkOverlap: 4})
	return svc, &knowledgeFixture{kbs: kbs, docs: docs, chunks: chunks, files: files, objects: objects, store: store}
}

type memoryKBRepo struct {
	kbs map[string]*domain.KnowledgeBase
}

func (r *memoryKBRepo) Create(_ context.Context, kb *domain.KnowledgeBase) error {
	cp := *kb
	r.kbs[kb.ID] = &cp
	return nil
}
func (r *memoryKBRepo) Update(_ context.Context, kb *domain.KnowledgeBase) error {
	cp := *kb
	r.kbs[kb.ID] = &cp
	return nil
}
func (r *memoryKBRepo) GetByID(_ context.Context, id string) (*domain.KnowledgeBase, error) {
	kb, ok := r.kbs[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *kb
	return &cp, nil
}
func (r *memoryKBRepo) ListByProject(_ context.Context, orgID, projectID string, limit int, cursor string) ([]domain.KnowledgeBase, error) {
	var out []domain.KnowledgeBase
	for _, kb := range r.kbs {
		if kb.OrgID == orgID && kb.ProjectID == projectID && kb.Status != domain.KnowledgeBaseStatusDeleted {
			out = append(out, *kb)
		}
	}
	return out, nil
}

type memoryDocRepo struct{ docs map[string]*domain.Document }

func (r *memoryDocRepo) Create(_ context.Context, doc *domain.Document) error {
	cp := *doc
	r.docs[doc.ID] = &cp
	return nil
}
func (r *memoryDocRepo) Update(_ context.Context, doc *domain.Document) error {
	cp := *doc
	r.docs[doc.ID] = &cp
	return nil
}
func (r *memoryDocRepo) GetByID(_ context.Context, id string) (*domain.Document, error) {
	doc, ok := r.docs[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *doc
	return &cp, nil
}
func (r *memoryDocRepo) ListByKnowledgeBase(_ context.Context, orgID, kbID string, limit int, cursor string) ([]domain.Document, error) {
	var out []domain.Document
	for _, doc := range r.docs {
		if doc.OrgID == orgID && doc.KnowledgeBaseID == kbID && doc.Status != domain.DocumentStatusDeleted {
			out = append(out, *doc)
		}
	}
	return out, nil
}
func (r *memoryDocRepo) ListPending(context.Context, int) ([]domain.Document, error) {
	var out []domain.Document
	for _, doc := range r.docs {
		if doc.Status == domain.DocumentStatusPending || doc.Status == domain.DocumentStatusProcessing {
			out = append(out, *doc)
		}
	}
	return out, nil
}
func (r *memoryDocRepo) DeleteChunks(context.Context, string) error { return nil }

type memoryChunkRepo struct{ chunks []domain.DocumentChunk }

func (r *memoryChunkRepo) CreateBatch(_ context.Context, chunks []domain.DocumentChunk) error {
	r.chunks = append(r.chunks, chunks...)
	return nil
}
func (r *memoryChunkRepo) ReplaceByDocument(_ context.Context, documentID string, chunks []domain.DocumentChunk) error {
	next := r.chunks[:0]
	for _, ch := range r.chunks {
		if ch.DocumentID != documentID {
			next = append(next, ch)
		}
	}
	r.chunks = append(next, chunks...)
	return nil
}
func (r *memoryChunkRepo) Search(_ context.Context, in domain.ChunkSearchInput) ([]domain.KnowledgeSearchResult, error) {
	var out []domain.KnowledgeSearchResult
	allowed := map[string]bool{}
	for _, id := range in.KnowledgeBaseIDs {
		allowed[id] = true
	}
	for _, ch := range r.chunks {
		if ch.OrgID != in.OrgID || ch.ProjectID != in.ProjectID || ch.Status == domain.DocumentChunkStatusDeleted {
			continue
		}
		if len(allowed) > 0 && !allowed[ch.KnowledgeBaseID] {
			continue
		}
		if strings.Contains(ch.Content, in.Query) {
			out = append(out, domain.KnowledgeSearchResult{ChunkID: ch.ID, DocumentID: ch.DocumentID, FileID: ch.FileID, KnowledgeBaseID: ch.KnowledgeBaseID, Content: ch.Content, Score: 0.9, Metadata: ch.Metadata})
		}
	}
	return out, nil
}

type memoryKnowledgeFileRepo struct{ files map[string]*domain.File }

func (r *memoryKnowledgeFileRepo) Create(context.Context, *domain.File) error { return nil }
func (r *memoryKnowledgeFileRepo) Update(context.Context, *domain.File) error { return nil }
func (r *memoryKnowledgeFileRepo) GetByID(_ context.Context, id string) (*domain.File, error) {
	f, ok := r.files[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *f
	return &cp, nil
}
func (r *memoryKnowledgeFileRepo) ListByProject(context.Context, string, string, int) ([]domain.File, error) {
	return nil, nil
}

type memoryKnowledgeObjectRepo struct{ objects map[string]*domain.FileObject }

func (r *memoryKnowledgeObjectRepo) Create(context.Context, *domain.FileObject) error { return nil }
func (r *memoryKnowledgeObjectRepo) GetByFileID(_ context.Context, fileID string) (*domain.FileObject, error) {
	obj, ok := r.objects[fileID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *obj
	return &cp, nil
}

type memoryKnowledgeProjectRepo struct{ projects map[string]*domain.Project }

func (r *memoryKnowledgeProjectRepo) Create(context.Context, *domain.Project) error { return nil }
func (r *memoryKnowledgeProjectRepo) GetByID(_ context.Context, id string) (*domain.Project, error) {
	p, ok := r.projects[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *p
	return &cp, nil
}
func (r *memoryKnowledgeProjectRepo) ListByOrg(context.Context, string) ([]domain.Project, error) {
	return nil, nil
}
func (r *memoryKnowledgeProjectRepo) Update(context.Context, *domain.Project) error { return nil }
func (r *memoryKnowledgeProjectRepo) Delete(context.Context, string) error          { return nil }

type memoryKnowledgeMemberRepo struct{ members map[string]*domain.OrgMember }

func (r *memoryKnowledgeMemberRepo) Upsert(context.Context, *domain.OrgMember) error { return nil }
func (r *memoryKnowledgeMemberRepo) Get(_ context.Context, orgID, userID string) (*domain.OrgMember, error) {
	m, ok := r.members[orgID+"/"+userID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *m
	return &cp, nil
}
func (r *memoryKnowledgeMemberRepo) ListByOrg(context.Context, string) ([]domain.OrgMember, error) {
	return nil, nil
}
func (r *memoryKnowledgeMemberRepo) Remove(context.Context, string, string) error { return nil }

type memoryKnowledgeAudit struct{}

func (memoryKnowledgeAudit) Create(context.Context, *domain.AuditLog) error { return nil }
func (memoryKnowledgeAudit) ListByOrg(context.Context, string, int) ([]domain.AuditLog, error) {
	return nil, nil
}

type memoryKnowledgeUsage struct{}

func (memoryKnowledgeUsage) Create(context.Context, *domain.UsageRecord) error { return nil }
func (memoryKnowledgeUsage) ListByOrg(context.Context, string, int) ([]domain.UsageRecord, error) {
	return nil, nil
}

type memoryKnowledgeStore struct{ objects map[string][]byte }

func (s *memoryKnowledgeStore) PutObject(context.Context, storage.PutObjectInput) (*storage.PutObjectOutput, error) {
	return nil, nil
}
func (s *memoryKnowledgeStore) GetObject(_ context.Context, in storage.GetObjectInput) (*storage.GetObjectOutput, error) {
	data, ok := s.objects[in.Bucket+"/"+in.Key]
	if !ok {
		return nil, errors.New("not found")
	}
	return &storage.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(data)), Size: int64(len(data)), ContentType: "text/plain"}, nil
}
func (s *memoryKnowledgeStore) DeleteObject(context.Context, storage.DeleteObjectInput) error {
	return nil
}
func (s *memoryKnowledgeStore) StatObject(context.Context, storage.StatObjectInput) (*storage.ObjectInfo, error) {
	return nil, nil
}
