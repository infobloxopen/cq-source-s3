# Tasks: CloudQuery S3 Source Plugin

**Input**: Design documents from `/specs/001-s3-source-plugin/`
**Prerequisites**: plan.md ✅, spec.md ✅, research.md ✅, data-model.md ✅, contracts/ ✅

**Tests**: Included — Constitution Principle II (TDD) mandates test-first development.

**Organization**: Tasks grouped by user story for independent implementation and testing.

## Format: `[ID] [P?] [Story?] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story (US1–US6) this task belongs to
- Exact file paths included in all descriptions

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Initialize Go project, dependencies, build tooling, and CI

- [X] T001 Initialize Go module with `go mod init github.com/infobloxopen/cq-source-s3` and set Go version in go.mod
- [X] T002 Add core dependencies: plugin-sdk/v4, aws-sdk-go-v2 (config, service/s3), arrow-go/v18, zerolog in go.mod
- [X] T003 [P] Create project directory structure: plugin/, client/, internal/naming/, internal/testutil/, test/, docs/tables/
- [X] T004 [P] Create Makefile with targets: build, test, lint, vet, tidy
- [X] T005 [P] Create .goreleaser.yaml for release builds
- [X] T006 [P] Create .github/workflows/ci.yml with lint, unit test, and E2E stages (LocalStack via Docker Compose)
- [X] T007 [P] Create test/docker-compose.yml with LocalStack service definition (S3 on port 4566)

**Checkpoint**: Project compiles with `go build ./...` and CI pipeline is configured.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Entry point, plugin wiring, and shared test utilities that ALL user stories depend on

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T008 Create main.go entry point: `serve.Plugin(plugin.Plugin()).Serve(ctx)` in main.go
- [X] T009 Create plugin wiring in plugin/plugin.go: `NewPlugin()` calling `plugin.NewPlugin("cq-source-s3", version, client.Configure)`
- [X] T010 Create Client struct skeleton in client/client.go: embed `plugin.UnimplementedDestination`, fields for S3 client, spec, logger, state client
- [X] T011 Create Configure function signature in client/client.go matching `plugin.NewClientFunc` (unmarshal spec, return Client)
- [X] T012 [P] Create shared test helper for LocalStack S3 client in internal/testutil/testutil.go: S3 client pointing at localhost:4566, bucket creation, object upload utilities
- [X] T013 [P] Create shared test helper for generating Parquet test files in internal/testutil/parquet.go: create in-memory Parquet files with configurable schema and row count

**Checkpoint**: `go build ./...` succeeds; Configure returns a skeleton Client; test helpers available.

---

## Phase 3: User Story 2 — Plugin Configuration & Validation (Priority: P1)

**Goal**: Users write YAML `spec` blocks and get clear validation errors for invalid configs.

**Independent Test**: Pass various valid/invalid specs to `Validate()` and `SetDefaults()` and verify behavior.

> US2 is ordered before US1 because table naming (US3) and sync (US1) both depend on a validated, defaulted Spec.

### Tests for User Story 2

- [X] T014 [P] [US2] Write unit tests for Spec.SetDefaults() in client/spec_test.go: verify defaults for filetype, rows_per_record, concurrency
- [X] T015 [P] [US2] Write unit tests for Spec.Validate() in client/spec_test.go: missing bucket, missing region, invalid filetype, rows_per_record < 1, valid config, concurrency -1 allowed

### Implementation for User Story 2

- [X] T016 [US2] Implement Spec struct with JSON tags in client/spec.go per data-model.md
- [X] T017 [US2] Implement Spec.SetDefaults() in client/spec.go: filetype="parquet", rows_per_record=500, concurrency=50
- [X] T018 [US2] Implement Spec.Validate() in client/spec.go: bucket required, region required, filetype must be "parquet", rows_per_record >= 1
- [X] T019 [US2] Wire Spec into Configure function in client/client.go: json.Unmarshal, SetDefaults, Validate, create AWS config with region and optional local_profile

**Checkpoint**: `go test ./client/ -run TestSpec` passes; invalid configs produce clear errors.

---

## Phase 4: User Story 3 — Auto-Discovered Table Naming (Priority: P1)

**Goal**: S3 key prefixes are deterministically converted to valid table names.

**Independent Test**: Pass various S3 keys to `Normalize()` and verify expected table names.

### Tests for User Story 3

- [X] T020 [P] [US3] Write unit tests for table name normalization in internal/naming/naming_test.go: root file \u2192 filename sans extension, nested prefix \u2192 underscore-joined, hyphens/dots \u2192 underscores, consecutive separators collapsed, multiple files same prefix \u2192 same name

### Implementation for User Story 3

