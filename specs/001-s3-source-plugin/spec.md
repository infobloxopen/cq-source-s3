# Feature Specification: CloudQuery S3 Source Plugin

**Feature Branch**: `001-s3-source-plugin`
**Created**: 2026-02-21
**Status**: Draft
**Input**: User description: "Build a CloudQuery source plugin for S3 that mirrors the behavior of the upstream cloudquery/s3 source plugin"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Basic Full Sync from S3 Bucket (Priority: P1)

A CloudQuery user configures the plugin with a bucket name, region, and runs `cloudquery sync`. The plugin connects to S3, discovers all Parquet files, derives table names from the object key prefixes, reads the Parquet data, and emits Apache Arrow record batches to the configured destination.

**Why this priority**: This is the core value proposition — reading Parquet files from S3 and syncing rows into a destination. Without this, no other feature has meaning.

**Independent Test**: Seed an S3-compatible bucket with Parquet files at various key prefixes, configure the plugin with `bucket` and `region`, run a sync, and verify that every expected table appears in the destination with correct row counts and column values.

**Acceptance Scenarios**:

1. **Given** an S3 bucket containing `datafile_0.parquet` at the root, **When** the user runs a sync, **Then** a table named `datafile_0` is created with all rows from the file.
2. **Given** an S3 bucket containing `data/2024/datafile_1.parquet`, **When** the user runs a sync, **Then** a table named `data_2024` is created with all rows from the file.
3. **Given** an S3 bucket containing `data/2024/01/14/datafile_2.parquet`, **When** the user runs a sync, **Then** a table named `data_2024_01_14` is created with all rows from the file.
4. **Given** an S3 bucket containing `data/2024/02/14/14/15/datafile_3.parquet`, **When** the user runs a sync, **Then** a table named `data_2024_02_14_14_15` is created with all rows from the file.
5. **Given** an S3 bucket with two Parquet files under the same prefix `reports/2024/a.parquet` and `reports/2024/b.parquet`, **When** the user runs a sync, **Then** both files contribute rows to a single table `reports_2024`.
6. **Given** an S3 bucket containing a mix of `.parquet` and `.csv` files, **When** the user runs a sync with the default `filetype: parquet`, **Then** only `.parquet` files are processed; `.csv` files are ignored.
7. **Given** a Parquet file with columns (id: int64, name: utf8, created_at: timestamp), **When** synced, **Then** the destination table schema matches those column names and types faithfully.

---

### User Story 2 - Plugin Configuration & Validation (Priority: P1)

A CloudQuery user writes a YAML source configuration specifying the plugin's `spec` block with fields like `bucket`, `region`, `path_prefix`, `rows_per_record`, `concurrency`, and `local_profile`. The plugin validates the configuration at startup and rejects invalid or incomplete configurations with clear, actionable error messages.

**Why this priority**: Configuration is the user's primary interface. Invalid configs must be caught early with clear messages to prevent wasted debugging time. This is co-equal with P1 because without correct config parsing, syncing cannot work.

**Independent Test**: Provide various valid and invalid YAML configurations and verify that the plugin accepts or rejects them with appropriate messages.

**Acceptance Scenarios**:

1. **Given** a configuration with `bucket: "my-bucket"` and `region: "us-east-1"`, **When** the plugin initializes, **Then** it accepts the configuration and proceeds.
2. **Given** a configuration missing `bucket`, **When** the plugin initializes, **Then** it returns an error stating that `bucket` is required.
3. **Given** a configuration missing `region`, **When** the plugin initializes, **Then** it returns an error stating that `region` is required.
4. **Given** a configuration with no `rows_per_record` specified, **When** the plugin initializes, **Then** it defaults to 500.
5. **Given** a configuration with `rows_per_record: 0` or a negative number, **When** the plugin initializes, **Then** it returns a validation error.
6. **Given** a configuration with no `concurrency` specified, **When** the plugin initializes, **Then** it defaults to 50.
7. **Given** a configuration with `concurrency: -1`, **When** the plugin initializes, **Then** concurrency is treated as unlimited (no cap on parallel object reads).
8. **Given** a configuration with `local_profile: "my-profile"`, **When** the plugin initializes, **Then** it uses that named AWS profile for authentication.
9. **Given** a configuration with no `local_profile` specified, **When** the plugin initializes, **Then** it uses the default AWS credential chain (environment variables, instance role, etc.).
10. **Given** a configuration with `path_prefix: "data/2024/"`, **When** the plugin lists objects, **Then** only objects under that prefix are considered for discovery.
11. **Given** a configuration with `filetype: "csv"` (unsupported), **When** the plugin initializes, **Then** it returns an error listing supported file types.

