package storage

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3Uploader handles file uploads to S3-compatible storage
type S3Uploader struct {
	client   *minio.Client
	bucket   string
	region   string
	endpoint string
	useSSL   bool
}

// S3Config contains configuration for S3 uploader
type S3Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	Region    string
	UseSSL    bool
}

// NewS3Uploader creates a new S3 uploader instance
func NewS3Uploader(config S3Config) (*S3Uploader, error) {
	// Initialize MinIO client
	client, err := minio.New(config.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(config.AccessKey, config.SecretKey, ""),
		Secure: config.UseSSL,
		Region: config.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create S3 client: %w", err)
	}

	return &S3Uploader{
		client:   client,
		bucket:   config.Bucket,
		region:   config.Region,
		endpoint: config.Endpoint,
		useSSL:   config.UseSSL,
	}, nil
}

// Upload uploads a file to S3 and returns the HTTPS URL
func (s *S3Uploader) Upload(ctx context.Context, filePath, objectName string) (string, error) {
	// Upload the file
	_, err := s.client.FPutObject(ctx, s.bucket, objectName, filePath, minio.PutObjectOptions{
		ContentType: "video/mp4",
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload file: %w", err)
	}

	// Generate the HTTPS URL
	url := s.generateHTTPSURL(objectName)
	return url, nil
}

// generateHTTPSURL creates the HTTPS URL for an object
func (s *S3Uploader) generateHTTPSURL(objectName string) string {
	protocol := "https"
	if !s.useSSL {
		protocol = "http"
	}

	// Format: https://endpoint/bucket/object
	return fmt.Sprintf("%s://%s/%s/%s", protocol, s.endpoint, s.bucket, objectName)
}

// EnsureBucket ensures the bucket exists, creates it if it doesn't
func (s *S3Uploader) EnsureBucket(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("failed to check bucket existence: %w", err)
	}

	if !exists {
		err = s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{
			Region: s.region,
		})
		if err != nil {
			return fmt.Errorf("failed to create bucket: %w", err)
		}
	}

	return nil
}

// GetObjectName generates a unique object name from a file path
func GetObjectName(jobID, filePath string) string {
	filename := filepath.Base(filePath)
	return fmt.Sprintf("combined/%s/%s", jobID, filename)
}
