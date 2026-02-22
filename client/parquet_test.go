package client

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/parquet/file"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"
	"github.com/infobloxopen/cq-source-s3/internal/testutil"
)

func TestReadParquetSchema(t *testing.T) {
	sc := testutil.SimpleTestSchema()
	data, err := testutil.GenerateParquet(sc, 10)
	if err != nil {
		t.Fatalf("GenerateParquet: %v", err)
	}

	// Write to temp file
	tmpFile, err := os.CreateTemp("", "test-*.parquet")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	if _, err := tmpFile.Write(data); err != nil {
		t.Fatalf("Write: %v", err)
	}
	_ = tmpFile.Close()

	// Read schema using low-level parquet API (same as our readParquetSchema)
	pf, err := file.OpenParquetFile(tmpFile.Name(), false)
	if err != nil {
		t.Fatalf("OpenParquetFile: %v", err)
	}
	defer func() { _ = pf.Close() }()

	reader, err := pqarrow.NewFileReader(pf, pqarrow.ArrowReadProperties{
		Parallel: true,
	}, memory.DefaultAllocator)
	if err != nil {
		t.Fatalf("NewFileReader: %v", err)
	}

	readSchema, err := reader.Schema()
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}

	// Compare field count, names, and types (Parquet roundtrip adds field metadata)
	if readSchema.NumFields() != sc.NumFields() {
		t.Fatalf("field count = %d, want %d", readSchema.NumFields(), sc.NumFields())
	}
	for i := 0; i < sc.NumFields(); i++ {
		got := readSchema.Field(i)
		want := sc.Field(i)
		if got.Name != want.Name {
			t.Errorf("field %d name = %q, want %q", i, got.Name, want.Name)
		}
		if got.Type.ID() != want.Type.ID() {
			t.Errorf("field %d type = %v, want %v", i, got.Type, want.Type)
		}
	}

	// Verify field names
	if readSchema.Field(0).Name != "id" {
		t.Errorf("field 0 name = %q, want %q", readSchema.Field(0).Name, "id")
	}
	if readSchema.Field(1).Name != "name" {
		t.Errorf("field 1 name = %q, want %q", readSchema.Field(1).Name, "name")
	}
}

func TestStreamRecords_BatchSize(t *testing.T) {
	sc := testutil.SimpleTestSchema()
	numRows := 1000
	data, err := testutil.GenerateParquet(sc, numRows)
	if err != nil {
		t.Fatalf("GenerateParquet: %v", err)
	}

	// Write to temp file and read with batch size
	tmpFile, err := os.CreateTemp("", "test-batch-*.parquet")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	if _, err := tmpFile.Write(data); err != nil {
		t.Fatalf("Write: %v", err)
	}
	_ = tmpFile.Close()

	pf, err := file.OpenParquetFile(tmpFile.Name(), false)
	if err != nil {
		t.Fatalf("OpenParquetFile: %v", err)
	}
	defer func() { _ = pf.Close() }()

	batchSize := 100
	reader, err := pqarrow.NewFileReader(pf, pqarrow.ArrowReadProperties{
		Parallel:  true,
		BatchSize: int64(batchSize),
	}, memory.DefaultAllocator)
	if err != nil {
		t.Fatalf("NewFileReader: %v", err)
	}

	rr, err := reader.GetRecordReader(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("GetRecordReader: %v", err)
	}
	defer rr.Release()

	var totalRows int64
	var batchCount int
	for rr.Next() {
		rec := rr.RecordBatch()
		totalRows += rec.NumRows()
		batchCount++
	}
	if err := rr.Err(); err != nil {
		t.Fatalf("RecordReader error: %v", err)
	}

	if totalRows != int64(numRows) {
		t.Errorf("total rows = %d, want %d", totalRows, numRows)
	}

	// With 1000 rows and batch size 100, expect ~10 batches
	if batchCount < 2 {
		t.Errorf("expected multiple batches, got %d", batchCount)
	}
}

func TestGenerateParquet_Roundtrip(t *testing.T) {
	sc := testutil.SimpleTestSchema()
	numRows := 50
	data, err := testutil.GenerateParquet(sc, numRows)
	if err != nil {
		t.Fatalf("GenerateParquet: %v", err)
	}

	// Verify we can read the data back
	buf := bytes.NewReader(data)
	pf, err := file.NewParquetReader(buf)
	if err != nil {
		t.Fatalf("NewParquetReader: %v", err)
	}
	defer func() { _ = pf.Close() }()

	if pf.NumRows() != int64(numRows) {
		t.Errorf("NumRows = %d, want %d", pf.NumRows(), numRows)
	}
}

func TestDifferentSchema(t *testing.T) {
	sc1 := testutil.SimpleTestSchema()
	sc2 := testutil.DifferentTestSchema()

	if sc1.Equal(sc2) {
		t.Error("SimpleTestSchema and DifferentTestSchema should not be equal")
	}

	// Verify field names differ
	if sc1.Field(1).Name == sc2.Field(1).Name {
		t.Error("second field should differ between schemas")
	}
}

func TestDifferentSchema_Values(t *testing.T) {
	// Ensure both schemas produce valid Parquet files
	for _, tc := range []struct {
		name string
		sc   *arrow.Schema
	}{
		{"simple", testutil.SimpleTestSchema()},
		{"different", testutil.DifferentTestSchema()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := testutil.GenerateParquet(tc.sc, 10)
			if err != nil {
				t.Fatalf("GenerateParquet: %v", err)
			}
			if len(data) == 0 {
				t.Fatal("empty parquet data")
			}
		})
	}
}