- [X] T021 [US3] Implement Normalize(key string) string in internal/naming/naming.go per spec rules: strip filename for nested keys, join prefix segments with `_`, replace invalid chars, collapse consecutive underscores

**Checkpoint**: `go test ./internal/naming/` passes all naming scenarios from spec.

---

## Phase 5: User Story 1 — Basic Full Sync from S3 Bucket (Priority: P1) 🎯 MVP

**Goal**: Plugin discovers Parquet files in S3, derives tables from key prefixes, reads Parquet data, and emits Arrow record batches to the destination.

**Independent Test**: Seed LocalStack bucket with Parquet files at various prefixes, run sync, verify all tables and rows appear correctly.

> This is the largest phase — it wires together discovery, Parquet reading, schema derivation, and the sync loop.

### Tests for User Story 1

- [X] T022 [P] [US1] Write unit tests for S3 object discovery and grouping in client/discover_test.go: mock S3 listing, verify prefix grouping, parquet extension filter, path_prefix filter, pagination handling
- [X] T023 [P] [US1] Write unit tests for Parquet schema reading in client/parquet_test.go: read schema from test Parquet file, verify Arrow schema columns and types match
- [X] T024 [P] [US1] Write unit tests for Parquet record streaming in client/parquet_test.go: stream records from test Parquet file, verify row counts and batch sizes
- [X] T025 [P] [US1] Write unit tests for schema mismatch detection in client/discover_test.go: two files with different schemas under same prefix → error with file names and column details
- [X] T026 [US1] Write integration test for full sync pipeline in test/e2e_test.go: seed LocalStack with Parquet files at 4 different prefix depths, run Sync(), verify SyncMigrateTable and SyncInsert messages for each table

### Implementation for User Story 1

- [X] T027 [US1] Implement S3 object listing with pagination in client/discover.go: ListObjectsV2Paginator, filter by .parquet extension, return []S3Object
- [X] T028 [US1] Implement prefix grouping in client/discover.go: group []S3Object by normalized table name (using internal/naming), return []DiscoveredTable
- [X] T029 [US1] Implement Parquet schema reader in client/parquet.go: download S3 object to temp file, open with file.NewParquetReader, extract Arrow schema via pqarrow.NewFileReader.Schema()
- [X] T030 [US1] Implement schema validation across files in client/discover.go: compare Arrow schemas of all files in a DiscoveredTable, fail fast on mismatch (FR-013)
- [X] T031 [US1] Implement CQ table builder in client/discover.go: schema.NewTableFromArrowSchema(), schema.AddCqIDs(), set IsIncremental=true, set table Name
- [X] T032 [US1] Implement Tables() method on Client in client/client.go: call discover, build CQ tables, apply TableOptions filter via schema.Tables.FilterDfs()
- [X] T033 [US1] Implement Parquet record streaming in client/parquet.go: download to temp file, pqarrow.FileReader.GetRecordReader with BatchSize=rows_per_record, yield arrow.Record batches
- [X] T034 [US1] Implement Sync() method skeleton in client/sync.go: iterate discovered tables, emit SyncMigrateTable, stream records from each object as SyncInsert
- [X] T035 [US1] Implement Close() method on Client in client/client.go: close state client if non-nil

**Checkpoint**: E2E test passes against LocalStack — seed bucket, sync, verify all expected tables and rows emitted.

---

## Phase 6: User Story 4 — Incremental Sync via Backend State (Priority: P2)

**Goal**: Subsequent syncs skip already-ingested objects by comparing `LastModified` against a stored cursor.

**Independent Test**: Run full sync, upload new file, run second sync, verify only new file's rows appear.

### Tests for User Story 4

- [X] T036 [P] [US4] Write unit tests for cursor key formatting in client/cursor_test.go: verify key format `s3/{bucket}/{table}/last_modified_cursor`
- [X] T037 [P] [US4] Write unit tests for cursor read/write in client/cursor_test.go: mock state client, test GetCursor (empty → zero-time), SetCursor, RFC3339Nano round-trip
- [X] T038 [P] [US4] Write unit tests for cursor-based object filtering in client/discover_test.go: objects with LastModified > cursor pass, objects with LastModified <= cursor are skipped
- [X] T039 [US4] Write E2E test for incremental sync in test/e2e_test.go: full sync → verify cursor stored, upload new file → re-sync → verify only new rows emitted, no-change re-sync → zero rows

### Implementation for User Story 4

