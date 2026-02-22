# Research: CloudQuery S3 Source Plugin

**Phase**: 0 — Outline & Research
**Feature**: 001-s3-source-plugin
**Date**: 2026-02-21

---

## 1. CloudQuery Plugin SDK v4 — Source Plugin Contract

### Decision
Use `github.com/cloudquery/plugin-sdk/v4` (latest v4.94.x) as the plugin framework.

### Rationale
This is the only supported SDK for building CloudQuery source plugins in Go. It provides the gRPC server, schema utilities, message types, and state backend needed for our plugin.

### Key Findings

- **Module**: `github.com/cloudquery/plugin-sdk/v4`
- **Plugin Constructor**: `plugin.NewPlugin(name, version, newClientFunc, options...)`
- **Client Interface**: Implement `plugin.SourceClient` (Sync, Tables, Close); embed `plugin.UnimplementedDestination` for source-only plugins
- **NewClientFunc signature**: `func(ctx context.Context, logger zerolog.Logger, spec []byte, opts plugin.NewClientOptions) (plugin.Client, error)`
- **Schema**: `schema.NewTableFromArrowSchema(arrowSchema)` converts Arrow schemas to CQ tables; `schema.AddCqIDs(table)` adds internal CQ columns
- **Messages**: `message.SyncMigrateTable{Table}` emitted first per table, then `message.SyncInsert{Record}` for each Arrow record batch
- **State Backend**: `state.NewConnectedClient(ctx, backendOptions)` — returns `NoOpClient` if `backendOptions` is nil (graceful no-backend fallback)
- **Serve**: `serve.Plugin(p).Serve(ctx)` starts the gRPC server

### Alternatives Considered
- **Plugin SDK v3**: Deprecated; v4 is current. No reason to use v3.
- **Custom gRPC server**: Unnecessary; SDK handles all transport concerns.

---

## 2. Apache Arrow Go v18 — Parquet Reading

### Decision
Use `github.com/apache/arrow-go/v18` with the `pqarrow` high-level API for streaming Parquet data as Arrow record batches.

### Rationale
The `pqarrow` package provides a streaming `RecordReader` that yields `arrow.Record` batches with configurable `BatchSize`, which maps directly to our `rows_per_record` spec field. This avoids loading entire files into memory.

### Key Findings

- **Module**: `github.com/apache/arrow-go/v18` (latest v18.5.x)
- **Two-layer API**:
  - Low-level: `parquet/file.NewParquetReader(readerAtSeeker)` for file metadata access
  - High-level: `pqarrow.NewFileReader(fileReader, ArrowReadProperties{BatchSize, Parallel}, allocator)` for Arrow-native streaming
- **Streaming Pattern**:
  1. `file.NewParquetReader(readerAtSeeker)` → `*file.Reader`
  2. `pqarrow.NewFileReader(fileReader, ArrowReadProperties{BatchSize: rowsPerRecord, Parallel: true}, memory.DefaultAllocator)` → `*pqarrow.FileReader`
  3. `fileReader.Schema()` → `*arrow.Schema` (for table schema derivation)
  4. `fileReader.GetRecordReader(ctx, nil, nil)` → `RecordReader`
  5. Loop `recordReader.Read()` → `arrow.Record` batches
- **Temp file requirement**: `file.NewParquetReader` needs `io.ReaderAt + io.Seeker` — S3 `GetObject` returns `io.ReadCloser`, so we must download to a temp file first for random-access Parquet footer reading
- **Schema conversion**: `pqarrow.FromParquet(parquetSchema, props, kv)` → `*arrow.Schema`, or use `pqarrow.FileReader.Schema()`

### Alternatives Considered
- **xitongsys/parquet-go**: Less maintained, no native Arrow integration, would require manual conversion.
- **segmentio/parquet-go**: Good library but lacks the direct Arrow record batch streaming that `apache/arrow-go` provides natively.
- **Read entire file into memory**: Not viable for multi-GB files per FR-022.

---

## 3. AWS SDK Go v2 — S3 Operations

### Decision
Use `github.com/aws/aws-sdk-go-v2` with the `service/s3` package and `config` package for credential loading.

### Rationale
This is the current, maintained AWS SDK for Go. It supports the full credential chain, S3 pagination, and is the standard choice for Go AWS integrations.

### Key Findings

- **Modules**:
  - `github.com/aws/aws-sdk-go-v2/config` — `LoadDefaultConfig(ctx, optFns...)`
  - `github.com/aws/aws-sdk-go-v2/service/s3` — S3 client, paginator
  - `github.com/aws/aws-sdk-go-v2/service/s3/types` — `types.Object` (Key, Size, LastModified, ETag)
- **Config loading**: `config.LoadDefaultConfig(ctx, config.WithRegion(region), config.WithSharedConfigProfile(profile))`
  - Credential resolution order: env vars → shared credentials → shared config → EC2 IMDS → ECS
  - `local_profile` maps to `config.WithSharedConfigProfile()`
- **ListObjectsV2 pagination**: `s3.NewListObjectsV2Paginator(client, input, optFns)` — handles continuation tokens automatically
  - `paginator.HasMorePages()` / `paginator.NextPage(ctx)` loop
  - Each page yields `[]types.Object` in `Contents`
- **GetObject**: `client.GetObject(ctx, &s3.GetObjectInput{Bucket, Key})` → `GetObjectOutput{Body: io.ReadCloser, ContentLength, LastModified}`
- **LocalStack/MinIO support**: Use `s3.WithEndpointResolverV2()` or `aws.EndpointResolverWithOptionsFunc` to point at `http://localhost:4566` (LocalStack) or custom MinIO endpoint
- **Custom endpoint for testing**: Pass `func(o *s3.Options) { o.BaseEndpoint = aws.String(endpoint); o.UsePathStyle = true }` to `s3.NewFromConfig(cfg, optFns...)`