---

### User Story 3 - Auto-Discovered Table Naming (Priority: P1)

A CloudQuery user runs `cloudquery tables` or a sync and the plugin auto-discovers tables based on the directory structure (key prefixes) of objects in the bucket. The naming convention is deterministic and consistent.

**Why this priority**: Table discovery is inseparable from the core sync; it defines what the user sees in their destination. Incorrect or inconsistent naming renders the plugin unusable.

**Independent Test**: Seed a bucket with objects at various key depths, run table discovery, and verify the resulting table names match expectations.

**Acceptance Scenarios**:

1. **Given** an object at the bucket root `datafile_0.parquet`, **When** discovery runs, **Then** the table name is the filename without extension: `datafile_0`.
2. **Given** an object at `data/2024/datafile_1.parquet`, **When** discovery runs, **Then** the table name is derived from the directory prefix with `/` replaced by `_`: `data_2024`.
3. **Given** multiple objects under the same prefix `data/2024/01/14/`, **When** discovery runs, **Then** all files contribute to a single table `data_2024_01_14`.
4. **Given** objects at different depths under `data/`, **When** discovery runs, **Then** each unique directory prefix produces a separate table (e.g., `data/2024/` → `data_2024`, `data/2025/` → `data_2025`).
5. **Given** a `path_prefix` of `data/2024/`, **When** discovery runs, **Then** only objects under that prefix are discovered; objects outside it are not listed.
6. **Given** a prefix containing characters that are invalid for destination table names (e.g., hyphens, dots), **When** discovery runs, **Then** those characters are normalized to underscores and the table name is valid across common destinations.

---

### User Story 4 - Incremental Sync via Backend State (Priority: P2)

A CloudQuery user enables incremental syncing by configuring `backend_options` in the top-level sync configuration. On subsequent syncs, the plugin only fetches S3 objects whose `LastModified` timestamp is newer than the stored cursor, avoiding re-ingesting unchanged data.

**Why this priority**: Incremental sync is critical for production workloads with large, growing buckets, but it builds on top of the full-sync infrastructure from P1.

**Independent Test**: Run a full sync, record the state, upload a new Parquet file, run a second sync, and verify only the new file's rows appear.

**Acceptance Scenarios**:

1. **Given** `backend_options` is configured and a first sync completes, **When** no new objects are added and a second sync runs, **Then** the second sync produces zero new rows and completes quickly.
2. **Given** a first sync has completed and a new Parquet file is uploaded with a `LastModified` after the cursor, **When** a second sync runs, **Then** only the new file's rows are ingested.
3. **Given** `backend_options` is not configured, **When** a sync runs, **Then** all objects are fetched every time (full sync behavior).
4. **Given** incremental mode and an existing Parquet object is replaced (same key, new `LastModified`), **When** a sync runs, **Then** the updated object is re-fetched (objects are assumed immutable; the new `LastModified` triggers re-ingestion).
5. **Given** incremental mode and two files have the exact same `LastModified` timestamp, **When** a sync runs, **Then** both files are correctly ingested on the first sync and neither is re-ingested on subsequent syncs (no duplicates, no missed files).

---

### User Story 5 - Batching and Concurrency Controls (Priority: P2)

A CloudQuery user configures `rows_per_record` and `concurrency` to control memory usage and parallelism. The plugin respects these settings to handle buckets of varying sizes efficiently.

**Why this priority**: Batching and concurrency are important for production reliability and performance but rely on the core sync path being functional first.

**Independent Test**: Configure various `rows_per_record` and `concurrency` values, sync a bucket with many objects, and verify via logs and timing that the controls measurably affect behavior.

**Acceptance Scenarios**:

1. **Given** `rows_per_record: 100` and a Parquet file with 1000 rows, **When** synced, **Then** the plugin emits 10 record batches of approximately 100 rows each.
2. **Given** `rows_per_record: 500` (default), **When** synced, **Then** each record batch contains up to 500 rows.
3. **Given** `concurrency: 1`, **When** syncing a bucket with 10 files, **Then** files are processed sequentially (one at a time), observable via log timestamps.
4. **Given** `concurrency: 50` (default), **When** syncing a bucket with 100 files, **Then** up to 50 files are processed in parallel.
5. **Given** `concurrency: -1` (unlimited), **When** syncing a bucket with many files, **Then** all files are processed concurrently with no artificial limit.

