package services

import (
	"context"
	"io"
	"time"
)

// FileStorage defines the abstract storage interface consumed by application services.
// Any storage provider (AWS S3, MinIO, Google Cloud Storage, Local Filesystem)
// that implements these methods can be seamlessly injected.
type FileStorage interface {
	// Ping checks if the storage backend is reachable.
	Ping(ctx context.Context) error

	// Upload streams an object payload to the designated storage bucket/directory.
	Upload(ctx context.Context, bucketName, objectName string, reader io.Reader, size int64, contentType string) error

	// Download retrieves an object stream from storage.
	Download(ctx context.Context, bucketName, objectName string) (io.ReadCloser, error)

	// Delete removes an object from storage.
	Delete(ctx context.Context, bucketName, objectName string) error

	// PresignGetObject generates a temporary pre-signed GET URL for downloading.
	PresignGetObject(ctx context.Context, bucketName, objectName string, expiry time.Duration) (string, error)

	// PresignPutObject generates a temporary pre-signed PUT URL for direct uploading.
	PresignPutObject(ctx context.Context, bucketName, objectName string, expiry time.Duration) (string, error)

	// StatObject retrieves the size of an object in bytes from storage.
	StatObject(ctx context.Context, bucketName, objectName string) (int64, error)
}
