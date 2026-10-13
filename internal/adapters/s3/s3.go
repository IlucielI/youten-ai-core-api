package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"code-base-golang/internal/config"
)

// S3 wraps the MinIO client for AWS S3 and MinIO compatible object storage.
type S3 struct {
	client     *minio.Client
	bucketName string
}

// New initializes an S3 client and verifies connection reachability.
func New(cfg config.Config) (*S3, error) {
	endpoint := strings.TrimSpace(cfg.S3Endpoint)
	endpoint = strings.TrimPrefix(endpoint, "http://")
	endpoint = strings.TrimPrefix(endpoint, "https://")
	if endpoint == "" {
		return nil, errors.New("s3 endpoint is required")
	}

	accessKey := strings.TrimSpace(cfg.S3AccessKey)
	if accessKey == "" {
		return nil, errors.New("s3 access key is required")
	}

	secretKey := strings.TrimSpace(cfg.S3SecretKey)
	if secretKey == "" {
		return nil, errors.New("s3 secret key is required")
	}

	bucketLookup := minio.BucketLookupAuto
	if cfg.S3ForcePathStyle {
		bucketLookup = minio.BucketLookupPath
	}

	client, err := minio.New(endpoint, &minio.Options{
		Creds:        credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure:       cfg.S3UseSSL,
		Region:       strings.TrimSpace(cfg.S3Region),
		BucketLookup: bucketLookup,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create s3 client: %w", err)
	}

	s3Adapter := &S3{
		client:     client,
		bucketName: strings.TrimSpace(cfg.S3BucketName),
	}

	// Verify connectivity with a timeout probe
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := s3Adapter.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping s3 endpoint %s: %w", endpoint, err)
	}

	return s3Adapter, nil
}

// Client returns the underlying *minio.Client instance.
func (s *S3) Client() *minio.Client {
	if s == nil {
		return nil
	}
	return s.client
}

// BucketName returns the default configured bucket name.
func (s *S3) BucketName() string {
	if s == nil {
		return ""
	}
	return s.bucketName
}

// Ping checks if the S3 / MinIO storage service is reachable.
func (s *S3) Ping(ctx context.Context) error {
	if s == nil || s.client == nil {
		return errors.New("s3 client is nil")
	}

	// Probe storage liveness by checking bucket existence or listing
	bucket := s.bucketName
	if bucket == "" {
		bucket = "probe"
	}
	_, err := s.client.BucketExists(ctx, bucket)
	return err
}

// EnsureBucketExists creates the specified bucket if it does not already exist.
func (s *S3) EnsureBucketExists(ctx context.Context, bucketName string) error {
	if s == nil || s.client == nil {
		return errors.New("s3 client is nil")
	}

	targetBucket := s.resolveBucket(bucketName)
	if targetBucket == "" {
		return errors.New("bucket name is required")
	}

	exists, err := s.client.BucketExists(ctx, targetBucket)
	if err != nil {
		return fmt.Errorf("check bucket exists: %w", err)
	}
	if exists {
		return nil
	}

	if err := s.client.MakeBucket(ctx, targetBucket, minio.MakeBucketOptions{}); err != nil {
		return fmt.Errorf("make bucket: %w", err)
	}

	return nil
}

// Upload streams an object to the specified bucket.
func (s *S3) Upload(ctx context.Context, bucketName, objectName string, reader io.Reader, size int64, contentType string) error {
	if s == nil || s.client == nil {
		return errors.New("s3 client is nil")
	}

	targetBucket := s.resolveBucket(bucketName)
	if targetBucket == "" {
		return errors.New("bucket name is required")
	}
	if strings.TrimSpace(objectName) == "" {
		return errors.New("object name is required")
	}

	_, err := s.client.PutObject(ctx, targetBucket, objectName, reader, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	return err
}

// Download retrieves an object stream from the bucket.
func (s *S3) Download(ctx context.Context, bucketName, objectName string) (io.ReadCloser, error) {
	if s == nil || s.client == nil {
		return nil, errors.New("s3 client is nil")
	}

	targetBucket := s.resolveBucket(bucketName)
	if targetBucket == "" {
		return nil, errors.New("bucket name is required")
	}
	if strings.TrimSpace(objectName) == "" {
		return nil, errors.New("object name is required")
	}

	return s.client.GetObject(ctx, targetBucket, objectName, minio.GetObjectOptions{})
}

// Delete removes an object from the bucket.
func (s *S3) Delete(ctx context.Context, bucketName, objectName string) error {
	if s == nil || s.client == nil {
		return errors.New("s3 client is nil")
	}

	targetBucket := s.resolveBucket(bucketName)
	if targetBucket == "" {
		return errors.New("bucket name is required")
	}
	if strings.TrimSpace(objectName) == "" {
		return errors.New("object name is required")
	}

	return s.client.RemoveObject(ctx, targetBucket, objectName, minio.RemoveObjectOptions{})
}

// PresignGetObject generates a presigned GET URL for downloading an object.
func (s *S3) PresignGetObject(ctx context.Context, bucketName, objectName string, expiry time.Duration) (string, error) {
	if s == nil || s.client == nil {
		return "", errors.New("s3 client is nil")
	}

	targetBucket := s.resolveBucket(bucketName)
	if targetBucket == "" {
		return "", errors.New("bucket name is required")
	}
	if strings.TrimSpace(objectName) == "" {
		return "", errors.New("object name is required")
	}

	if expiry <= 0 {
		expiry = time.Hour
	}

	presignedURL, err := s.client.PresignedGetObject(ctx, targetBucket, objectName, expiry, url.Values{})
	if err != nil {
		return "", err
	}

	return presignedURL.String(), nil
}

// PresignPutObject generates a presigned PUT URL for uploading an object.
func (s *S3) PresignPutObject(ctx context.Context, bucketName, objectName string, expiry time.Duration) (string, error) {
	if s == nil || s.client == nil {
		return "", errors.New("s3 client is nil")
	}

	targetBucket := s.resolveBucket(bucketName)
	if targetBucket == "" {
		return "", errors.New("bucket name is required")
	}
	if strings.TrimSpace(objectName) == "" {
		return "", errors.New("object name is required")
	}

	if expiry <= 0 {
		expiry = time.Hour
	}

	presignedURL, err := s.client.PresignedPutObject(ctx, targetBucket, objectName, expiry)
	if err != nil {
		return "", err
	}

	return presignedURL.String(), nil
}

// StatObject retrieves the size of an object in bytes from the bucket.
func (s *S3) StatObject(ctx context.Context, bucketName, objectName string) (int64, error) {
	if s == nil || s.client == nil {
		return 0, errors.New("s3 client is nil")
	}

	targetBucket := s.resolveBucket(bucketName)
	if targetBucket == "" {
		return 0, errors.New("bucket name is required")
	}
	if strings.TrimSpace(objectName) == "" {
		return 0, errors.New("object name is required")
	}

	info, err := s.client.StatObject(ctx, targetBucket, objectName, minio.StatObjectOptions{})
	if err != nil {
		return 0, err
	}
	return info.Size, nil
}

func (s *S3) resolveBucket(bucketName string) string {
	trimmed := strings.TrimSpace(bucketName)
	if trimmed != "" {
		return trimmed
	}
	return s.bucketName
}
