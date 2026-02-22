package client

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/parquet/file"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// readParquetSchema downloads an S3 object to a temp file and reads its Arrow schema.
func (c *Client) readParquetSchema(ctx context.Context, key string) (*arrow.Schema, error) {
	tmpFile, cleanup, err := c.downloadToTemp(ctx, key)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	pf, err := file.OpenParquetFile(tmpFile.Name(), false)
	if err != nil {
		return nil, fmt.Errorf("failed to open parquet file %s: %w", key, err)
	}
	defer func() { _ = pf.Close() }()

	reader, err := pqarrow.NewFileReader(pf, pqarrow.ArrowReadProperties{
		Parallel: true,
	}, memory.DefaultAllocator)
	if err != nil {
		return nil, fmt.Errorf("failed to create arrow reader for %s: %w", key, err)
	}

	sc, err := reader.Schema()
	if err != nil {
		return nil, fmt.Errorf("failed to read schema from %s: %w", key, err)
	}

	return sc, nil
}

// streamRecords downloads an S3 object and streams Arrow record batches to the channel.
func (c *Client) streamRecords(ctx context.Context, key string, batchSize int, records chan<- arrow.RecordBatch) error {
	tmpFile, cleanup, err := c.downloadToTemp(ctx, key)
	if err != nil {
		return err
	}
	defer cleanup()

	pf, err := file.OpenParquetFile(tmpFile.Name(), false)
	if err != nil {
		return fmt.Errorf("failed to open parquet file %s: %w", key, err)
	}
	defer func() { _ = pf.Close() }()

	reader, err := pqarrow.NewFileReader(pf, pqarrow.ArrowReadProperties{
		Parallel:  true,
		BatchSize: int64(batchSize),
	}, memory.DefaultAllocator)
	if err != nil {
		return fmt.Errorf("failed to create arrow reader for %s: %w", key, err)
	}

	rr, err := reader.GetRecordReader(ctx, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to get record reader for %s: %w", key, err)
	}
	defer rr.Release()

	for rr.Next() {
		rec := rr.RecordBatch()
		rec.Retain()
		select {
		case records <- rec:
		case <-ctx.Done():
			rec.Release()
			return ctx.Err()
		}
	}
	if err := rr.Err(); err != nil {
		return fmt.Errorf("error reading records from %s: %w", key, err)
	}

	return nil
}

// downloadToTemp downloads an S3 object to a temporary file and returns the file
// and a cleanup function.
func (c *Client) downloadToTemp(ctx context.Context, key string) (*os.File, func(), error) {
	resp, err := c.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.spec.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to download %s: %w", key, err)
	}
	defer func() { _ = resp.Body.Close() }()

	tmpFile, err := os.CreateTemp("", "cq-s3-*.parquet")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create temp file: %w", err)
	}

	cleanup := func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
	}

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("failed to write temp file for %s: %w", key, err)
	}

	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("failed to seek temp file for %s: %w", key, err)
	}

	return tmpFile, cleanup, nil
}
