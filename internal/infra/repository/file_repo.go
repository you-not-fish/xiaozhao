package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/xiaozhao/xiaozhao/internal/domain"
)

type fileRow struct {
	ID        string    `gorm:"column:id;primaryKey;size:64"`
	OrgID     string    `gorm:"column:org_id;size:64;not null"`
	ProjectID string    `gorm:"column:project_id;size:64;not null"`
	UserID    string    `gorm:"column:user_id;size:64;not null"`
	Filename  string    `gorm:"column:filename;size:255;not null"`
	MimeType  string    `gorm:"column:mime_type;size:255;not null;default:application/octet-stream"`
	SizeBytes int64     `gorm:"column:size_bytes;not null;default:0"`
	SHA256    string    `gorm:"column:sha256;size:64;not null;default:''"`
	Purpose   string    `gorm:"column:purpose;size:32;not null"`
	Status    string    `gorm:"column:status;size:32;not null"`
	CreatedAt time.Time `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null;default:now()"`
}

func (fileRow) TableName() string { return "files" }

type fileObjectRow struct {
	ID              string    `gorm:"column:id;primaryKey;size:64"`
	FileID          string    `gorm:"column:file_id;size:64;not null"`
	StorageProvider string    `gorm:"column:storage_provider;size:32;not null"`
	Bucket          string    `gorm:"column:bucket;size:255;not null"`
	ObjectKey       string    `gorm:"column:object_key;type:text;not null"`
	ETag            string    `gorm:"column:etag;size:255"`
	VersionID       string    `gorm:"column:version_id;size:255"`
	CreatedAt       time.Time `gorm:"column:created_at;not null;default:now()"`
}

func (fileObjectRow) TableName() string { return "file_objects" }

type FileRepo struct{ db *gorm.DB }

func NewFileRepo(db *gorm.DB) *FileRepo { return &FileRepo{db: db} }

func (r *FileRepo) Create(ctx context.Context, f *domain.File) error {
	row := fileRowFromDomain(f)
	if row.CreatedAt.IsZero() {
		row.CreatedAt = now()
	}
	row.UpdatedAt = row.CreatedAt
	return r.db.WithContext(ctx).Create(&row).Error
}

func (r *FileRepo) Update(ctx context.Context, f *domain.File) error {
	row := fileRowFromDomain(f)
	row.UpdatedAt = now()
	res := r.db.WithContext(ctx).Model(&fileRow{}).Where("id = ?", f.ID).Updates(map[string]any{
		"filename":   row.Filename,
		"mime_type":  row.MimeType,
		"size_bytes": row.SizeBytes,
		"sha256":     row.SHA256,
		"purpose":    row.Purpose,
		"status":     row.Status,
		"updated_at": row.UpdatedAt,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	f.UpdatedAt = row.UpdatedAt
	return nil
}

func (r *FileRepo) GetByID(ctx context.Context, id string) (*domain.File, error) {
	var row fileRow
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return fileRowToDomain(row), nil
}

func (r *FileRepo) ListByProject(ctx context.Context, orgID, projectID string, limit int) ([]domain.File, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var rows []fileRow
	err := r.db.WithContext(ctx).
		Where("org_id = ? AND project_id = ? AND status <> ?", orgID, projectID, domain.FileStatusDeleted).
		Order("created_at DESC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.File, 0, len(rows))
	for _, row := range rows {
		out = append(out, *fileRowToDomain(row))
	}
	return out, nil
}

type FileObjectRepo struct{ db *gorm.DB }

func NewFileObjectRepo(db *gorm.DB) *FileObjectRepo { return &FileObjectRepo{db: db} }

func (r *FileObjectRepo) Create(ctx context.Context, obj *domain.FileObject) error {
	row := fileObjectRow{
		ID:              obj.ID,
		FileID:          obj.FileID,
		StorageProvider: obj.StorageProvider,
		Bucket:          obj.Bucket,
		ObjectKey:       obj.ObjectKey,
		ETag:            obj.ETag,
		VersionID:       obj.VersionID,
		CreatedAt:       obj.CreatedAt,
	}
	if row.CreatedAt.IsZero() {
		row.CreatedAt = now()
	}
	return r.db.WithContext(ctx).Create(&row).Error
}

func (r *FileObjectRepo) GetByFileID(ctx context.Context, fileID string) (*domain.FileObject, error) {
	var row fileObjectRow
	err := r.db.WithContext(ctx).Where("file_id = ?", fileID).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &domain.FileObject{
		ID:              row.ID,
		FileID:          row.FileID,
		StorageProvider: row.StorageProvider,
		Bucket:          row.Bucket,
		ObjectKey:       row.ObjectKey,
		ETag:            row.ETag,
		VersionID:       row.VersionID,
		CreatedAt:       row.CreatedAt,
	}, nil
}

func fileRowFromDomain(f *domain.File) fileRow {
	return fileRow{
		ID:        f.ID,
		OrgID:     f.OrgID,
		ProjectID: f.ProjectID,
		UserID:    f.UserID,
		Filename:  f.Filename,
		MimeType:  f.MimeType,
		SizeBytes: f.SizeBytes,
		SHA256:    f.SHA256,
		Purpose:   string(f.Purpose),
		Status:    string(f.Status),
		CreatedAt: f.CreatedAt,
		UpdatedAt: f.UpdatedAt,
	}
}

func fileRowToDomain(row fileRow) *domain.File {
	return &domain.File{
		ID:        row.ID,
		OrgID:     row.OrgID,
		ProjectID: row.ProjectID,
		UserID:    row.UserID,
		Filename:  row.Filename,
		MimeType:  row.MimeType,
		SizeBytes: row.SizeBytes,
		SHA256:    row.SHA256,
		Purpose:   domain.FilePurpose(row.Purpose),
		Status:    domain.FileStatus(row.Status),
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}