---

### User Story 6 - Observability and Error Handling (Priority: P3)

A CloudQuery user can observe plugin behavior through structured logs and receives clear, actionable error messages when things go wrong, without credentials being leaked in log output.

**Why this priority**: Observability is a quality-of-life feature that improves debugging and operational confidence. It is not required for basic functionality but is essential for production readiness.

**Independent Test**: Trigger various error conditions and inspect log output; verify no credentials appear in any log line.

**Acceptance Scenarios**:

1. **Given** a sync is running, **When** the plugin discovers tables, **Then** it logs the number of tables found and their names.
2. **Given** a sync is running, **When** the plugin processes each file, **Then** it logs the object key, size, and number of rows read.
3. **Given** a bucket name that does not exist, **When** the plugin attempts to list objects, **Then** it returns a clear error: "bucket not found" or equivalent.
4. **Given** insufficient IAM permissions, **When** the plugin attempts to list or read objects, **Then** it returns an access-denied error with a suggestion to check credentials and permissions.
5. **Given** a malformed Parquet file that cannot be parsed, **When** the plugin encounters it, **Then** it logs a warning with the object key and continues processing other files.
6. **Given** any error or log output, **When** inspected, **Then** no AWS credentials, secret keys, or session tokens appear.

---

### Edge Cases

- What happens when the bucket is completely empty? The plugin completes with zero tables and zero rows, logging a warning.
- What happens when `path_prefix` matches no objects? Same as empty bucket — zero tables, zero rows, a warning log.
- What happens when a Parquet file has zero rows? The table is still created with the correct schema but no rows are emitted.
- What happens when two files under the same table prefix have different schemas (different columns or types)? The plugin fails fast with an actionable error naming the conflicting files, conflicting columns, and expected vs. actual types.
- What happens when an S3 object listed during discovery is deleted before the plugin reads it? The plugin logs a warning for the missing object and continues with remaining files.
- What happens with very large Parquet files (multiple GB)? The plugin streams data in batches controlled by `rows_per_record` rather than loading the entire file into memory.
- What happens when S3 pagination is needed (>1000 objects)? The plugin uses continuation tokens to iterate through all pages.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Plugin MUST accept a `bucket` (string, required) configuration field specifying the S3 bucket name.
- **FR-002**: Plugin MUST accept a `region` (string, required) configuration field specifying the AWS region.
- **FR-003**: Plugin MUST accept an optional `local_profile` (string) field; when set, the named AWS profile is used for authentication; when omitted, the default credential chain is used.
- **FR-004**: Plugin MUST accept an optional `path_prefix` (string) field; when set, only objects whose keys start with this prefix are listed and synced.
- **FR-005**: Plugin MUST accept an optional `filetype` (string, default `"parquet"`) field; only `"parquet"` is supported in this version; other values MUST produce a validation error.
- **FR-006**: Plugin MUST accept an optional `rows_per_record` (integer, default 500) field controlling the maximum number of rows per Apache Arrow record batch; values less than 1 MUST produce a validation error.
- **FR-007**: Plugin MUST accept an optional `concurrency` (integer, default 50) field controlling the maximum number of S3 objects processed in parallel; negative values MUST mean unlimited concurrency.
- **FR-008**: Plugin MUST discover tables by listing all objects in the bucket (respecting `path_prefix`), grouping objects by their directory prefix, and deriving table names by replacing `/` separators with `_`.
- **FR-009**: For objects at the bucket root (no `/` in key), the table name MUST be the filename without its extension.
- **FR-010**: Table names MUST be normalized so that characters invalid for common destination systems (hyphens, dots, spaces, etc.) are replaced with underscores.
- **FR-011**: Plugin MUST use S3 list pagination (continuation tokens) to handle buckets with more than 1000 objects.
- **FR-012**: Plugin MUST read Parquet file schemas to generate table schemas; each table's columns and types MUST faithfully represent the Parquet metadata.
- **FR-013**: When multiple Parquet files contribute to a single table and their schemas differ, the plugin MUST fail with an actionable error identifying the conflicting files and columns.
- **FR-014**: Plugin MUST emit rows as Apache Arrow record batches sized according to `rows_per_record`.
- **FR-015**: Plugin MUST process objects concurrently up to the limit set by `concurrency`.
- **FR-016**: Plugin MUST support incremental syncing via CloudQuery backend state when `backend_options` is configured; the cursor MUST use S3 object `LastModified` timestamps.
- **FR-017**: The incremental cursor comparison MUST use `>` (strictly greater than) against the stored timestamp; files with `LastModified` exactly equal to the cursor are NOT re-fetched, since they were ingested in the prior sync.
- **FR-018**: When `backend_options` is not configured, every sync MUST fetch all objects (full sync).
- **FR-019**: Plugin MUST NOT log or expose AWS credentials, secret keys, or session tokens in any output.
- **FR-020**: Plugin MUST log discovery results (table names and object counts), per-file progress (key, size, row count), and errors in a structured format.
- **FR-021**: Plugin MUST gracefully handle missing objects (deleted between listing and reading) by logging a warning and continuing.
- **FR-022**: Plugin MUST stream Parquet data rather than loading entire files into memory, to support large files.