- [X] T040 [US4] Implement cursor key formatting in client/cursor.go: `CursorKey(bucket, tableName) string`
- [X] T041 [US4] Implement GetCursor and SetCursor in client/cursor.go: read/write via state.Client, parse/format as RFC3339Nano
- [X] T042 [US4] Initialize state client in Sync function in client/sync.go: `state.NewConnectedClient(ctx, opts.BackendOptions)` (in Sync, not Configure, since BackendOptions is per-sync)
- [X] T043 [US4] Integrate cursor filtering into discovery in client/discover.go: after listing, filter objects where `LastModified > cursor` per table
- [X] T044 [US4] Update Sync() in client/sync.go: after processing each table, SetCursor with max LastModified; call Flush at end

**Checkpoint**: E2E incremental sync test passes — second sync with no changes produces zero inserts.

---

## Phase 7: User Story 5 — Batching and Concurrency Controls (Priority: P2)

**Goal**: `rows_per_record` controls Arrow record batch sizes; `concurrency` controls parallel S3 reads.

**Independent Test**: Vary rows_per_record and concurrency, verify measurable behavioral differences.

### Tests for User Story 5

- [X] T045 [P] [US5] Write unit tests for batch size control in client/parquet_test.go: 1000-row Parquet file with rows_per_record=100 → 10 batches, rows_per_record=500 → 2 batches
- [X] T046 [P] [US5] Write unit tests for concurrency semaphore in client/sync_test.go: verify max concurrent goroutines matches concurrency setting (1, 50, -1)

### Implementation for User Story 5

- [X] T047 [US5] Implement concurrency semaphore in client/sync.go: buffered channel of size `concurrency`; skip semaphore when concurrency < 0 (unlimited)
- [X] T048 [US5] Wire rows_per_record into Parquet reader in client/parquet.go: pass as ArrowReadProperties.BatchSize to pqarrow.NewFileReader
- [X] T049 [US5] Update Sync() to process objects concurrently in client/sync.go: per-table goroutines acquire semaphore, process object, release semaphore; errgroup for error propagation

**Checkpoint**: Unit tests verify batch counts change with rows_per_record; concurrency test verifies parallel limits.

---

## Phase 8: User Story 6 — Observability and Error Handling (Priority: P3)

**Goal**: Structured logs for discovery/sync progress; clear error messages; no credential leaks.

**Independent Test**: Trigger error conditions, inspect logs, verify no credentials in output.

### Tests for User Story 6

- [X] T050 [P] [US6] Write unit tests for error handling in client/sync_test.go: missing object between list and read → warning logged + continue, malformed Parquet → warning logged + continue
- [X] T051 [P] [US6] Write unit test for credential scrubbing in client/client_test.go: verify logger output does not contain AWS_SECRET_ACCESS_KEY or session tokens

### Implementation for User Story 6

- [X] T052 [US6] Add structured discovery logging in client/discover.go: log table count, table names, object counts per table
- [X] T053 [US6] Add structured per-file logging in client/sync.go: log object key, size, row count after processing each file
- [X] T054 [US6] Add error handling for missing objects in client/sync.go: catch NoSuchKey from GetObject, log warning with key, continue with remaining files (FR-021)
- [X] T055 [US6] Add error handling for malformed Parquet in client/parquet.go: catch parquet reader errors, log warning with key, continue with remaining files
- [X] T056 [US6] Add bucket-not-found and access-denied error wrapping in client/discover.go: wrap S3 errors with user-friendly messages suggesting credential/permission checks

**Checkpoint**: Error scenario tests pass; logs contain expected structured output; no credentials in output.

---

## Phase 9: Polish & Cross-Cutting Concerns

**Purpose**: Documentation, cleanup, consistency scan, CI validation

- [X] T057 [P] Write README.md with configuration example, table naming rules, incremental sync docs, building instructions per quickstart.md
- [X] T058 [P] Add GoDoc comments to all exported types and functions across client/, plugin/, internal/naming/
- [X] T059 Run consistency scan: verify all references, error messages, log strings, and examples are accurate (Constitution Principle V)
- [X] T060 Run `go mod tidy`, `go vet ./...`, `golangci-lint run` and fix all findings
- [X] T061 Validate quickstart.md scenario end-to-end against LocalStack
- [X] T062 Run full CI pipeline: `go test ./...` including E2E against LocalStack, confirm all green

**Checkpoint**: All tests pass, linter clean, README accurate, CI green.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies — start immediately
- **Phase 2 (Foundational)**: Depends on Phase 1 — BLOCKS all user stories
- **Phase 3 (US2 Config)**: Depends on Phase 2 — first user story because all others need validated Spec
- **Phase 4 (US3 Naming)**: Depends on Phase 2 — no dependency on US2 (pure logic, no Spec needed)
- **Phase 5 (US1 Sync)**: Depends on Phase 2, Phase 3 (Spec), Phase 4 (naming) — core MVP
- **Phase 6 (US4 Incremental)**: Depends on Phase 5 (sync infrastructure)
- **Phase 7 (US5 Batching)**: Depends on Phase 5 (sync infrastructure)
- **Phase 8 (US6 Observability)**: Depends on Phase 5 (sync infrastructure)
- **Phase 9 (Polish)**: Depends on all desired user stories being complete

