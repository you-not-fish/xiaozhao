package v1

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	fileapp "github.com/xiaozhao/xiaozhao/internal/app/file"
	"github.com/xiaozhao/xiaozhao/internal/app/rbac"
	"github.com/xiaozhao/xiaozhao/internal/domain"
	"github.com/xiaozhao/xiaozhao/internal/gateway/httpctx"
	"github.com/xiaozhao/xiaozhao/internal/infra/storage"
)

func TestFileUploadRequiresAuth(t *testing.T) {
	h, _, _, _ := newFileHandlerTestHarness(1024)
	req := newMultipartFileRequest(t, "/v1/files", map[string]string{"project_id": "proj_1", "purpose": "conversation"}, "file", "a.txt", []byte("a"))
	rr := httptest.NewRecorder()

	h.Upload(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestFileUploadRequiresTenant(t *testing.T) {
	h, _, _, _ := newFileHandlerTestHarness(1024)
	req := newMultipartFileRequest(t, "/v1/files", map[string]string{"project_id": "proj_1", "purpose": "conversation"}, "file", "a.txt", []byte("a"))
	req = req.WithContext(httpctx.WithUserID(req.Context(), "user_1"))
	rr := httptest.NewRecorder()

	h.Upload(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestFileUploadRequiresMultipartFile(t *testing.T) {
	h, _, _, _ := newFileHandlerTestHarness(1024)
	req := newMultipartFileRequest(t, "/v1/files", map[string]string{"project_id": "proj_1", "purpose": "conversation"}, "", "", nil)
	req = withUserOrg(req, "user_1", "org_1")
	rr := httptest.NewRecorder()

	h.Upload(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
}

func TestFileContentStreamsObject(t *testing.T) {
	h, files, objects, store := newFileHandlerTestHarness(1024)
	f := &domain.File{
		ID:        "file_1",
		OrgID:     "org_1",
		ProjectID: "proj_1",
		UserID:    "user_1",
		Filename:  "a.txt",
		MimeType:  "text/plain",
		SizeBytes: 5,
		Purpose:   domain.FilePurposeConversation,
		Status:    domain.FileStatusUploaded,
		CreatedAt: time.Now(),
	}
	if err := files.Create(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	obj := &domain.FileObject{ID: "fobj_1", FileID: "file_1", Bucket: "bucket", ObjectKey: "files/file_1/a.txt", StorageProvider: "fake"}
	if err := objects.Create(context.Background(), obj); err != nil {
		t.Fatal(err)
	}
	store.objects[obj.Bucket+"/"+obj.ObjectKey] = fileHandlerStoredObject{data: []byte("hello"), contentType: "text/plain"}

	req := httptest.NewRequest(http.MethodGet, "/v1/files/file_1/content", nil)
	req = withURLParam(req, "fileID", "file_1")
	req = withUserOrg(req, "user_1", "org_1")
	rr := httptest.NewRecorder()

	h.Content(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Body.String(); got != "hello" {
		t.Fatalf("body = %q, want hello", got)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("content-type = %q", ct)
	}
	if cd := rr.Header().Get("Content-Disposition"); !strings.Contains(cd, "a.txt") {
		t.Fatalf("content-disposition = %q", cd)
	}
}

func TestFileGetDoesNotLeakCrossTenantFile(t *testing.T) {
	h, files, _, _ := newFileHandlerTestHarness(1024)
	if err := files.Create(context.Background(), &domain.File{
		ID:        "file_2",
		OrgID:     "org_2",
		ProjectID: "proj_2",
		UserID:    "user_2",
		Filename:  "secret.txt",
		MimeType:  "text/plain",
		SizeBytes: 1,
		Purpose:   domain.FilePurposeConversation,
		Status:    domain.FileStatusUploaded,
		CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/files/file_2", nil)
	req = withURLParam(req, "fileID", "file_2")
	req = withUserOrg(req, "user_1", "org_1")
	rr := httptest.NewRecorder()

	h.Get(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rr.Code, rr.Body.String())
	}
}

func newFileHandlerTestHarness(maxUpload int64) (*FileHandler, *fileHandlerFileRepo, *fileHandlerObjectRepo, *fileHandlerStore) {
	files := &fileHandlerFileRepo{files: map[string]*domain.File{}}
	objects := &fileHandlerObjectRepo{objects: map[string]*domain.FileObject{}}
	projects := &fileHandlerProjectRepo{projects: map[string]*domain.Project{
		"proj_1": {ID: "proj_1", OrgID: "org_1", Name: "P1"},
		"proj_2": {ID: "proj_2", OrgID: "org_2", Name: "P2"},
	}}
	members := &fileHandlerMemberRepo{members: map[string]*domain.OrgMember{
		"org_1/user_1": {OrgID: "org_1", UserID: "user_1", Role: domain.RoleAdmin, Status: domain.MemberStatusActive},
		"org_2/user_2": {OrgID: "org_2", UserID: "user_2", Role: domain.RoleAdmin, Status: domain.MemberStatusActive},
	}}
	store := &fileHandlerStore{objects: map[string]fileHandlerStoredObject{}}
	svc := fileapp.NewService(files, objects, projects, fileHandlerAuditRepo{}, rbac.NewChecker(members), store, fileapp.Options{
		Bucket:         "bucket",
		Provider:       "fake",
		MaxUploadBytes: maxUpload,
	})
	return NewFileHandler(svc, maxUpload), files, objects, store
}

func newMultipartFileRequest(t *testing.T, target string, fields map[string]string, fileField, filename string, content []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for k, v := range fields {
		if err := writer.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if fileField != "" {
		part, err := writer.CreateFormFile(fileField, filename)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, target, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func withUserOrg(req *http.Request, userID, orgID string) *http.Request {
	ctx := httpctx.WithUserID(req.Context(), userID)
	ctx = httpctx.WithOrgID(ctx, orgID)
	return req.WithContext(ctx)
}

type fileHandlerFileRepo struct {
	files map[string]*domain.File
}

func (r *fileHandlerFileRepo) Create(_ context.Context, f *domain.File) error {
	cp := *f
	r.files[f.ID] = &cp
	return nil
}

func (r *fileHandlerFileRepo) Update(_ context.Context, f *domain.File) error {
	if _, ok := r.files[f.ID]; !ok {
		return domain.ErrNotFound
	}
	cp := *f
	r.files[f.ID] = &cp
	return nil
}

func (r *fileHandlerFileRepo) GetByID(_ context.Context, id string) (*domain.File, error) {
	f, ok := r.files[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *f
	return &cp, nil
}

func (r *fileHandlerFileRepo) ListByProject(_ context.Context, orgID, projectID string, limit int) ([]domain.File, error) {
	var out []domain.File
	for _, f := range r.files {
		if f.OrgID == orgID && f.ProjectID == projectID && f.Status != domain.FileStatusDeleted {
			out = append(out, *f)
		}
	}
	return out, nil
}

type fileHandlerObjectRepo struct {
	objects map[string]*domain.FileObject
}

func (r *fileHandlerObjectRepo) Create(_ context.Context, obj *domain.FileObject) error {
	cp := *obj
	r.objects[obj.FileID] = &cp
	return nil
}

func (r *fileHandlerObjectRepo) GetByFileID(_ context.Context, fileID string) (*domain.FileObject, error) {
	obj, ok := r.objects[fileID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *obj
	return &cp, nil
}

type fileHandlerProjectRepo struct {
	projects map[string]*domain.Project
}

func (r *fileHandlerProjectRepo) Create(_ context.Context, p *domain.Project) error {
	cp := *p
	r.projects[p.ID] = &cp
	return nil
}

func (r *fileHandlerProjectRepo) GetByID(_ context.Context, id string) (*domain.Project, error) {
	p, ok := r.projects[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *p
	return &cp, nil
}

func (r *fileHandlerProjectRepo) ListByOrg(_ context.Context, orgID string) ([]domain.Project, error) {
	var out []domain.Project
	for _, p := range r.projects {
		if p.OrgID == orgID {
			out = append(out, *p)
		}
	}
	return out, nil
}

func (r *fileHandlerProjectRepo) Update(_ context.Context, p *domain.Project) error {
	cp := *p
	r.projects[p.ID] = &cp
	return nil
}

func (r *fileHandlerProjectRepo) Delete(_ context.Context, id string) error {
	delete(r.projects, id)
	return nil
}

type fileHandlerMemberRepo struct {
	members map[string]*domain.OrgMember
}

func (r *fileHandlerMemberRepo) Upsert(_ context.Context, m *domain.OrgMember) error {
	cp := *m
	r.members[m.OrgID+"/"+m.UserID] = &cp
	return nil
}

func (r *fileHandlerMemberRepo) Get(_ context.Context, orgID, userID string) (*domain.OrgMember, error) {
	m, ok := r.members[orgID+"/"+userID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *m
	return &cp, nil
}

func (r *fileHandlerMemberRepo) ListByOrg(_ context.Context, orgID string) ([]domain.OrgMember, error) {
	var out []domain.OrgMember
	for _, m := range r.members {
		if m.OrgID == orgID {
			out = append(out, *m)
		}
	}
	return out, nil
}

func (r *fileHandlerMemberRepo) Remove(_ context.Context, orgID, userID string) error {
	delete(r.members, orgID+"/"+userID)
	return nil
}

type fileHandlerAuditRepo struct{}

func (fileHandlerAuditRepo) Create(context.Context, *domain.AuditLog) error { return nil }
func (fileHandlerAuditRepo) ListByOrg(context.Context, string, int) ([]domain.AuditLog, error) {
	return nil, nil
}

type fileHandlerStoredObject struct {
	data        []byte
	contentType string
}

type fileHandlerStore struct {
	objects map[string]fileHandlerStoredObject
}

func (s *fileHandlerStore) PutObject(_ context.Context, in storage.PutObjectInput) (*storage.PutObjectOutput, error) {
	data, err := io.ReadAll(in.Reader)
	if err != nil {
		return nil, err
	}
	s.objects[in.Bucket+"/"+in.Key] = fileHandlerStoredObject{data: data, contentType: in.ContentType}
	return &storage.PutObjectOutput{ETag: "etag"}, nil
}

func (s *fileHandlerStore) GetObject(_ context.Context, in storage.GetObjectInput) (*storage.GetObjectOutput, error) {
	obj, ok := s.objects[in.Bucket+"/"+in.Key]
	if !ok {
		return nil, errors.New("not found")
	}
	return &storage.GetObjectOutput{
		Body:        io.NopCloser(bytes.NewReader(obj.data)),
		Size:        int64(len(obj.data)),
		ContentType: obj.contentType,
		ETag:        "etag",
	}, nil
}

func (s *fileHandlerStore) DeleteObject(_ context.Context, in storage.DeleteObjectInput) error {
	delete(s.objects, in.Bucket+"/"+in.Key)
	return nil
}

func (s *fileHandlerStore) StatObject(_ context.Context, in storage.StatObjectInput) (*storage.ObjectInfo, error) {
	obj, ok := s.objects[in.Bucket+"/"+in.Key]
	if !ok {
		return nil, errors.New("not found")
	}
	return &storage.ObjectInfo{Size: int64(len(obj.data)), ContentType: obj.contentType, ETag: "etag"}, nil
}
