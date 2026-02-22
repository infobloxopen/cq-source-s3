# Tasks: Docker Build & Publish

**Input**: Design documents from `/specs/002-docker-build-publish/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: Included where applicable. This feature produces no new Go application code—only infrastructure files (Dockerfile, CI workflow, Makefile). Existing E2E tests are reused by the CI pipeline. Verification tasks validate the build artifacts. The release workflow also creates a GitHub Release with auto-generated release notes.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3, US4)
- Include exact file paths in descriptions

---

## Phase 1: Setup

**Purpose**: Project initialization — build context and Makefile infrastructure

- [X] T001 Create .dockerignore to exclude specs/, docs/, test/, .git/, .specify/, *.md (except README.md) from build context in .dockerignore
- [X] T002 [P] Update CI workflow branch pattern to include `002-*` branches in .github/workflows/ci.yml
- [X] T003 [P] Update Makefile .PHONY and add VERSION/GIT_HASH variables and docker-build target in Makefile

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core Dockerfile that all user stories depend on — single-arch first, multi-arch later

**CRITICAL**: No user story work can begin until the Dockerfile builds a working single-arch image

- [X] T004 Create multi-stage Dockerfile with golang:1.25-alpine builder and gcr.io/distroless/static-debian12:nonroot runtime in Dockerfile
- [X] T005 Verify static binary builds locally with `make docker-build` and container starts on port 7777

**Checkpoint**: `docker run -p 7777:7777 cq-source-s3:local` starts the plugin and listens on gRPC port 7777

---

## Phase 3: User Story 1 — Pull and Run Container Image (Priority: P1) MVP

**Goal**: An operator can pull and run the container image; plugin starts on port 7777 with no extra configuration

**Independent Test**: `docker run -p 7777:7777 <image>` starts, process listens on 7777, exits cleanly on SIGTERM

### Implementation for User Story 1

- [X] T006 [US1] Validate Dockerfile ENTRYPOINT is `/cq-source-s3` and CMD is `["serve", "--address", "[::]:7777"]` in Dockerfile
- [X] T007 [US1] Validate EXPOSE 7777 is declared in Dockerfile
- [X] T008 [US1] Verify container runs as nonroot user (UID 65534) by inspecting running container
- [X] T009 [US1] Verify plugin responds to gRPC connection on port 7777 by running container and checking TCP port
- [X] T010 [US1] Verify custom listen address override works: `docker run <image> serve --address [::]:8080`
- [X] T011 [US1] Verify image size is under 50 MB compressed using `docker image ls`

**Checkpoint**: Container image is runnable, listens on 7777, configurable via CLI args, under 50 MB

---

## Phase 4: User Story 2 — Multi-Architecture Deployment (Priority: P2)

**Goal**: The same image tag works on both amd64 and arm64 hosts via an OCI manifest list

**Independent Test**: `docker manifest inspect <image>:<tag>` shows linux/amd64 and linux/arm64 platforms

### Implementation for User Story 2

- [X] T012 [US2] Add `FROM --platform=$BUILDPLATFORM` and TARGETOS/TARGETARCH args to builder stage in Dockerfile
- [X] T013 [US2] Add `--mount=type=cache` for /go/pkg/mod and /root/.cache/go-build in Dockerfile
- [X] T014 [US2] Add docker-build-multiarch target to Makefile using `docker buildx build --platform linux/amd64,linux/arm64`
- [X] T015 [US2] Verify multi-arch build succeeds locally with `make docker-build-multiarch` (creates manifest for both archs)

**Checkpoint**: `docker buildx build --platform linux/amd64,linux/arm64` succeeds and produces a manifest list

---

## Phase 5: User Story 3 — Automated CI Build, Test & Publish (Priority: P3)

**Goal**: Pushing a semver tag triggers CI that runs lint + tests + builds multi-arch image + publishes to GCR and GHCR

**Independent Test**: Push a test tag, verify the workflow runs E2E tests and builds the image (publish can be validated by dry-run or actual push)

### Implementation for User Story 3

- [X] T016 [US3] Create release workflow with semver tag trigger in .github/workflows/release.yml
- [X] T017 [US3] Add lint job (golangci-lint) to release workflow in .github/workflows/release.yml
- [X] T018 [US3] Add test job with LocalStack service container, unit tests, and E2E tests to release workflow in .github/workflows/release.yml
- [X] T019 [US3] Add QEMU and Buildx setup steps to build-push job in .github/workflows/release.yml
- [X] T020 [US3] Add GCR login step using docker/login-action with _json_key in .github/workflows/release.yml
- [X] T021 [US3] Add GHCR login step using docker/login-action with GITHUB_TOKEN in .github/workflows/release.yml
- [X] T022 [US3] Add docker/metadata-action step with GCR and GHCR image names in .github/workflows/release.yml
- [X] T023 [US3] Add docker/build-push-action step with multi-arch platforms, tags, labels, and gha cache in .github/workflows/release.yml
- [X] T024 [US3] Add `needs: [lint, test]` dependency so build-push only runs after tests pass in .github/workflows/release.yml
- [X] T025 [US3] Set workflow permissions (contents: write, packages: write, id-token: write) in .github/workflows/release.yml
- [X] T026 [US3] Pass VERSION build-arg from semver tag to Dockerfile via build-push-action in .github/workflows/release.yml
- [X] T027 [US3] Add release job that creates a GitHub Release via softprops/action-gh-release@v2 with `generate_release_notes: true` in .github/workflows/release.yml
- [X] T028 [US3] Append image digest and pull commands (GCR + GHCR) to the GitHub Release body in .github/workflows/release.yml
- [X] T029 [US3] Add `needs: [build-push]` dependency to release job so it only runs after successful publish in .github/workflows/release.yml

**Checkpoint**: Complete release.yml workflow — lint → test → build-push → release pipeline with all gates

---

## Phase 6: User Story 4 — Identify Exact Source of a Running Image (Priority: P4)

**Goal**: Every published image tag contains a git hash suffix enabling exact commit traceability

**Independent Test**: Inspect a published tag and verify the short SHA maps to a valid commit

### Implementation for User Story 4

- [X] T030 [US4] Configure docker/metadata-action tag template `type=raw,value=v{{version}}-{{sha}}` for primary tag in .github/workflows/release.yml
- [X] T031 [US4] Add semver tag (`type=semver,pattern=v{{version}}`), minor tag (`type=semver,pattern=v{{major}}.{{minor}}`), and latest tag in .github/workflows/release.yml
- [X] T032 [US4] Verify ldflags inject correct version into plugin.Version via VERSION build-arg in Dockerfile

**Checkpoint**: Metadata action produces tags `v1.2.3-abc1234`, `v1.2.3`, `v1.2`, `latest` from a semver tag push

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Documentation, consistency, and final validation

- [X] T033 [P] Add container usage section (pull, run, configure, multi-arch) to README.md
- [X] T034 [P] Add docker-push target to Makefile with GCR and GHCR image names
- [X] T035 Update Makefile help text and .PHONY list for all new targets in Makefile
- [X] T036 Run `go mod tidy` and verify no module changes
- [X] T037 Run codebase consistency scan: verify all Makefile targets have help comments, CI branch patterns include `002-*`, and no stale references
- [X] T038 Validate quickstart.md commands work end-to-end against local Docker build using specs/002-docker-build-publish/quickstart.md

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately
- **Foundational (Phase 2)**: Depends on T001 (.dockerignore) — BLOCKS all user stories
- **US1 (Phase 3)**: Depends on Phase 2 (working Dockerfile) — validates single-arch image
- **US2 (Phase 4)**: Depends on Phase 2 — adds multi-arch to existing Dockerfile
- **US3 (Phase 5)**: Depends on Phase 2 — can start in parallel with US1/US2 (different file: release.yml)
- **US4 (Phase 6)**: Depends on Phase 5 (release.yml exists) — configures tag templates
- **Polish (Phase 7)**: Depends on all user stories being complete

### User Story Dependencies

- **US1 (P1)**: Depends on Phase 2 only. Independent of US2-US4.
- **US2 (P2)**: Depends on Phase 2 only. Independent of US1, US3, US4. Modifies same Dockerfile as US1 but different sections.
- **US3 (P3)**: Depends on Phase 2 only. Creates new file (release.yml). Independent of US1, US2.
- **US4 (P4)**: Depends on US3 (release.yml must exist). Adds tag configuration to release.yml.

### Within Each User Story

- Dockerfile changes (US1, US2) are in the same file — execute sequentially
- Release workflow tasks (US3, US4) build on each other — execute sequentially
- US1/US2 (Dockerfile) and US3 (release.yml) are different files — can execute in parallel

### Parallel Opportunities

- T001, T002, T003 can all run in parallel (Setup phase)
- T033, T034 can run in parallel (Polish phase, different files)
- US1+US2 (Dockerfile) can run in parallel with US3 (release.yml) since they modify different files
- T006-T011 (US1 validation) are sequential since they verify the same image

---

## Parallel Example: After Foundational Phase

```bash
# Stream A (Dockerfile): US1 → US2
T006 → T007 → T008 → T009 → T010 → T011  # US1: validate image
T012 → T013 → T014 → T015                  # US2: add multi-arch

