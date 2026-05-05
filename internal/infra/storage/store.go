package storage

import (
	"context"
	"io"
	"time"
)

type PutObjectInput struct {
	Bucket      string
	Key         string
	Reader      io.Reader
	Size        int64
	ContentType string
}

type PutObjectOutput struct {
	ETag      string
	VersionID string
}

type GetObjectInput struct {
	Bucket string
	Key    string
}

type GetObjectOutput struct {
	Body         io.ReadCloser
	Size         int64
	ContentType  string
	ETag         string
	LastModified time.Time
}

type DeleteObjectInput struct {
	Bucket string
	Key    string
}

type StatObjectInput struct {
	Bucket string
	Key    string
}

type ObjectInfo struct {
	Size         int64
	ContentType  string
	ETag         string
	LastModified time.Time
}

type Store interface {
	PutObject(ctx context.Context, in PutObjectInput) (*PutObjectOutput, error)
	GetObject(ctx context.Context, in GetObjectInput) (*GetObjectOutput, error)
	DeleteObject(ctx context.Context, in DeleteObjectInput) error
	StatObject(ctx context.Context, in StatObjectInput) (*ObjectInfo, error)
}
