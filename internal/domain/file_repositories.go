package domain

import "context"

type FileRepository interface {
	Create(ctx context.Context, f *File) error
	Update(ctx context.Context, f *File) error
	GetByID(ctx context.Context, id string) (*File, error)
	ListByProject(ctx context.Context, orgID, projectID string, limit int) ([]File, error)
}

type FileObjectRepository interface {
	Create(ctx context.Context, obj *FileObject) error
	GetByFileID(ctx context.Context, fileID string) (*FileObject, error)
}
