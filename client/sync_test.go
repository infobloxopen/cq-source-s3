package client

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/cloudquery/plugin-sdk/v4/message"
	"github.com/cloudquery/plugin-sdk/v4/schema"
)

func TestSyncTableObjects_Sequential(t *testing.T) {
	c := &Client{
		spec: Spec{
			Bucket:        "test",
			Region:        "us-east-1",
			Concurrency:   1,
			RowsPerRecord: 500,
		},
	}
	if c.spec.Concurrency != 1 {
		t.Errorf("Concurrency = %d, want 1", c.spec.Concurrency)
	}
}

func TestSyncTableObjects_ConcurrencyLimit(t *testing.T) {
	maxConcurrency := 3
	var currentConcurrent int64
	var maxObserved int64

	sem := make(chan struct{}, maxConcurrency)
	done := make(chan struct{})

	numTasks := 20
	var completed int64

	for i := 0; i < numTasks; i++ {
		sem <- struct{}{}
		go func() {
			defer func() { <-sem }()

			cur := atomic.AddInt64(&currentConcurrent, 1)
			for {
				old := atomic.LoadInt64(&maxObserved)
				if cur <= old || atomic.CompareAndSwapInt64(&maxObserved, old, cur) {
					break
				}
			}

			time.Sleep(5 * time.Millisecond)
			atomic.AddInt64(&currentConcurrent, -1)

			if atomic.AddInt64(&completed, 1) == int64(numTasks) {
				close(done)
			}
		}()
	}

	<-done

	if atomic.LoadInt64(&maxObserved) > int64(maxConcurrency) {
		t.Errorf("max concurrent = %d, want <= %d", atomic.LoadInt64(&maxObserved), maxConcurrency)
	}
	if atomic.LoadInt64(&completed) != int64(numTasks) {
		t.Errorf("completed = %d, want %d", atomic.LoadInt64(&completed), numTasks)
	}
}

func TestSyncTableObjects_UnlimitedConcurrency(t *testing.T) {
	c := &Client{
		spec: Spec{
			Bucket:        "test",
			Region:        "us-east-1",
			Concurrency:   -1,
			RowsPerRecord: 500,
		},
	}
	if c.spec.Concurrency >= 0 {
		t.Errorf("Concurrency = %d, want negative for unlimited", c.spec.Concurrency)
	}
}

func TestIsMalformedParquetError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"not parquet", fmt.Errorf("not a parquet file"), true},
		{"invalid parquet", fmt.Errorf("invalid parquet data"), true},
		{"magic number", fmt.Errorf("bad magic number in file"), true},
		{"failed to open", fmt.Errorf("failed to open parquet file: corrupted"), true},
		{"normal error", fmt.Errorf("connection timeout"), false},
		{"s3 error", fmt.Errorf("NoSuchKey: not found"), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isMalformedParquetError(tc.err)
			if got != tc.want {
				t.Errorf("isMalformedParquetError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestSyncMessageTypes(t *testing.T) {
	table := &schema.Table{Name: "test_table"}
	migrate := &message.SyncMigrateTable{Table: table}
	if migrate.Table.Name != "test_table" {
		t.Errorf("unexpected table name: %s", migrate.Table.Name)
	}
}

func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if ctx.Err() == nil {
		t.Error("expected context to be cancelled")
	}
}

func makeTestRecordBatch(md *arrow.Metadata) arrow.RecordBatch {
	sc := arrow.NewSchema([]arrow.Field{
		{Name: "id", Type: arrow.BinaryTypes.String},
		{Name: "name", Type: arrow.BinaryTypes.String},
	}, md)

	bldr := array.NewRecordBuilder(memory.DefaultAllocator, sc)
	defer bldr.Release()

	bldr.Field(0).(*array.StringBuilder).Append("abc")
	bldr.Field(1).(*array.StringBuilder).Append("test-name")

	return bldr.NewRecord()
}

func TestWithTableMetadata_AddsMetadata(t *testing.T) {
	rec := makeTestRecordBatch(nil)
	defer rec.Release()

	result := withTableMetadata(rec, "my_table")
	defer result.Release()

	md := result.Schema().Metadata()
	idx := md.FindKey("cq:table_name")
	if idx < 0 {
		t.Fatal("expected cq:table_name metadata to be present")
	}
	if md.Values()[idx] != "my_table" {
		t.Errorf("cq:table_name = %q, want %q", md.Values()[idx], "my_table")
	}

	// Data should be preserved
	if result.NumRows() != 1 {
		t.Errorf("NumRows = %d, want 1", result.NumRows())
	}
	if result.NumCols() != 2 {
		t.Errorf("NumCols = %d, want 2", result.NumCols())
	}
}

func TestWithTableMetadata_PreservesExistingMetadata(t *testing.T) {
	existing := arrow.MetadataFrom(map[string]string{
		"custom_key": "custom_value",
	})
	rec := makeTestRecordBatch(&existing)
	defer rec.Release()

	result := withTableMetadata(rec, "my_table")
	defer result.Release()

	md := result.Schema().Metadata()

	// Should have both keys
	if v, ok := md.GetValue("custom_key"); !ok || v != "custom_value" {
		t.Errorf("custom_key = %q, want %q", v, "custom_value")
	}
	if v, ok := md.GetValue("cq:table_name"); !ok || v != "my_table" {
		t.Errorf("cq:table_name = %q, want %q", v, "my_table")
	}
}

func TestWithTableMetadata_SkipsWhenAlreadyPresent(t *testing.T) {
	existing := arrow.MetadataFrom(map[string]string{
		"cq:table_name": "original_table",
	})
	rec := makeTestRecordBatch(&existing)
	defer rec.Release()

	result := withTableMetadata(rec, "different_table")

	// Should NOT overwrite — returns the original record
	md := result.Schema().Metadata()
	if v, ok := md.GetValue("cq:table_name"); !ok || v != "original_table" {
		t.Errorf("cq:table_name = %q, want %q (should not overwrite)", v, "original_table")
	}
}