### User Story Dependencies

- **US2 (Config)**: After Foundational — independent, no other story dependency
- **US3 (Naming)**: After Foundational — independent, no other story dependency
- **US1 (Sync)**: After US2 + US3 — depends on validated Spec and naming logic
- **US4 (Incremental)**: After US1 — extends sync with cursor filtering
- **US5 (Batching)**: After US1 — extends sync with concurrency/batching controls
- **US6 (Observability)**: After US1 — adds logging/error handling to existing sync path

### Within Each User Story

- Tests MUST be written and FAIL before implementation (Constitution Principle II — TDD)
- Models/types before services
- Services before integration points
- Core logic before cross-cutting concerns

### Parallel Opportunities

**Phase 1**: T003, T004, T005, T006, T007 can all run in parallel
**Phase 2**: T012, T013 can run in parallel (after T008–T011)
**Phase 3 (US2)**: T014, T015 test tasks run in parallel; then T016–T019 sequential
**Phase 4 (US3)**: T020 (test) then T021 (impl) — small phase
**Phase 3+4 together**: US2 and US3 can run in parallel (different files, no dependency)
**Phase 5 (US1)**: T022–T026 test tasks run in parallel; T027–T028 parallel; T029 separate
**Phase 6+7+8 together**: US4, US5, US6 can all start in parallel after US1 completes (different files)

---

## Parallel Example: User Story 1

```bash
# After Phase 2 + US2 + US3 complete:

# Launch all test tasks in parallel:
T022: "Unit tests for S3 object discovery in client/discover_test.go"
T023: "Unit tests for Parquet schema reading in client/parquet_test.go"
T024: "Unit tests for Parquet record streaming in client/parquet_test.go"
T025: "Unit tests for schema mismatch in client/discover_test.go"

# Then T026 (E2E test) depends on understanding the full flow

# Launch independent implementation tasks:
T027 + T028: discovery listing and grouping (both in client/discover.go but separate functions)
T029: Parquet schema reader (client/parquet.go — different file)

# Sequential implementation chain:
T030 → T031 → T032 → T033 → T034 → T035
```

---

## Parallel Example: Post-MVP User Stories

```bash
# After US1 (Phase 5) completes, launch all three P2/P3 stories in parallel:

# Developer A: US4 (Incremental Sync)
T036–T039 (tests) → T040–T044 (implementation)

# Developer B: US5 (Batching/Concurrency)
T045–T046 (tests) → T047–T049 (implementation)

# Developer C: US6 (Observability)
T050–T051 (tests) → T052–T056 (implementation)
```

---

## Implementation Strategy

### MVP First (User Stories 1 + 2 + 3)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (CRITICAL — blocks all stories)
3. Complete Phase 3: US2 (Config) + Phase 4: US3 (Naming) — can run in parallel
4. Complete Phase 5: US1 (Basic Full Sync)
5. **STOP and VALIDATE**: E2E sync against LocalStack works end-to-end
6. Deploy/demo — plugin can read Parquet from S3 and emit to any CQ destination

### Incremental Delivery

1. Setup + Foundational → Project builds, tests run
2. Add US2 (Config) + US3 (Naming) → Spec validation + table naming work
3. Add US1 (Sync) → **MVP! Full sync works** → Deploy/Demo
4. Add US4 (Incremental) → Production-ready for large buckets → Deploy/Demo
5. Add US5 (Batching) → Fine-tuned performance controls → Deploy/Demo
6. Add US6 (Observability) → Production-grade logging and errors → Deploy/Demo
7. Polish → README, consistency scan, CI green → Release

### Parallel Team Strategy

With multiple developers:

1. Team completes Setup + Foundational together
2. Once Foundational is done:
   - Developer A: US2 (Config) → then US1 (Sync) → then US4 (Incremental)
   - Developer B: US3 (Naming) → then US5 (Batching) → then US6 (Observability)
3. Polish phase is collaborative

---

## Notes

- [P] tasks = different files, no dependencies on in-progress tasks
- [USn] label maps task to specific user story for traceability
- Each user story is independently testable after completion
- TDD is mandatory (Constitution Principle II): write tests first, verify they FAIL, then implement
- E2E tests against LocalStack run on every CI push (Constitution Principle VI)
- Commit after each task or logical group
- Stop at any checkpoint to validate the story independently
