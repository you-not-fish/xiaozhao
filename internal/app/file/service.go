// Package file implements uploaded file lifecycle.
package file

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/xiaozhao/xiaozhao/internal/app/rbac"
	"github.com/xiaozhao/xiaozhao/internal/domain"
	"github.com/xiaozhao/xiaozhao/internal/infra/storage"
	"github.com/xiaozhao/xiaozhao/internal/pkg/errcode"
	"github.com/xiaozhao/xiaozhao/internal/pkg/id"
)

type Service struct {
	files     domain.FileRepository
	objects   domain.FileObjectRepository
	projects  domain.ProjectRepository
	audit     domain.AuditLogRepository
	rbac      *rbac.Checker
	store     storage.Store
	bucket    string
	provider  string
	maxUpload int64
}

type Options struct {
	Bucket         string
	Provider       string
	MaxUploadBytes int64
}

func NewService(
	files domain.FileRepository,
	objects domain.FileObjectRepository,
	projects domain.ProjectRepository,
	audit domain.AuditLogRepository,
	rbacChecker *rbac.Checker,
	store storage.Store,
	opts Options,
) *Service {
	if opts.Provider == "" {
		opts.Provider = "minio"
	}
	if opts.MaxUploadBytes <= 0 {
		opts.MaxUploadBytes = 50 * 1024 * 1024
	}
	return &Service{
		files:     files,
		objects:   objects,
		projects:  projects,
		audit:     audit,
		rbac:      rbacChecker,
		store:     store,
		bucket:    opts.Bucket,
		provider:  opts.Provider,
		maxUpload: opts.MaxUploadBytes,
	}
}

type UploadInput struct {
	OrgID      string
	ProjectID  string
	UserID     string
	Filename   string
	HeaderMIME string
	SizeBytes  int64
	Purpose    domain.FilePurpose
	Body       io.Reader
}

type DownloadResult struct {
	File        *domain.File
	Object      *domain.FileObject
	Body        io.ReadCloser
	ContentType string
	Size        int64
}

func (s *Service) Upload(ctx context.Context, in UploadInput) (*domain.File, error) {
	if in.ProjectID == "" {
		return nil, errcode.New(errcode.CodeInvalidArgument, "project_id is required")
	}
	if err := s.authorizeProject(ctx, in.OrgID, in.ProjectID, in.UserID); err != nil {
		return nil, err
	}
	if !in.Purpose.Valid() {
		return nil, errcode.New(errcode.CodeInvalidArgument, "invalid file purpose")
	}
	filename := sanitizeFilename(in.Filename)
	if filename == "" {
		return nil, errcode.New(errcode.CodeInvalidArgument, "filename is required")
	}
	if in.SizeBytes <= 0 {
		return nil, errcode.New(errcode.CodeInvalidArgument, "file must not be empty")
	}
	if in.SizeBytes > s.maxUpload {
		return nil, errcode.Newf(errcode.CodeInvalidArgument, "file exceeds max upload size %d bytes", s.maxUpload)
	}

	buffered := bufio.NewReader(in.Body)
	head, _ := buffered.Peek(512)
	// 不能只信任浏览器传来的 Content-Type；这里用文件头 sniff 得到服务端判断。
	mimeType := http.DetectContentType(head)
	if len(head) == 0 {
		return nil, errcode.New(errcode.CodeInvalidArgument, "file must not be empty")
	}
	if err := validateExtension(filename); err != nil {
		return nil, err
	}

	fileID := id.New(id.PrefixFile)
	f := &domain.File{
		ID:        fileID,
		OrgID:     in.OrgID,
		ProjectID: in.ProjectID,
		UserID:    in.UserID,
		Filename:  filename,
		MimeType:  mimeType,
		SizeBytes: in.SizeBytes,
		Purpose:   in.Purpose,
		Status:    domain.FileStatusUploading,
		CreatedAt: time.Now(),
	}
	// 先创建 uploading 记录，再写对象存储：失败时能留下审计和重试线索。
	if err := s.files.Create(ctx, f); err != nil {
		return nil, errcode.Wrap(err, errcode.CodeInternal, "create file metadata")
	}

	key := objectKey(in.OrgID, in.ProjectID, fileID, filename)
	hash := sha256.New()
	reader := io.TeeReader(buffered, hash)
	put, err := s.store.PutObject(ctx, storage.PutObjectInput{
		Bucket:      s.bucket,
		Key:         key,
		Reader:      reader,
		Size:        in.SizeBytes,
		ContentType: mimeType,
	})
	if err != nil {
		f.Status = domain.FileStatusFailed
		_ = s.files.Update(context.Background(), f)
		return nil, errcode.Wrap(err, errcode.CodeUnavailable, "upload object")
	}

	f.SHA256 = hex.EncodeToString(hash.Sum(nil))
	f.Status = domain.FileStatusUploaded
	if err := s.files.Update(ctx, f); err != nil {
		_ = s.store.DeleteObject(context.Background(), storage.DeleteObjectInput{Bucket: s.bucket, Key: key})
		return nil, errcode.Wrap(err, errcode.CodeInternal, "update file metadata")
	}
	obj := &domain.FileObject{
		ID:              id.New(id.PrefixFileObject),
		FileID:          fileID,
		StorageProvider: s.provider,
		Bucket:          s.bucket,
		ObjectKey:       key,
		ETag:            put.ETag,
		VersionID:       put.VersionID,
		CreatedAt:       time.Now(),
	}
	if err := s.objects.Create(ctx, obj); err != nil {
		f.Status = domain.FileStatusFailed
		_ = s.files.Update(context.Background(), f)
		_ = s.store.DeleteObject(context.Background(), storage.DeleteObjectInput{Bucket: s.bucket, Key: key})
		return nil, errcode.Wrap(err, errcode.CodeInternal, "create file object")
	}
	s.auditLog(in.OrgID, in.ProjectID, in.UserID, "file.uploaded", fileID, map[string]any{
		"filename":   filename,
		"mime_type":  mimeType,
		"size_bytes": in.SizeBytes,
		"sha256":     f.SHA256,
		"object_key": key,
	})
	return f, nil
}