### Alternatives Considered
- **aws-sdk-go v1**: Deprecated; not recommended for new projects.
- **MinIO Go client**: Would work for S3 but adds a non-standard dependency; aws-sdk-go-v2 is universally compatible.

---

## 4. Plugin Directory Layout

### Decision
Follow the standard CQ source plugin layout as demonstrated by `cq-source-xkcd`, adapted for our S3-specific needs.

### Rationale
Consistency with the CQ plugin ecosystem ensures discoverability, reduces onboarding friction, and follows established patterns that work with CQ tooling (e.g., `cloudquery tables`, `cloudquery sync`, `.goreleaser.yaml`).

### Key Findings

- **Reference**: `github.com/hermanschaaf/cq-source-xkcd` (uses SDK v4.19.0)
- **Entry point**: `main.go` → `serve.Plugin(plugin.NewPlugin(...)).Serve(ctx)`
- **Plugin wiring**: `plugin/plugin.go` → `plugin.NewPlugin("cq-source-s3", version, client.Configure, options...)`
- **Client**: `client/client.go` — struct with S3 client, spec, state client, logger; embeds `plugin.UnimplementedDestination`
- **Spec**: `client/spec.go` — `Spec` struct with JSON tags, `Validate()`, `SetDefaults()`
- **Configure func**: Unmarshals spec JSON, validates, creates S3 client, returns `plugin.Client`
- **Tables**: Built dynamically via `Tables()` method — list S3, group by prefix, read schemas, build `schema.Table` objects
- **Sync**: Direct streaming — no scheduler; iterate tables, process objects with concurrency semaphore, emit SyncMigrateTable then SyncInsert

### Alternatives Considered
- **Monorepo with multiple packages**: Overkill for a single-purpose plugin.
- **Using `resources/services/` pattern from xkcd**: Not a good fit since our tables are dynamic (discovered at runtime), not static.

---

## 5. Incremental Sync & State Backend

### Decision
Use the CQ SDK `state` package with cursor keys formatted as `s3/{bucket}/{tableName}/last_modified_cursor` and RFC3339Nano timestamp values.

### Rationale
The SDK's state package provides a clean key-value interface that gracefully degrades to a no-op when no backend is configured. RFC3339Nano preserves sub-second precision of S3 `LastModified` timestamps.

### Key Findings

- **State client**: `state.NewConnectedClient(ctx, opts.BackendOptions)` — returns `NoOpClient` when `BackendOptions` is nil
- **Key format**: `s3/{bucket}/{tableName}/last_modified_cursor`
- **Value format**: `time.Format(time.RFC3339Nano)` for write, `time.Parse(time.RFC3339Nano, val)` for read
- **Cursor comparison**: `object.LastModified > cursor` (strictly greater than, per FR-017)
- **Flush**: Call `stateClient.Flush(ctx)` after all tables are synced to persist cursors
- **Per-table cursor**: Each discovered table maintains its own cursor, tracking the max `LastModified` across all its contributing objects

### Alternatives Considered
- **Global cursor (single timestamp for all tables)**: Rejected because tables may have objects with very different update frequencies; per-table cursors are more precise.
- **ETag-based cursors**: S3 ETags are opaque and not ordered; timestamps are simpler and ordered.

---

## 6. Table Name Normalization

### Decision
Table names are derived from S3 key directory prefixes: strip the filename, join path segments with `_`, replace invalid characters with `_`. Root-level files use the filename without extension.

### Rationale
Matches the upstream `cloudquery/s3` plugin behavior and the spec's user stories. Deterministic naming ensures consistent table names across syncs.

### Key Findings

- **Rules**:
  - `data/2024/file.parquet` → table name `data_2024` (directory prefix, `/` → `_`)
  - `datafile_0.parquet` → table name `datafile_0` (root file, strip extension)
  - Invalid chars (hyphens, dots, spaces) → `_`
  - Multiple files under same prefix → single table
- **Implementation**: `internal/naming/naming.go` with pure functions, thoroughly unit-tested
- **Edge cases**: Empty prefix after normalization, leading/trailing underscores, consecutive underscores → collapse to single `_`

### Alternatives Considered
- **Full key path including filename**: Would produce unique tables per file; not what users expect.
- **Hash-based naming**: Not human-readable; debugging nightmare.

---

## 7. Concurrency & Temp File Strategy

### Decision
Use a semaphore (buffered channel) for concurrency control. Download S3 objects to OS temp files for Parquet random-access reading. Clean up temp files after processing.

### Rationale
Parquet requires random access (`io.ReaderAt + io.Seeker`) for footer reading. S3 `GetObject` returns a stream. Temp files provide the required interface without holding entire files in memory. Semaphore pattern is idiomatic Go.

### Key Findings

- **Semaphore**: `sem := make(chan struct{}, concurrency)` — `-1` concurrency means no semaphore (unlimited)
- **Temp file**: `os.CreateTemp("", "cq-s3-*.parquet")` → download `GetObject` body → seek to start → pass to `file.NewParquetReader`
- **Cleanup**: `defer os.Remove(tmpFile.Name())` per goroutine
- **Memory**: Only the current batch of rows is in memory at any time (controlled by `rows_per_record`)

### Alternatives Considered
- **In-memory buffer (`bytes.Reader`)**: Works for small files but violates FR-022 for multi-GB files.
- **S3 range requests to simulate seeking**: Complex, fragile, and slower than temp file for Parquet's footer-first access pattern.
