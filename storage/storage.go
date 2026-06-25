// Package storage is a thin wrapper over an S3-compatible object store
// (Cloudflare R2, AWS S3, MinIO, …) for the handful of operations an app
// actually needs: put, get, delete, and presigned URLs.
//
// It is built on aws-sdk-go-v2 and sets the checksum knobs R2 requires
// (aws-sdk-go-v2 >= ~1.30 sends flexible checksums by default that R2 rejects).
package storage

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Config configures a Bucket against any S3-compatible endpoint.
type Config struct {
	Endpoint        string // full https endpoint, e.g. https://<acct>.r2.cloudflarestorage.com
	Region          string // "auto" for R2; the real region for AWS S3
	AccessKeyID     string
	SecretAccessKey string
	Bucket          string
}

// Bucket is a handle to one bucket on an S3-compatible store.
type Bucket struct {
	client  *s3.Client
	presign *s3.PresignClient
	name    string
}

// New builds a Bucket for any S3-compatible endpoint.
func New(cfg Config) *Bucket {
	region := cfg.Region
	if region == "" {
		region = "auto"
	}
	awscfg := aws.Config{
		Region:      region,
		Credentials: credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		// R2 rejects the flexible checksums aws-sdk-go-v2 sends by default; only
		// send/validate them when explicitly requested.
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
	}
	client := s3.NewFromConfig(awscfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.Endpoint)
	})
	return &Bucket{client: client, presign: s3.NewPresignClient(client), name: cfg.Bucket}
}

// NewR2 is a convenience constructor for Cloudflare R2: it builds the endpoint
// from your account id and uses region "auto".
func NewR2(accountID, accessKeyID, secretAccessKey, bucket string) *Bucket {
	return New(Config{
		Endpoint:        fmt.Sprintf("https://%s.r2.cloudflarestorage.com", accountID),
		Region:          "auto",
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
		Bucket:          bucket,
	})
}

// Put uploads body under key. contentType may be empty.
func (b *Bucket) Put(ctx context.Context, key string, body io.Reader, contentType string) error {
	in := &s3.PutObjectInput{Bucket: &b.name, Key: &key, Body: body}
	if contentType != "" {
		in.ContentType = &contentType
	}
	if _, err := b.client.PutObject(ctx, in); err != nil {
		return fmt.Errorf("storage: put %q: %w", key, err)
	}
	return nil
}

// Get returns a reader for the object at key. The caller must close it.
func (b *Bucket) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := b.client.GetObject(ctx, &s3.GetObjectInput{Bucket: &b.name, Key: &key})
	if err != nil {
		return nil, fmt.Errorf("storage: get %q: %w", key, err)
	}
	return out.Body, nil
}

// Delete removes the object at key. Deleting a missing key is not an error.
func (b *Bucket) Delete(ctx context.Context, key string) error {
	if _, err := b.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: &b.name, Key: &key}); err != nil {
		return fmt.Errorf("storage: delete %q: %w", key, err)
	}
	return nil
}

// PresignGet returns a short-lived URL that downloads the object at key —
// useful for serving private files without proxying bytes through your app.
func (b *Bucket) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	req, err := b.presign.PresignGetObject(ctx, &s3.GetObjectInput{Bucket: &b.name, Key: &key},
		func(o *s3.PresignOptions) { o.Expires = ttl })
	if err != nil {
		return "", fmt.Errorf("storage: presign get %q: %w", key, err)
	}
	return req.URL, nil
}

// PresignPut returns a short-lived URL that uploads to key — useful for direct
// browser-to-bucket uploads (the bytes never touch your server). contentType
// may be empty.
func (b *Bucket) PresignPut(ctx context.Context, key, contentType string, ttl time.Duration) (string, error) {
	in := &s3.PutObjectInput{Bucket: &b.name, Key: &key}
	if contentType != "" {
		in.ContentType = &contentType
	}
	req, err := b.presign.PresignPutObject(ctx, in, func(o *s3.PresignOptions) { o.Expires = ttl })
	if err != nil {
		return "", fmt.Errorf("storage: presign put %q: %w", key, err)
	}
	return req.URL, nil
}
