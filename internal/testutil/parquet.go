package testutil

import (
	"bytes"
	"fmt"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"
)

// GenerateParquet creates an in-memory Parquet file with the given schema and
// row count. Each column is filled with deterministic test data.
func GenerateParquet(sc *arrow.Schema, numRows int) ([]byte, error) {
	alloc := memory.DefaultAllocator
	builder := array.NewRecordBuilder(alloc, sc)
	defer builder.Release()

	for i := 0; i < numRows; i++ {
		for j, field := range sc.Fields() {
			switch field.Type.ID() {
			case arrow.INT64:
				builder.Field(j).(*array.Int64Builder).Append(int64(i))
			case arrow.FLOAT64:
				builder.Field(j).(*array.Float64Builder).Append(float64(i) * 1.1)
			case arrow.STRING, arrow.LARGE_STRING:
				builder.Field(j).(*array.StringBuilder).Append(fmt.Sprintf("val_%d", i))
			case arrow.BOOL:
				builder.Field(j).(*array.BooleanBuilder).Append(i%2 == 0)
			default:
				builder.Field(j).AppendNull()
			}
		}
	}

	rec := builder.NewRecordBatch()
	defer rec.Release()

	var buf bytes.Buffer
	writer, err := pqarrow.NewFileWriter(sc, &buf, nil, pqarrow.DefaultWriterProps())
	if err != nil {
		return nil, fmt.Errorf("failed to create parquet writer: %w", err)
	}

	if err := writer.Write(rec); err != nil {
		return nil, fmt.Errorf("failed to write record to parquet: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close parquet writer: %w", err)
	}

	return buf.Bytes(), nil
}

// SimpleTestSchema returns a simple Arrow schema for testing: (id: int64, name: string).
func SimpleTestSchema() *arrow.Schema {
	return arrow.NewSchema([]arrow.Field{
		{Name: "id", Type: arrow.PrimitiveTypes.Int64},
		{Name: "name", Type: arrow.BinaryTypes.String},
	}, nil)
}

// DifferentTestSchema returns a schema that differs from SimpleTestSchema for
// testing schema mismatch: (id: int64, value: float64).
func DifferentTestSchema() *arrow.Schema {
	return arrow.NewSchema([]arrow.Field{
		{Name: "id", Type: arrow.PrimitiveTypes.Int64},
		{Name: "value", Type: arrow.PrimitiveTypes.Float64},
	}, nil)
}
