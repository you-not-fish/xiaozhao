package file

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/xiaozhao/xiaozhao/internal/app/rbac"
	"github.com/xiaozhao/xiaozhao/internal/domain"
	"github.com/xiaozhao/xiaozhao/internal/infra/storage"
	"github.com/xiaozhao/xiaozhao/internal/pkg/errcode"
)

func TestUploadSuccessStoresMetadataAndObject(t *testing.T) {
	svc, files, objects, store, audit := newTestService(1024)
	content := []byte("hello knowledge base")

	f, err := svc.Upload(context.Background(), UploadInput{
		OrgID:     "org_1",
		ProjectID: "proj_1",
		UserID:    "user_1",
		Filename:  "../report name.txt",
		SizeBytes: int64(len(content)),
		Purpose:   domain.FilePurposeKnowledgeBase,
		Body:      bytes.NewReader(content),
	})
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if f.Status != domain.FileStatusUploaded {
		t.Fatalf("status = %s, want uploaded", f.Status)
	}
	if f.Filename != "report_name.txt" {
		t.Fatalf("filename = %q", f.Filename)
	}
	wantHash := sha256.Sum256(content)
	if f.SHA256 != hex.EncodeToString(wantHash[:]) {
		t.Fatalf("sha256 = %s", f.SHA256)
	}
	stored, ok := files.files[f.ID]
	if !ok || stored.Status != domain.FileStatusUploaded {
		t.Fatalf("metadata not uploaded: %#v", stored)
	}
	obj, err := objects.GetByFileID(context.Background(), f.ID)
	if err != nil {
		t.Fatalf("file object missing: %v", err)
	}
	if !strings.HasPrefix(obj.ObjectKey, "orgs/org_1/projects/proj_1/files/"+f.ID+"/") {
		t.Fatalf("object key = %q", obj.ObjectKey)
	}
	if got := string(store.objects[obj.Bucket+"/"+obj.ObjectKey].data); got != string(content) {
		t.Fatalf("stored object = %q", got)
	}
	if len(audit.logs) != 1 || audit.logs[0].Action != "file.uploaded" {
		t.Fatalf("audit logs = %#v", audit.logs)
	}
}

func TestUploadRejectsTooLargeAndEmptyFiles(t *testing.T) {
	svc, files, _, store, _ := newTestService(4)

	_, err := svc.Upload(context.Background(), UploadInput{
		OrgID:     "org_1",
		ProjectID: "proj_1",
		UserID:    "user_1",
		Filename:  "large.txt",
		SizeBytes: 5,
		Purpose:   domain.FilePurposeConversation,
		Body:      bytes.NewReader([]byte("12345")),
	})
	if errcode.CodeOf(err) != errcode.CodeInvalidArgument {
		t.Fatalf("large error code = %s, want invalid_argument", errcode.CodeOf(err))
	}
	if len(files.files) != 0 || len(store.objects) != 0 {
		t.Fatalf("too-large upload wrote metadata/object")
	}

	_, err = svc.Upload(context.Background(), UploadInput{
		OrgID:     "org_1",
		ProjectID: "proj_1",
		UserID:    "user_1",
		Filename:  "empty.txt",
		SizeBytes: 0,
		Purpose:   domain.FilePurposeConversation,
		Body:      bytes.NewReader(nil),
	})
	if errcode.CodeOf(err) != errcode.CodeInvalidArgument {
		t.Fatalf("empty error code = %s, want invalid_argument", errcode.CodeOf(err))
	}
}

func TestUploadSniffsMIMEInsteadOfTrustingHeader(t *testing.T) {
	svc, _, _, _, _ := newTestService(1024)
	content := []byte("plain text content")

	f, err := svc.Upload(context.Background(), UploadInput{
		OrgID:      "org_1",
		ProjectID:  "proj_1",
		UserID:     "user_1",
		Filename:   "note.txt",
		HeaderMIME: "application/pdf",
		SizeBytes:  int64(len(content)),
		Purpose:    domain.FilePurposeUserData,
		Body:       bytes.NewReader(content),
	})
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if f.MimeType == "application/pdf" || !strings.HasPrefix(f.MimeType, "text/plain") {
		t.Fatalf("mime type = %q, want sniffed text/plain", f.MimeType)
	}
}

