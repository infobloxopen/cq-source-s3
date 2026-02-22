# CloudQuery S3 Source Plugin

A [CloudQuery](https://cloudquery.io) source plugin that reads **Parquet** files
from AWS S3 buckets, auto-discovers tables from key prefixes, and emits Apache
Arrow record batches to any CloudQuery destination.

## Features

- **Auto-discovery**: Tables are derived from S3 key prefixes — no manual schema definition
- **Incremental sync**: Subsequent syncs skip already-ingested objects using a cursor
- **Configurable batching**: Control Arrow record batch size via `rows_per_record`
- **Parallel reads**: Concurrent S3 object processing via `concurrency` setting
- **Schema validation**: Files under the same prefix must share a compatible schema
- **Graceful error handling**: Deleted or malformed objects are warned and skipped

## Container Image

Pre-built multi-architecture Docker images (linux/amd64, linux/arm64) are published
on every release to GHCR.

### Pull and Run

```bash
docker run -p 7777:7777 ghcr.io/infobloxopen/cq-source-s3:latest
```

The plugin starts and listens on gRPC port 7777.

### With AWS Credentials

```bash
docker run -p 7777:7777 \
  -e AWS_ACCESS_KEY_ID=AKIA... \
  -e AWS_SECRET_ACCESS_KEY=... \
  -e AWS_REGION=us-east-1 \
  ghcr.io/infobloxopen/cq-source-s3:v1.2.3
```

### Custom Listen Address

```bash
docker run -p 8080:8080 ghcr.io/infobloxopen/cq-source-s3:latest serve --address "[::]:8080"
```

### Building the Image Locally

```bash
# Single-arch (current platform)
make docker-build

# Multi-arch (requires buildx)
docker buildx create --name multiarch --driver docker-container --use
make docker-build-multiarch
```

### Image Tags

Each release produces these tags:

| Tag | Example | Description |
|-----|---------|-------------|
| `v<semver>-<sha>` | `v1.2.3-abc1234` | Exact version + commit (primary) |
| `v<semver>` | `v1.2.3` | Semver release |
| `v<major>.<minor>` | `v1.2` | Minor release (rolls forward) |
| `latest` | `latest` | Most recent release |

## Installation

```bash
go install github.com/infobloxopen/cq-source-s3@latest
```

Or build from source:

```bash
git clone https://github.com/infobloxopen/cq-source-s3.git
cd cq-source-s3
go build -o cq-source-s3 .
```

## Configuration

Create a CloudQuery config file (e.g., `s3-to-postgres.yml`):

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
    # local_profile: "my-profile"   # Optional: use a named AWS profile
    # filetype: "parquet"           # Default (only supported format)
    # rows_per_record: 500          # Default: 500 rows per Arrow record batch
    # concurrency: 50               # Default: 50 parallel S3 reads (-1 = unlimited)
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

## Running

```bash
# Full sync
cloudquery sync s3-to-postgres.yml

# Subsequent syncs (with backend_options) only fetch new/updated objects
cloudquery sync s3-to-postgres.yml
```

## Table Naming Rules

Tables are auto-discovered from S3 key prefixes:

| S3 Object Key | Table Name |
|---|---|
| `datafile_0.parquet` | `datafile_0` |
| `data/2024/file.parquet` | `data_2024` |
| `data/2024/01/14/file.parquet` | `data_2024_01_14` |
| `reports/2024/a.parquet` + `reports/2024/b.parquet` | `reports_2024` |

**Rules:**

- Directory prefix segments are joined with `_`
- Root-level files use the filename without extension
- Invalid characters (hyphens, dots, spaces) become `_`
- Consecutive underscores are collapsed
- Multiple files under the same prefix contribute rows to a single table
- All files under a prefix must have the same Arrow schema

## Incremental Sync

When `backend_options` is configured:

1. **First sync**: All objects are fetched; per-table cursors are stored
2. **Subsequent syncs**: Only objects with `LastModified > cursor` are fetched
3. **No backend**: Every sync fetches all objects (full sync)

Cursor keys follow the format `s3/{bucket}/{table}/last_modified_cursor`.

## Spec Reference

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `bucket` | string | **Yes** | — | S3 bucket name |
| `region` | string | **Yes** | — | AWS region (e.g., `us-east-1`) |
| `local_profile` | string | No | `""` | Named AWS profile for authentication |
| `path_prefix` | string | No | `""` | Only sync objects under this key prefix |
| `filetype` | string | No | `"parquet"` | File format (only `"parquet"` supported) |
| `rows_per_record` | int | No | `500` | Max rows per Arrow record batch |
| `concurrency` | int | No | `50` | Max parallel S3 reads (`-1` = unlimited) |

## Development

### Prerequisites

- Go 1.25+
- Docker (for E2E tests with LocalStack)

### Building and Testing

```bash
make build      # Build the binary
make test       # Run unit tests
make lint       # Run linter
make tidy       # Tidy Go modules

# E2E tests (requires LocalStack)
docker-compose -f test/docker-compose.yml up -d
make e2e
```

### Project Structure

```
main.go                 # Entry point
plugin/plugin.go        # Plugin wiring
client/
  client.go             # Client struct, Configure, Tables, Sync, Close
  spec.go               # Spec struct, SetDefaults, Validate
  discover.go           # S3 listing, prefix grouping, schema validation
  sync.go               # Sync orchestration, concurrency, error handling
  cursor.go             # State backend cursor read/write
  parquet.go            # Parquet reading and streaming
internal/
  naming/naming.go      # Table name normalization
  testutil/             # Shared test helpers
test/
  e2e_test.go           # E2E tests against LocalStack
  docker-compose.yml    # LocalStack service
```

## License

See [LICENSE](LICENSE) for details.
