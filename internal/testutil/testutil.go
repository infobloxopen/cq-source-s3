package testutil

import (
	"bytes"
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const (
	DefaultEndpoint = "http://localhost:4566"
	DefaultRegion   = "us-east-1"
	DefaultBucket   = "test-bucket"
)

// TestEndpoint returns the S3 endpoint for testing. Uses CQ_S3_TEST_ENDPOINT
// environment variable if set, otherwise defaults to LocalStack.
func TestEndpoint() string {
	if ep := os.Getenv("CQ_S3_TEST_ENDPOINT"); ep != "" {
		return ep
	}
	return DefaultEndpoint
}

// NewTestS3Client creates an S3 client pointing at the test endpoint (LocalStack).
func NewTestS3Client(ctx context.Context) (*s3.Client, error) {
	endpoint := TestEndpoint()

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(DefaultRegion),
		config.WithCredentialsProvider(aws.CredentialsProviderFunc(
			func(ctx context.Context) (aws.Credentials, error) {
				return aws.Credentials{
					AccessKeyID:     "test",
					SecretAccessKey: "test",
				}, nil
			},
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load test AWS config: %w", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})

	return client, nil
}

// CreateBucket creates an S3 bucket in the test endpoint. Ignores errors if
// the bucket already exists.
func CreateBucket(ctx context.Context, client *s3.Client, bucket string) error {
	_, err := client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		// Ignore BucketAlreadyOwnedByYou or BucketAlreadyExists
		return nil
	}
	return nil
}

// UploadObject uploads data to an S3 bucket at the given key.
func UploadObject(ctx context.Context, client *s3.Client, bucket, key string, data []byte) error {
	_, err := client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(data),
	})
	if err != nil {
		return fmt.Errorf("failed to upload %s/%s: %w", bucket, key, err)
	}
	return nil
}

// CleanBucket deletes all objects in a bucket. Does not delete the bucket itself.
func CleanBucket(ctx context.Context, client *s3.Client, bucket string) error {
	paginator := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to list objects in %s: %w", bucket, err)
		}
		for _, obj := range page.Contents {
			_, err := client.DeleteObject(ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(bucket),
				Key:    obj.Key,
			})
			if err != nil {
				return fmt.Errorf("failed to delete %s/%s: %w", bucket, *obj.Key, err)
			}
		}
	}
	return nil
}