func TestUploadMarksFailedWhenObjectStorageFails(t *testing.T) {
	svc, files, _, store, _ := newTestService(1024)
	store.failPut = true
	content := []byte("hello")

	_, err := svc.Upload(context.Background(), UploadInput{
		OrgID:     "org_1",
		ProjectID: "proj_1",
		UserID:    "user_1",
		Filename:  "fail.txt",
		SizeBytes: int64(len(content)),
		Purpose:   domain.FilePurposeConversation,
		Body:      bytes.NewReader(content),
	})
	if errcode.CodeOf(err) != errcode.CodeUnavailable {
		t.Fatalf("error code = %s, want service_unavailable", errcode.CodeOf(err))
	}
	if len(files.files) != 1 {
		t.Fatalf("file metadata count = %d, want 1", len(files.files))
	}
	for _, f := range files.files {
		if f.Status != domain.FileStatusFailed {
			t.Fatalf("status = %s, want failed", f.Status)
		}
	}
}

func TestDeleteSoftDeletesAndBestEffortDeletesObject(t *testing.T) {
	svc, files, objects, store, _ := newTestService(1024)
	f := &domain.File{
		ID:        "file_1",
		OrgID:     "org_1",
		ProjectID: "proj_1",
		UserID:    "user_1",
		Filename:  "a.txt",
		MimeType:  "text/plain",
		SizeBytes: 1,
		Purpose:   domain.FilePurposeConversation,
		Status:    domain.FileStatusUploaded,
		CreatedAt: time.Now(),
	}
	if err := files.Create(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	obj := &domain.FileObject{ID: "fobj_1", FileID: "file_1", Bucket: "bucket", ObjectKey: "orgs/org_1/projects/proj_1/files/file_1/a.txt", StorageProvider: "fake"}
	if err := objects.Create(context.Background(), obj); err != nil {
		t.Fatal(err)
	}
	store.objects[obj.Bucket+"/"+obj.ObjectKey] = storedObject{data: []byte("a"), contentType: "text/plain"}

	if err := svc.Delete(context.Background(), "org_1", "user_1", "file_1"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if files.files["file_1"].Status != domain.FileStatusDeleted {
		t.Fatalf("status = %s, want deleted", files.files["file_1"].Status)
	}
	if len(store.deleted) != 1 {
		t.Fatalf("deleted objects = %#v", store.deleted)
	}
}

func TestCrossTenantGetDoesNotLeakFile(t *testing.T) {
	svc, files, _, _, _ := newTestService(1024)
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

	_, err := svc.Get(context.Background(), "org_1", "user_1", "file_2")
	if errcode.CodeOf(err) != errcode.CodeNotFound {
		t.Fatalf("error code = %s, want not_found", errcode.CodeOf(err))
	}
}

func newTestService(maxUpload int64) (*Service, *memoryFileRepo, *memoryFileObjectRepo, *memoryStore, *memoryAuditRepo) {
	files := &memoryFileRepo{files: map[string]*domain.File{}}
	objects := &memoryFileObjectRepo{objects: map[string]*domain.FileObject{}}
	projects := &memoryProjectRepo{projects: map[string]*domain.Project{
		"proj_1": {ID: "proj_1", OrgID: "org_1", Name: "P1"},
		"proj_2": {ID: "proj_2", OrgID: "org_2", Name: "P2"},
	}}
	members := &memoryMemberRepo{members: map[string]*domain.OrgMember{
		"org_1/user_1": {OrgID: "org_1", UserID: "user_1", Role: domain.RoleAdmin, Status: domain.MemberStatusActive},
		"org_2/user_2": {OrgID: "org_2", UserID: "user_2", Role: domain.RoleAdmin, Status: domain.MemberStatusActive},
	}}
	audit := &memoryAuditRepo{}
	store := &memoryStore{objects: map[string]storedObject{}}
	svc := NewService(files, objects, projects, audit, rbac.NewChecker(members), store, Options{
		Bucket:         "bucket",
		Provider:       "fake",
		MaxUploadBytes: maxUpload,
	})
	return svc, files, objects, store, audit
}

type memoryFileRepo struct {
	files map[string]*domain.File
}

func (r *memoryFileRepo) Create(_ context.Context, f *domain.File) error {
	cp := *f
	r.files[f.ID] = &cp
	return nil
}

func (r *memoryFileRepo) Update(_ context.Context, f *domain.File) error {
	if _, ok := r.files[f.ID]; !ok {
		return domain.ErrNotFound
	}
	cp := *f
	r.files[f.ID] = &cp
	return nil
}

func (r *memoryFileRepo) GetByID(_ context.Context, id string) (*domain.File, error) {
	f, ok := r.files[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *f
	return &cp, nil
}

func (r *memoryFileRepo) ListByProject(_ context.Context, orgID, projectID string, limit int) ([]domain.File, error) {
	out := []domain.File{}
	for _, f := range r.files {
		if f.OrgID == orgID && f.ProjectID == projectID && f.Status != domain.FileStatusDeleted {
			out = append(out, *f)
		}
	}
	return out, nil
}

type memoryFileObjectRepo struct {
	objects map[string]*domain.FileObject
}

func (r *memoryFileObjectRepo) Create(_ context.Context, obj *domain.FileObject) error {
	cp := *obj
	r.objects[obj.FileID] = &cp
	return nil
}

func (r *memoryFileObjectRepo) GetByFileID(_ context.Context, fileID string) (*domain.FileObject, error) {
	obj, ok := r.objects[fileID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *obj
	return &cp, nil
}

type memoryProjectRepo struct {
	projects map[string]*domain.Project
}

func (r *memoryProjectRepo) Create(_ context.Context, p *domain.Project) error {
	cp := *p
	r.projects[p.ID] = &cp
	return nil
}

func (r *memoryProjectRepo) GetByID(_ context.Context, id string) (*domain.Project, error) {
	p, ok := r.projects[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *p
	return &cp, nil
}

func (r *memoryProjectRepo) ListByOrg(_ context.Context, orgID string) ([]domain.Project, error) {
	var out []domain.Project
	for _, p := range r.projects {
		if p.OrgID == orgID {
			out = append(out, *p)
		}
	}
	return out, nil
}

func (r *memoryProjectRepo) Update(_ context.Context, p *domain.Project) error {
	cp := *p
	r.projects[p.ID] = &cp
	return nil
}

func (r *memoryProjectRepo) Delete(_ context.Context, id string) error {
	delete(r.projects, id)
	return nil
}

type memoryMemberRepo struct {
	members map[string]*domain.OrgMember
}

func (r *memoryMemberRepo) Upsert(_ context.Context, m *domain.OrgMember) error {
	cp := *m
	r.members[m.OrgID+"/"+m.UserID] = &cp
	return nil
}

func (r *memoryMemberRepo) Get(_ context.Context, orgID, userID string) (*domain.OrgMember, error) {
	m, ok := r.members[orgID+"/"+userID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *m
	return &cp, nil
}

func (r *memoryMemberRepo) ListByOrg(_ context.Context, orgID string) ([]domain.OrgMember, error) {
	var out []domain.OrgMember
	for _, m := range r.members {
		if m.OrgID == orgID {
			out = append(out, *m)
		}
	}
	return out, nil
}

func (r *memoryMemberRepo) Remove(_ context.Context, orgID, userID string) error {
	delete(r.members, orgID+"/"+userID)
	return nil
}

type memoryAuditRepo struct {
	logs []domain.AuditLog
}

func (r *memoryAuditRepo) Create(_ context.Context, a *domain.AuditLog) error {
	r.logs = append(r.logs, *a)
	return nil
}

func (r *memoryAuditRepo) ListByOrg(_ context.Context, orgID string, limit int) ([]domain.AuditLog, error) {
	var out []domain.AuditLog
	for _, l := range r.logs {
		if l.OrgID == orgID {
			out = append(out, l)
		}
	}
	return out, nil
}

type storedObject struct {
	data        []byte
	contentType string
}

type memoryStore struct {
	objects map[string]storedObject
	deleted []string
	failPut bool
}

func (s *memoryStore) PutObject(_ context.Context, in storage.PutObjectInput) (*storage.PutObjectOutput, error) {
	if s.failPut {
		return nil, errors.New("put failed")
	}
	data, err := io.ReadAll(in.Reader)
	if err != nil {
		return nil, err
	}
	s.objects[in.Bucket+"/"+in.Key] = storedObject{data: data, contentType: in.ContentType}
	return &storage.PutObjectOutput{ETag: "etag"}, nil
}

func (s *memoryStore) GetObject(_ context.Context, in storage.GetObjectInput) (*storage.GetObjectOutput, error) {
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

func (s *memoryStore) DeleteObject(_ context.Context, in storage.DeleteObjectInput) error {
	key := in.Bucket + "/" + in.Key
	s.deleted = append(s.deleted, key)
	delete(s.objects, key)
	return nil
}

func (s *memoryStore) StatObject(_ context.Context, in storage.StatObjectInput) (*storage.ObjectInfo, error) {
	obj, ok := s.objects[in.Bucket+"/"+in.Key]
	if !ok {
		return nil, errors.New("not found")
	}
	return &storage.ObjectInfo{Size: int64(len(obj.data)), ContentType: obj.contentType, ETag: "etag"}, nil
}
