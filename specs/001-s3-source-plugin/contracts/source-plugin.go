// Package contracts defines the interface contract for the cq-source-s3
// CloudQuery source plugin.
//
// This file documents the Go interfaces that the plugin must implement to
// satisfy the CloudQuery Plugin SDK v4 source plugin contract.
//
// These interfaces are NOT defined by this project — they come from
// github.com/cloudquery/plugin-sdk/v4. This file serves as a reference
// contract for development and review.

package contracts

import (
	"context"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/cloudquery/plugin-sdk/v4/message"
	"github.com/cloudquery/plugin-sdk/v4/plugin"
	"github.com/cloudquery/plugin-sdk/v4/schema"
	"github.com/rs/zerolog"
)

// ---------------------------------------------------------------------------
// SDK Interfaces (reference — defined by plugin-sdk/v4)
// ---------------------------------------------------------------------------

// SourceClient is the interface a source plugin must implement.
// Our Client struct in client/client.go implements this.
type SourceClient interface {
	// Close releases resources held by the client (S3 connections, state client).
	Close(ctx context.Context) error

	// Tables returns the list of tables the plugin can sync.
	// For cq-source-s3, tables are discovered dynamically from S3 key prefixes.
	Tables(ctx context.Context, options plugin.TableOptions) (schema.Tables, error)

	// Sync streams data from S3 to the destination via the res channel.
	// Emits SyncMigrateTable per table, then SyncInsert per record batch.
	Sync(ctx context.Context, options plugin.SyncOptions, res chan<- message.SyncMessage) error
}

// NewClientFunc is the factory function signature expected by plugin.NewPlugin.
// Our Configure function in client/client.go matches this signature.
type NewClientFunc func(
	ctx context.Context,
	logger zerolog.Logger,
	spec []byte,
	opts plugin.NewClientOptions,
) (plugin.Client, error)

// ---------------------------------------------------------------------------
// Plugin-Specific Interfaces (defined by this project)
// ---------------------------------------------------------------------------

// TableDiscoverer lists S3 objects and groups them into logical tables.
type TableDiscoverer interface {
	// Discover lists all matching objects in the bucket and returns
	// discovered tables grouped by key prefix.
	Discover(ctx context.Context) ([]DiscoveredTable, error)
}

// DiscoveredTable represents a logical table derived from S3 key prefixes.
type DiscoveredTable struct {
	// Name is the normalized table name (e.g., "data_2024").
	Name string

	// Prefix is the original S3 key prefix (e.g., "data/2024/").
	Prefix string

	// Objects is the list of S3 objects belonging to this table.
	Objects []S3Object

	// ArrowSchema is the Arrow schema derived from the first Parquet file.
	ArrowSchema *arrow.Schema

	// Table is the CQ SDK table built from ArrowSchema with CQ internal columns.
	Table *schema.Table
}

// S3Object holds metadata about a single S3 object.
type S3Object struct {
	Key          string
	Size         int64
	LastModified string // RFC3339Nano
}

// ParquetReader reads Parquet data from an S3 object and streams Arrow records.
type ParquetReader interface {
	// ReadSchema returns the Arrow schema of the Parquet file without reading data.
	ReadSchema(ctx context.Context, bucket, key string) (*arrow.Schema, error)

	// StreamRecords downloads the Parquet file and streams Arrow record batches
	// to the provided channel. Each record contains up to batchSize rows.
	StreamRecords(ctx context.Context, bucket, key string, batchSize int, records chan<- arrow.RecordBatch) error
}

// CursorStore reads and writes incremental sync cursors via the state backend.
type CursorStore interface {
	// GetCursor retrieves the stored cursor timestamp for a table.
	// Returns zero-time if no cursor exists.
	GetCursor(ctx context.Context, tableName string) (string, error)

	// SetCursor stores the cursor timestamp for a table.
	SetCursor(ctx context.Context, tableName, timestamp string) error

	// Flush persists all pending cursor writes.
	Flush(ctx context.Context) error
}

// TableNameNormalizer converts S3 key prefixes into valid table names.
type TableNameNormalizer interface {
	// Normalize converts an S3 key into a table name.
	// For keys with directory prefixes: strip filename, join segments with "_".
	// For root-level keys: use filename without extension.
	// Invalid characters are replaced with "_".
	Normalize(key string) string
}