# Stream B (release.yml): US3 → US4
T016 → T017 → T018 → T019 → T020 → T021 → T022 → T023 → T024 → T025 → T026 → T027 → T028 → T029  # US3: full pipeline + release
T030 → T031 → T032                          # US4: tag templates
```

Streams A and B can run concurrently (different files).

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001-T003)
2. Complete Phase 2: Foundational (T004-T005)
3. Complete Phase 3: User Story 1 (T006-T011)
4. **STOP and VALIDATE**: `docker run -p 7777:7777 cq-source-s3:local` works
5. Single-arch, locally-built image is usable — this is the MVP

### Incremental Delivery

1. Setup + Foundational → Dockerfile builds ✓
2. Add US1 → Image runs on 7777 ✓ (MVP!)
3. Add US2 → Multi-arch build works ✓
4. Add US3 → CI pipeline automates everything + creates GitHub Release ✓
5. Add US4 → Tags include git hash for traceability ✓
6. Polish → Documentation, consistency ✓

Each story adds value without breaking previous stories.

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- No new Go application code — all tasks produce infrastructure files
- Existing E2E tests (`test/e2e_test.go`) are reused by the release workflow
- `plugin.Version` is already declared in `plugin/plugin.go` and set via ldflags — no code changes needed
- Commit after each phase checkpoint for clean rollback points
