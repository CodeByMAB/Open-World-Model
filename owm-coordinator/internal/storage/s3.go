// Package storage provides S3-compatible object storage for gradient blobs and model checkpoints.
package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/owmnetwork/owm-coordinator/internal/config"
)

// S3Client wraps aws-sdk-go-v2 S3 for OWM gradient and model storage.
type S3Client struct {
	client *s3.Client
	bucket string
}

// NewS3Client constructs an S3 client with optional custom endpoint (e.g. MinIO) and static credentials.
func NewS3Client(cfg config.S3Config) (*S3Client, error) {
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("storage: bucket is required")
	}
	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}
	awsCfg := aws.Config{
		Region: region,
	}
	if cfg.AccessKey != "" && cfg.SecretKey != "" {
		awsCfg.Credentials = credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")
	}
	s3Opts := []func(*s3.Options){
		func(o *s3.Options) { o.Region = region },
	}
	if cfg.Endpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			o.UsePathStyle = true
		})
	}
	client := s3.NewFromConfig(awsCfg, s3Opts...)
	return &S3Client{client: client, bucket: cfg.Bucket}, nil
}

// GetObject downloads the object at key and returns its body.
func (c *S3Client) GetObject(ctx context.Context, key string) ([]byte, error) {
	out, err := c.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	defer out.Body.Close()
	return io.ReadAll(out.Body)
}

// PutObject uploads data to key and returns the ETag.
func (c *S3Client) PutObject(ctx context.Context, key string, data []byte) (etag string, err error) {
	out, err := c.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(data),
	})
	if err != nil {
		return "", err
	}
	if out.ETag != nil {
		return *out.ETag, nil
	}
	return "", nil
}
