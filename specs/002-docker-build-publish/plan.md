# Implementation Plan: Docker Build & Publish

**Branch**: `002-docker-build-publish` | **Date**: 2026-02-21 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/002-docker-build-publish/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/plan-template.md` for the execution workflow.

## Summary

Containerise the cq-source-s3 CloudQuery source plugin as a multi-arch Docker image. The image uses a distroless base with a statically compiled Go binary as the entrypoint, listens on gRPC port 7777, and is published to GCR with semver+git-hash tags. A GitHub Actions workflow automates the build → E2E test → publish pipeline, triggered by semver git tags.

## Technical Context

**Language/Version**: Go 1.25 (latest stable per go.mod)
**Primary Dependencies**: CloudQuery Plugin SDK v4 (`github.com/cloudquery/plugin-sdk/v4`), AWS SDK for Go v2, Apache Arrow Go v18
**Storage**: N/A (this feature adds no new storage; plugin reads from S3 at runtime)
**Testing**: Go standard `testing` package; existing E2E suite in `test/` uses LocalStack service container
**Target Platform**: linux/amd64, linux/arm64 (container images); CI runs on GitHub Actions ubuntu-latest
**Project Type**: CloudQuery source plugin (gRPC server binary packaged as Docker image)
**Performance Goals**: Container image < 50 MB compressed; plugin ready to accept gRPC connections within 5 seconds of start; CI pipeline < 15 minutes with warm cache
**Constraints**: CGO_ENABLED=0 (static compilation); distroless base (no shell, no package manager); no native ARM hardware required for cross-compilation
**Scale/Scope**: 1 Dockerfile, 1 new GitHub Actions workflow, Makefile additions; no application code changes

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| # | Principle | Status | Notes |
|---|-----------|--------|-------|
| I | Latest Go Version | PASS | go.mod declares `go 1.25.5`; Dockerfile will use `golang:1.25` builder image |
| II | Test-Driven Development | PASS | No new application code—only infrastructure (Dockerfile, CI workflow). Existing tests cover plugin behavior. New CI workflow validates image builds and runs E2E tests before publish. |
| III | End-to-End S3 Testing | PASS | Existing E2E suite runs against LocalStack; CI workflow includes LocalStack service container and runs `go test ./test/...` before image publish |
| IV | Documentation Synchronization | PASS | README.md will be updated with container usage instructions; quickstart.md generated for this feature |
| V | Codebase Consistency Scanning | PASS | No renamed/removed entities; feature adds new files only. Consistency scan will verify Makefile help text and CI branch patterns are updated. |
| VI | E2E Tests Always Run | PASS | New release workflow runs E2E tests as a mandatory gate before publish; existing CI workflow continues to run E2E on every push/PR |

**Gate result: PASS** — No violations. Proceeding to Phase 0.

## Project Structure

### Documentation (this feature)

```text
specs/002-docker-build-publish/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
└── tasks.md             # Phase 2 output (/speckit.tasks command)
```

### Source Code (repository root)

```text
Dockerfile                           # Multi-stage build: Go builder → distroless runtime
.dockerignore                        # Exclude unnecessary files from build context
.github/workflows/release.yml        # New workflow: build, test, publish on semver tags
Makefile                             # Updated: docker-build, docker-push targets
README.md                            # Updated: container usage section
plugin/plugin.go                     # Unchanged (Version already set via ldflags)
main.go                              # Unchanged
test/                                # Unchanged (existing E2E tests reused by CI)
```

**Structure Decision**: Single project structure — this feature adds infrastructure files (Dockerfile, CI workflow) at the repository root. No new Go packages or source directories are needed since the plugin binary is already built from the existing codebase.

## Complexity Tracking

> No constitution violations — this section is intentionally empty.
