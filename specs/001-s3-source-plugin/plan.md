# Implementation Plan: CloudQuery S3 Source Plugin

**Branch**: `001-s3-source-plugin` | **Date**: 2026-02-21 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/001-s3-source-plugin/spec.md`

## Summary

Build a CloudQuery source plugin (`cq-source-s3`) that reads Parquet files from AWS S3 buckets, auto-discovers tables from S3 key prefix structure, and emits Apache Arrow record batches to any CloudQuery destination. The plugin supports incremental sync via the CQ state backend, configurable batching (`rows_per_record`), and concurrency control. Implementation uses CloudQuery Plugin SDK v4, aws-sdk-go-v2 for S3 operations, and apache/arrow-go v18 for Parquet reading. E2E tests run against LocalStack/MinIO on every CI commit.

## Technical Context

**Language/Version**: Go (latest stable, e.g., 1.25 per Constitution Principle I)
**Primary Dependencies**:
- `github.com/cloudquery/plugin-sdk/v4` (v4.94.x) — plugin framework, schema, message, state, serve
- `github.com/aws/aws-sdk-go-v2` — S3 client (`service/s3`), config (`config`), types
- `github.com/apache/arrow-go/v18` — Parquet reading (`parquet/file`, `parquet/pqarrow`), Arrow records
- `github.com/rs/zerolog` — structured logging (SDK dependency)
**Storage**: AWS S3 (read-only source); CloudQuery state backend for incremental cursors
**Testing**: Go `testing` package; `go test ./...`; E2E against LocalStack/MinIO via Docker
**Target Platform**: Linux/macOS (CloudQuery CLI host), no CGO
**Project Type**: CloudQuery source plugin (gRPC server binary)
**Performance Goals**: Stream Parquet files without full-file memory loading; batch sizes controlled by `rows_per_record` (default 500); parallel object processing controlled by `concurrency` (default 50)
**Constraints**: No CGO unless justified; no credentials in logs; graceful handling of deleted-between-list-and-read objects
**Scale/Scope**: Buckets with 1000s of Parquet objects, multi-GB individual files, paginated listing

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| # | Principle | Status | Notes |
|---|-----------|--------|-------|
| I | Latest Go Version | **PASS** | `go.mod` will declare latest stable Go (e.g., 1.25) |
| II | Test-Driven Development | **PASS** | Plan mandates Red-Green-Refactor for every feature; tasks will be structured test-first |
| III | End-to-End S3 Testing | **PASS** | E2E tests against LocalStack/MinIO exercising full sync pipeline with real Parquet uploads |
| IV | Documentation Synchronization | **PASS** | README, GoDoc, and spec/plan docs updated alongside code in every PR |
| V | Codebase Consistency Scanning | **PASS** | Consistency scan step included before PR; `go mod tidy`, vet, lint gates |
| VI | E2E Tests Always Run | **PASS** | CI workflow runs E2E on every push/PR; no skip flags; Docker provisions LocalStack automatically |

**Gate Result**: **PASS** — all 6 principles satisfied. Proceeding to Phase 0.

## Project Structure

### Documentation (this feature)

```text
specs/001-s3-source-plugin/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
│   └── source-plugin.go # Go interface contract
└── tasks.md             # Phase 2 output (created by /speckit.tasks)
```

### Source Code (repository root)

```text
main.go                          # Entry point: serve.Plugin(...).Serve(ctx)
go.mod
go.sum
Makefile
.goreleaser.yaml
.github/
└── workflows/
    └── ci.yml                   # CI: lint, test, E2E (LocalStack)

plugin/
└── plugin.go                    # NewPlugin() wiring, version, Configure func

client/
├── client.go                    # Client struct (SourceClient impl), Sync, Tables, Close
├── spec.go                      # Spec struct, JSON tags, Validate(), SetDefaults()
├── discover.go                  # Table discovery: ListObjectsV2 → group by prefix → table names
├── sync.go                      # Sync orchestration: concurrency, per-table streaming
├── cursor.go                    # State backend cursor read/write, key formatting
└── parquet.go                   # Parquet reader: download to temp file, pqarrow streaming

resources/
└── plugin/
    ├── plugin.go                # Alternative plugin wiring (if needed)
    └── client.go                # Resource-level client helpers

internal/
├── naming/
│   └── naming.go                # Table name normalization (prefix → underscore-separated)
└── testutil/
    └── testutil.go              # Shared test helpers (bucket seeding, etc.)

test/
├── e2e_test.go                  # E2E tests against LocalStack/MinIO
└── docker-compose.yml           # LocalStack service definition

docs/
└── tables/                      # Auto-generated table documentation (if applicable)
```

**Structure Decision**: Single-project Go binary following the standard CloudQuery source plugin layout (modeled on `cq-source-xkcd`). The `client/` package owns all source-plugin logic. The `internal/naming/` package isolates the reusable table-name normalization. E2E tests live in `test/` with a Docker Compose file for LocalStack.

## Complexity Tracking

> No constitution violations detected. All 6 principles are satisfied by the plan as designed.