### Key Entities

- **Spec (Configuration)**: The user-facing configuration block with fields: `bucket`, `region`, `local_profile`, `path_prefix`, `filetype`, `rows_per_record`, `concurrency`. Controls all plugin behavior.
- **Discovered Table**: A logical table derived from a unique S3 key prefix. Has a name (normalized from the prefix), a schema (derived from Parquet file metadata), and a set of contributing S3 objects.
- **S3 Object**: A file in the bucket identified by its key, size, and `LastModified` timestamp. The atomic unit of data to read.
- **Sync Cursor**: A per-table (or per-plugin) timestamp persisted via the CloudQuery state backend, representing the `LastModified` of the most recently ingested object. Used for incremental sync.
- **Record Batch**: An Apache Arrow record batch containing up to `rows_per_record` rows from one or more Parquet files belonging to the same table.

## Assumptions

- Objects in S3 are immutable: once written, their content does not change. If an object is overwritten (same key), it will have a new `LastModified` and be treated as a new object by incremental sync.
- Parquet files within the same table prefix share an identical schema. Schema evolution across files in the same prefix is not supported in this version.
- The plugin uses the standard AWS credential chain (environment variables → shared credentials file → instance metadata) unless `local_profile` is specified.
- The CloudQuery Plugin SDK handles registering the plugin, managing the gRPC lifecycle, and providing the state backend interface. The plugin implements the source plugin contract (Sync, GetDynamicTables, etc.).
- Table names derived from prefixes are case-sensitive and are not deduplicated across case variants (e.g., `Data/` and `data/` would produce separate tables).

## Non-Goals

- Supporting non-Parquet file formats (CSV, JSON, NDJSON) is explicitly out of scope. The `filetype` field exists for forward compatibility but only `"parquet"` is valid.
- Writing, transforming, or enriching data beyond what the CloudQuery SDK requires for sync correctness.
- Managing S3 bucket lifecycle policies, IAM roles/policies, or any AWS infrastructure resources.
- Schema evolution or schema merging across Parquet files with different column sets under the same prefix.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Given a bucket with the four example objects from the user stories, `cloudquery tables` lists exactly the four expected table names (`datafile_0`, `data_2024`, `data_2024_01_14`, `data_2024_02_14_14_15`).
- **SC-002**: A full sync loads 100% of rows from all matching Parquet objects with zero data loss (row counts in destination match source).
- **SC-003**: With backend state enabled, a second sync after no new uploads produces zero new inserts and completes in under 10% of the first sync's wall-clock time.
- **SC-004**: With backend state enabled, uploading a new Parquet object and re-syncing causes only that object's rows to appear in the destination.
- **SC-005**: Changing `rows_per_record` measurably changes the number of record batches emitted (verifiable via logs or unit tests).
- **SC-006**: Changing `concurrency` measurably changes the parallelism of object reads (verifiable via log timestamps or unit tests).
- **SC-007**: The README includes a working configuration example that matches upstream field names and defaults, and explains table naming rules and incremental sync limitations.
- **SC-008**: All E2E tests pass against a real S3-compatible service (LocalStack or MinIO) in CI on every commit.