func (s *Service) List(ctx context.Context, orgID, projectID, userID string, limit int) ([]domain.File, error) {
	if projectID == "" {
		return nil, errcode.New(errcode.CodeInvalidArgument, "project_id is required")
	}
	if err := s.authorizeProject(ctx, orgID, projectID, userID); err != nil {
		return nil, err
	}
	files, err := s.files.ListByProject(ctx, orgID, projectID, limit)
	if err != nil {
		return nil, errcode.Wrap(err, errcode.CodeInternal, "list files")
	}
	return files, nil
}

func (s *Service) Get(ctx context.Context, orgID, userID, fileID string) (*domain.File, error) {
	f, err := s.loadVisibleFile(ctx, orgID, userID, fileID)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (s *Service) Download(ctx context.Context, orgID, userID, fileID string) (*DownloadResult, error) {
	f, err := s.loadVisibleFile(ctx, orgID, userID, fileID)
	if err != nil {
		return nil, err
	}
	obj, err := s.objects.GetByFileID(ctx, fileID)
	if err != nil {
		return nil, errcode.New(errcode.CodeNotFound, "file object not found")
	}
	out, err := s.store.GetObject(ctx, storage.GetObjectInput{Bucket: obj.Bucket, Key: obj.ObjectKey})
	if err != nil {
		return nil, errcode.Wrap(err, errcode.CodeUnavailable, "download object")
	}
	// MVP 下载走后端代理，先复用已有鉴权/审计边界；预签名 URL 留到后续版本。
	return &DownloadResult{File: f, Object: obj, Body: out.Body, ContentType: out.ContentType, Size: out.Size}, nil
}

func (s *Service) Delete(ctx context.Context, orgID, userID, fileID string) error {
	f, err := s.loadVisibleFile(ctx, orgID, userID, fileID)
	if err != nil {
		return err
	}
	if f.UserID != userID {
		if _, err := s.rbac.Require(ctx, f.OrgID, userID, domain.RoleAdmin); err != nil {
			return err
		}
	}
	obj, _ := s.objects.GetByFileID(ctx, fileID)
	f.Status = domain.FileStatusDeleted
	if err := s.files.Update(ctx, f); err != nil {
		return errcode.Wrap(err, errcode.CodeInternal, "delete file metadata")
	}
	if obj != nil {
		// 对象存储删除失败不阻断软删除；元数据已不可见，后台可按审计补偿清理。
		_ = s.store.DeleteObject(context.Background(), storage.DeleteObjectInput{Bucket: obj.Bucket, Key: obj.ObjectKey})
	}
	s.auditLog(f.OrgID, f.ProjectID, userID, "file.deleted", fileID, nil)
	return nil
}

func (s *Service) loadVisibleFile(ctx context.Context, orgID, userID, fileID string) (*domain.File, error) {
	f, err := s.files.GetByID(ctx, fileID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, errcode.New(errcode.CodeNotFound, "file not found")
		}
		return nil, errcode.Wrap(err, errcode.CodeInternal, "get file")
	}
	if f.OrgID != orgID || f.Status == domain.FileStatusDeleted {
		return nil, errcode.New(errcode.CodeNotFound, "file not found")
	}
	if err := s.authorizeProject(ctx, f.OrgID, f.ProjectID, userID); err != nil {
		return nil, errcode.New(errcode.CodeNotFound, "file not found")
	}
	return f, nil
}

func (s *Service) authorizeProject(ctx context.Context, orgID, projectID, userID string) error {
	if projectID == "" {
		return errcode.New(errcode.CodeInvalidArgument, "project_id is required")
	}
	if _, err := s.rbac.Require(ctx, orgID, userID, domain.RoleViewer); err != nil {
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

func (s *Service) auditLog(orgID, projectID, userID, action, fileID string, metadata map[string]any) {
	if s.audit == nil {
		return
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	_ = s.audit.Create(context.Background(), &domain.AuditLog{
		ID:           id.New(id.PrefixAudit),
		OrgID:        orgID,
		ProjectID:    projectID,
		ActorUserID:  userID,
		Action:       action,
		ResourceType: "file",
		ResourceID:   fileID,
		Metadata:     metadata,
		CreatedAt:    time.Now(),
	})
}

var safeNameRE = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func sanitizeFilename(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = strings.Trim(name, ". ")
	name = safeNameRE.ReplaceAllString(name, "_")
	if len(name) > 180 {
		ext := filepath.Ext(name)
		base := strings.TrimSuffix(name, ext)
		if len(ext) > 20 {
			ext = ""
		}
		if len(base) > 160 {
			base = base[:160]
		}
		name = base + ext
	}
	return name
}

func objectKey(orgID, projectID, fileID, filename string) string {
	// object key 不使用用户原始路径，避免路径穿越、同名覆盖和跨租户目录混淆。
	return fmt.Sprintf("orgs/%s/projects/%s/files/%s/%s", orgID, projectID, fileID, filename)
}

func validateExtension(filename string) error {
	ext := strings.ToLower(filepath.Ext(filename))
	blocked := map[string]bool{
		".exe": true, ".dll": true, ".dylib": true, ".so": true, ".sh": true,
		".bat": true, ".cmd": true, ".com": true, ".scr": true,
	}
	if blocked[ext] {
		return errcode.New(errcode.CodeInvalidArgument, "file type is not allowed")
	}
	return nil
}
