# Quickstart: CloudQuery S3 Source Plugin

**Feature**: 001-s3-source-plugin
**Date**: 2026-02-21

---

## Prerequisites

- [CloudQuery CLI](https://docs.cloudquery.io/docs/quickstart) installed
- AWS credentials configured (environment variables, `~/.aws/credentials`, or instance role)
- An S3 bucket containing Parquet files
- A destination plugin configured (e.g., PostgreSQL, SQLite, file)

## Configuration

Create a file `s3-to-postgres.yml`:

```yaml
kind: source
spec:
  name: "s3"
  path: "infobloxopen/cq-source-s3"
  registry: "grpc"
  version: "v1.0.0"
  tables: ["*"]
  destinations: ["postgresql"]
  backend_options:
    table_name: "cq_state_s3"
    connection: "@@plugins.postgresql.connection"
  spec:
    bucket: "my-data-bucket"
    region: "us-east-1"
    # path_prefix: "data/2024/"     # Optional: only sync objects under this prefix
    # local_profile: "my-profile"    # Optional: use a named AWS profile
    # filetype: "parquet"            # Default: "parquet" (only supported format)
    # rows_per_record: 500           # Default: 500 rows per Arrow record batch
    # concurrency: 50                # Default: 50 parallel S3 object reads (-1 = unlimited)
---
kind: destination
spec:
  name: "postgresql"
  path: "cloudquery/postgresql"
  registry: "cloudquery"
  version: "v8.0.0"
  spec:
    connection_string: "postgresql://user:pass@localhost:5432/mydb?sslmode=disable"
```

## Running a Sync

```bash
# Full sync — reads all Parquet files and creates tables in the destination
cloudquery sync s3-to-postgres.yml

# Subsequent syncs with backend_options — only new/updated files are processed
cloudquery sync s3-to-postgres.yml
```

## Table Naming

Tables are auto-discovered from S3 key prefixes:

| S3 Object Key | Derived Table Name |
|---|---|
| `datafile_0.parquet` | `datafile_0` |
| `data/2024/datafile_1.parquet` | `data_2024` |
| `data/2024/01/14/datafile_2.parquet` | `data_2024_01_14` |
| `reports/2024/a.parquet` + `reports/2024/b.parquet` | `reports_2024` (merged) |

**Rules**:
- Directory prefix segments are joined with `_`
- Root-level files use the filename without extension
- Invalid characters (hyphens, dots, spaces) are replaced with `_`
- Multiple files under the same prefix contribute rows to a single table

## Listing Discovered Tables

```bash
cloudquery tables s3-to-postgres.yml
```

This outputs the list of auto-discovered table names based on the current bucket contents.

## Incremental Sync

When `backend_options` is configured in the source spec:

1. **First sync**: All objects are fetched; cursors are stored per table
2. **Subsequent syncs**: Only objects with `LastModified > stored_cursor` are fetched
3. **No backend**: Every sync fetches all objects (full sync)

Cursors are stored with keys like `s3/{bucket}/{tableName}/last_modified_cursor`.

## Spec Reference

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `bucket` | string | **Yes** | — | S3 bucket name |
| `region` | string | **Yes** | — | AWS region (e.g., `us-east-1`) |
| `local_profile` | string | No | `""` | Named AWS profile for authentication |
| `path_prefix` | string | No | `""` | Only sync objects under this S3 key prefix |
| `filetype` | string | No | `"parquet"` | File format (only `"parquet"` supported) |
| `rows_per_record` | int | No | `500` | Max rows per Arrow record batch |
| `concurrency` | int | No | `50` | Max parallel S3 object reads (`-1` = unlimited) |

## Building from Source

```bash
git clone https://github.com/infobloxopen/cq-source-s3.git
cd cq-source-s3
go build -o cq-source-s3 .

# Run tests (requires Docker for E2E)
go test ./...
```

## Local Development with LocalStack

```bash
# Start LocalStack
docker-compose -f test/docker-compose.yml up -d

# Seed test data
aws --endpoint-url=http://localhost:4566 s3 mb s3://test-bucket
aws --endpoint-url=http://localhost:4566 s3 cp test/testdata/ s3://test-bucket/ --recursive

# Run E2E tests
go test -v ./test/...
```
