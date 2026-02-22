# Data Model: CloudQuery S3 Source Plugin

**Phase**: 1 — Design & Contracts
**Feature**: 001-s3-source-plugin
**Date**: 2026-02-21

---

## Entities

### 1. Spec (Configuration)

The user-facing configuration struct, deserialized from the YAML `spec` block.

| Field | Type | Required | Default | Validation | Description |
|-------|------|----------|---------|------------|-------------|
| `bucket` | `string` | Yes | — | Non-empty | S3 bucket name |
| `region` | `string` | Yes | — | Non-empty | AWS region |
| `local_profile` | `string` | No | `""` | — | Named AWS profile for auth; empty = default credential chain |
| `path_prefix` | `string` | No | `""` | — | S3 key prefix filter; only objects under this prefix are discovered |
| `filetype` | `string` | No | `"parquet"` | Must be `"parquet"` | File format to read; only Parquet supported in v1 |
| `rows_per_record` | `int` | No | `500` | ≥ 1 | Max rows per Arrow record batch |
| `concurrency` | `int` | No | `50` | Any int; -1 = unlimited | Max parallel S3 object reads |

**Go struct**: `client/spec.go`

```go
type Spec struct {
    Bucket        string `json:"bucket"`
    Region        string `json:"region"`
    LocalProfile  string `json:"local_profile,omitempty"`
    PathPrefix    string `json:"path_prefix,omitempty"`
    FileType      string `json:"filetype,omitempty"`
    RowsPerRecord int    `json:"rows_per_record,omitempty"`
    Concurrency   int    `json:"concurrency,omitempty"`
}
```

**Methods**:
- `SetDefaults()` — applies default values for `FileType` ("parquet"), `RowsPerRecord` (500), `Concurrency` (50)
- `Validate() error` — returns error if `Bucket` empty, `Region` empty, `FileType` not "parquet", or `RowsPerRecord` < 1

**State transitions**: None (immutable after validation).

---

### 2. DiscoveredTable

A logical table derived from grouping S3 objects by their key prefix.

| Field | Type | Description |
|-------|------|-------------|
| `Name` | `string` | Normalized table name (prefix segments joined by `_`) |
| `Prefix` | `string` | Original S3 key prefix (e.g., `data/2024/`) |
| `Objects` | `[]S3Object` | List of S3 objects belonging to this table |
| `Schema` | `*arrow.Schema` | Arrow schema derived from the first Parquet file's metadata |
| `Table` | `*schema.Table` | CQ SDK table with columns, CQ IDs, and `IsIncremental = true` |

**Lifecycle**:
1. **Discovery**: `ListObjectsV2` → group by prefix → create `DiscoveredTable` per unique prefix
2. **Schema binding**: Read first Parquet file's schema → set `Schema` and build `Table`
3. **Validation**: If a subsequent file has a different schema → fail fast with error (FR-013)
4. **Sync**: Iterate `Objects`, emit `SyncMigrateTable` then stream records as `SyncInsert`

**Not persisted** — rebuilt on every sync invocation.

---

### 3. S3Object

Metadata about a single S3 object discovered during listing.

| Field | Type | Description |
|-------|------|-------------|
| `Key` | `string` | Full S3 object key |
| `Size` | `int64` | Object size in bytes |
| `LastModified` | `time.Time` | S3 LastModified timestamp |
| `ETag` | `string` | S3 ETag (informational, not used for cursor) |

**Source**: `types.Object` from `ListObjectsV2` response.

**Filtering**:
- Must end with extension matching `filetype` (e.g., `.parquet`)
- Must be under `path_prefix` (already filtered by the ListObjectsV2 `Prefix` parameter)
- In incremental mode: `LastModified > cursor` for the object's table

---

### 4. SyncCursor

Per-table state for incremental sync, persisted via the CQ state backend.

| Field | Type | Description |
|-------|------|-------------|
| Key | `string` | Format: `s3/{bucket}/{tableName}/last_modified_cursor` |
| Value | `string` | RFC3339Nano formatted timestamp of the max `LastModified` seen |

**Operations**:
- **Read**: `stateClient.GetKey(ctx, key)` → parse as `time.RFC3339Nano` → `time.Time`
- **Write**: `stateClient.SetKey(ctx, key, maxLastModified.Format(time.RFC3339Nano))`
- **Flush**: `stateClient.Flush(ctx)` after all tables complete
- **No backend**: `state.NewConnectedClient` returns `NoOpClient`; `GetKey` returns `""` (empty) → cursor is zero-time → all objects pass filter

**State transitions**:
```
[No cursor] → first sync → [cursor = max(LastModified across all objects in table)]
[cursor = T1] → new objects with LastModified > T1 → [cursor = max(new LastModified values)]
[cursor = T1] → no new objects → [cursor = T1] (unchanged)
```

---

### 5. RecordBatch (Arrow Record)

An Apache Arrow record batch emitted as a `message.SyncInsert`.

| Property | Type | Description |
|----------|------|-------------|
| `Schema` | `*arrow.Schema` | Column names and types from Parquet metadata |
| `NumRows` | `int` | Up to `rows_per_record` rows |
| `Columns` | `[]arrow.Array` | Column data arrays |

**Lifecycle**:
1. `pqarrow.FileReader.GetRecordReader(ctx, nil, nil)` creates a streaming reader with `BatchSize = rows_per_record`
2. Each `recordReader.Read()` yields one `arrow.Record` with up to `rows_per_record` rows
3. Wrapped in `message.SyncInsert{Record: record}` and sent to the `res` channel
4. The CQ SDK handles delivery to the destination

---

## Relationships

```
Spec (1) ──configures──> Client (1)
Client (1) ──discovers──> DiscoveredTable (0..*)
DiscoveredTable (1) ──contains──> S3Object (1..*)
DiscoveredTable (1) ──has──> SyncCursor (0..1)  [only when backend_options configured]
S3Object (1) ──produces──> RecordBatch (1..*)
DiscoveredTable (1) ──emits──> schema.Table (1)  [via SyncMigrateTable message]
RecordBatch (1) ──emits──> SyncInsert (1)        [via SyncInsert message]
```

## Validation Rules Summary

| Entity | Rule | Error |
|--------|------|-------|
| Spec | `bucket` non-empty | "bucket is required" |
| Spec | `region` non-empty | "region is required" |
| Spec | `filetype` = "parquet" | "unsupported filetype: {value}; supported: parquet" |
| Spec | `rows_per_record` ≥ 1 | "rows_per_record must be at least 1" |
| DiscoveredTable | All files have same Arrow schema | "schema mismatch in table {name}: file {a} has {cols_a}, file {b} has {cols_b}" |
| S3Object | Must have `.parquet` extension | (silently filtered out during discovery) |
| SyncCursor | Value must parse as RFC3339Nano | (log warning, treat as zero-time) |
