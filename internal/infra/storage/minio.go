package storage

import (
	"context"
	"fmt"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinIOConfig struct {
	Endpoint         string
	AccessKey        string
	SecretKey        string
	Bucket           string
	Region           string
	UseSSL           bool
	AutoCreateBucket bool
}

type MinIOStore struct {
	client *minio.Client
	bucket string
}

func NewMinIOStore(ctx context.Context, cfg MinIOConfig) (*MinIOStore, error) {
	if cfg.Endpoint == "" || cfg.Bucket == "" {
		return nil, fmt.Errorf("storage: endpoint and bucket are required")
	}
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("storage: init minio client: %w", err)
	}
	ok, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("storage: check bucket: %w", err)
	}
	if !ok {
		if !cfg.AutoCreateBucket {
			return nil, fmt.Errorf("storage: bucket %q does not exist", cfg.Bucket)
		}
		if err := client.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{Region: cfg.Region}); err != nil {
			return nil, fmt.Errorf("storage: create bucket: %w", err)
		}
	}
	return &MinIOStore{client: client, bucket: cfg.Bucket}, nil
}

func (s *MinIOStore) DefaultBucket() string { return s.bucket }

func (s *MinIOStore) PutObject(ctx context.Context, in PutObjectInput) (*PutObjectOutput, error) {
	bucket := fallbackBucket(in.Bucket, s.bucket)
	info, err := s.client.PutObject(ctx, bucket, in.Key, in.Reader, in.Size, minio.PutObjectOptions{
		ContentType: in.ContentType,
	})
	if err != nil {
		return nil, err
	}
	return &PutObjectOutput{ETag: info.ETag, VersionID: info.VersionID}, nil
}

func (s *MinIOStore) GetObject(ctx context.Context, in GetObjectInput) (*GetObjectOutput, error) {
	bucket := fallbackBucket(in.Bucket, s.bucket)
	obj, err := s.client.GetObject(ctx, bucket, in.Key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	info, err := obj.Stat()
	if err != nil {
		_ = obj.Close()
		return nil, err
	}
	return &GetObjectOutput{
		Body:         obj,
		Size:         info.Size,
		ContentType:  info.ContentType,
		ETag:         info.ETag,
		LastModified: info.LastModified,
	}, nil
}

func (s *MinIOStore) DeleteObject(ctx context.Context, in DeleteObjectInput) error {
	bucket := fallbackBucket(in.Bucket, s.bucket)
	return s.client.RemoveObject(ctx, bucket, in.Key, minio.RemoveObjectOptions{})
}

func (s *MinIOStore) StatObject(ctx context.Context, in StatObjectInput) (*ObjectInfo, error) {
	bucket := fallbackBucket(in.Bucket, s.bucket)
	info, err := s.client.StatObject(ctx, bucket, in.Key, minio.StatObjectOptions{})
	if err != nil {
		return nil, err
	}
	return &ObjectInfo{
		Size:         info.Size,
		ContentType:  info.ContentType,
		ETag:         info.ETag,
		LastModified: info.LastModified,
	}, nil
}

func fallbackBucket(got, def string) string {
	if got != "" {
		return got
	}
	return def
}
