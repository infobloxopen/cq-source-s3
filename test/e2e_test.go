package test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cloudquery/plugin-sdk/v4/message"
	"github.com/cloudquery/plugin-sdk/v4/plugin"
	"github.com/infobloxopen/cq-source-s3/client"
	"github.com/infobloxopen/cq-source-s3/internal/testutil"
	"github.com/rs/zerolog"
)

func skipIfNoLocalStack(t *testing.T) {
	t.Helper()
	endpoint := testutil.TestEndpoint()
	// Try to create a test client to see if LocalStack is reachable
	ctx := context.Background()
	s3Client, err := testutil.NewTestS3Client(ctx)
	if err != nil {
		t.Skipf("LocalStack not available at %s: %v", endpoint, err)
	}
	// Quick health check - try to list buckets
	_, err = s3Client.ListBuckets(ctx, nil)
	if err != nil {
		t.Skipf("LocalStack not responding at %s: %v", endpoint, err)
	}
}

func TestE2E_FullSync(t *testing.T) {
	skipIfNoLocalStack(t)

	ctx := context.Background()
	s3Client, err := testutil.NewTestS3Client(ctx)
	if err != nil {
		t.Fatalf("NewTestS3Client: %v", err)
	}

	bucket := "e2e-test-sync"
	if err := testutil.CreateBucket(ctx, s3Client, bucket); err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}
	defer func() {
		if err := testutil.CleanBucket(ctx, s3Client, bucket); err != nil {
			t.Logf("CleanBucket: %v", err)
		}
	}()

	// Generate test Parquet files
	sc := testutil.SimpleTestSchema()
	rows := 100

	// Seed bucket with files at different prefix depths
	files := map[string]int{
		"data/2024/file_a.parquet":  rows,
		"data/2024/file_b.parquet":  rows,
		"logs/access.parquet":       rows,
		"metrics/cpu/usage.parquet": rows,
		"root_file.parquet":         rows,
	}

	for key, numRows := range files {
		data, err := testutil.GenerateParquet(sc, numRows)
		if err != nil {
			t.Fatalf("GenerateParquet for %s: %v", key, err)
		}
		if err := testutil.UploadObject(ctx, s3Client, bucket, key, data); err != nil {
			t.Fatalf("UploadObject %s: %v", key, err)
		}
	}

	// Configure plugin client
	spec := client.Spec{
		Bucket:        bucket,
		Region:        testutil.DefaultRegion,
		RowsPerRecord: 500,
		Concurrency:   10,
		Endpoint:      testutil.TestEndpoint(),
		PathStyle:     true,
	}
	spec.SetDefaults()

	specBytes, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("json.Marshal spec: %v", err)
	}

	logger := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Logger()

	// Use a custom endpoint for LocalStack
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	t.Setenv("AWS_ENDPOINT_URL", testutil.TestEndpoint())

	pluginClient, err := client.Configure(ctx, logger, specBytes, plugin.NewClientOptions{})
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	defer func() {
		if err := pluginClient.Close(ctx); err != nil {
			t.Logf("Close: %v", err)
		}
	}()

	// Run Tables() to verify discovery
	tables, err := pluginClient.Tables(ctx, plugin.TableOptions{
		Tables: []string{"*"},
	})
	if err != nil {
		t.Fatalf("Tables: %v", err)
	}

	// Expect 4 tables: data_2024, logs, metrics_cpu, root_file
	if len(tables) != 4 {
		tableNames := make([]string, len(tables))
		for i, tbl := range tables {
			tableNames[i] = tbl.Name
		}
		t.Fatalf("expected 4 tables, got %d: %v", len(tables), tableNames)
	}

	// Run Sync()
	res := make(chan message.SyncMessage, 1000)
	syncDone := make(chan error, 1)

	go func() {
		syncDone <- pluginClient.Sync(ctx, plugin.SyncOptions{
			Tables: []string{"*"},
		}, res)
		close(res)
	}()

	var migrateCount int
	var insertCount int
	var totalRows int64

	for msg := range res {
		switch m := msg.(type) {
		case *message.SyncMigrateTable:
			migrateCount++
		case *message.SyncInsert:
			insertCount++
			totalRows += m.Record.NumRows()
			m.Record.Release()
		}
	}

	if err := <-syncDone; err != nil {
		t.Fatalf("Sync: %v", err)
	}

	// Verify results
	if migrateCount != 4 {
		t.Errorf("SyncMigrateTable count = %d, want 4", migrateCount)
	}

	expectedRows := int64(5 * rows) // 5 files * 100 rows
	if totalRows != expectedRows {
		t.Errorf("total rows = %d, want %d", totalRows, expectedRows)
	}

	t.Logf("Sync complete: %d tables, %d inserts, %d total rows", migrateCount, insertCount, totalRows)
}
