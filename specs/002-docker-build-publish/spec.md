# Feature Specification: Docker Build & Publish

**Feature Branch**: `002-docker-build-publish`  
**Created**: 2026-02-21  
**Status**: Draft  
**Input**: User description: "build and publish a docker image — the docker image should have the plugin as the entrypoint and built with distroless with a statically compiled binary — the container should listen on grpc port 7777 and be configurable like other cq-source plugins — the container should be built with buildx and use cache, building on mac arm and linux x86 and arm to produce a multi-arch OCI image — the image should be published to gcr and registered as a package — the image should be semver like other cq source plugins but have a suffix of the git hash — the github action should run end to end testing"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Pull and Run Container Image (Priority: P1)

An operator pulls the published container image from Google Container Registry and runs the cq-source-s3 plugin as a containerised gRPC service. The container starts, listens on port 7777, and is ready to accept CloudQuery sync requests—without the operator needing to compile Go or manage any build tooling.

**Why this priority**: This is the core value proposition—if users cannot pull and run the image, nothing else matters.

**Independent Test**: Pull the published image on a clean machine (`docker run`), verify the process starts, binds to port 7777, and responds to a gRPC health-check or plugin handshake.

**Acceptance Scenarios**:

1. **Given** the image is published to GCR, **When** an operator runs `docker run <image>:<tag>`, **Then** the plugin starts and listens on gRPC port 7777 with no additional configuration.
2. **Given** the container is running, **When** a CloudQuery CLI connects to port 7777, **Then** the plugin responds with its supported tables and accepts a sync request.
3. **Given** the operator specifies custom plugin configuration via environment variables or CLI flags, **When** the container starts, **Then** the plugin respects those overrides (e.g., setting log level, listen address).

---

### User Story 2 - Multi-Architecture Deployment (Priority: P2)

A platform team runs workloads across mixed-architecture nodes (linux/amd64, linux/arm64). They pull the same image tag and the container runtime automatically selects the correct architecture variant, allowing consistent deployment across clusters that include both Intel and ARM hosts.

**Why this priority**: Multi-arch support prevents "wrong architecture" failures and is essential for teams running heterogeneous infrastructure or Apple Silicon development machines.

**Independent Test**: Pull the image on both an amd64 and arm64 host (or use `docker manifest inspect` to verify the manifest list), run the container on each, and confirm the plugin starts correctly.

**Acceptance Scenarios**:

1. **Given** the image is published as a multi-arch OCI image, **When** a user pulls the image on a linux/amd64 host, **Then** the amd64 variant is selected and runs correctly.
2. **Given** the image is published as a multi-arch OCI image, **When** a user pulls the image on a linux/arm64 host, **Then** the arm64 variant is selected and runs correctly.
3. **Given** a user runs `docker manifest inspect <image>:<tag>`, **Then** the manifest lists at least linux/amd64 and linux/arm64 platforms.

---

### User Story 3 - Automated CI Build, Test & Publish (Priority: P3)

A developer pushes a semver tag to the repository. The CI pipeline automatically builds the multi-arch image, runs end-to-end tests against LocalStack, and—only when tests pass—publishes the image to GCR with a version tag that includes both the semver and the git short hash.

**Why this priority**: Automation ensures every published image is tested and correctly versioned, reducing human error and enabling reproducible deployments.

**Independent Test**: Push a test tag, observe the CI workflow, and verify it runs E2E tests, builds the image, and publishes with the correct tag format (e.g., `v1.2.3-abc1234`).

**Acceptance Scenarios**:

1. **Given** a developer pushes a semver tag (e.g., `v1.2.3`), **When** the CI pipeline triggers, **Then** the pipeline builds the multi-arch image, runs E2E tests, and publishes the image only if tests pass.
2. **Given** the image is published, **When** a user inspects the image tag, **Then** the tag follows the format `v<MAJOR>.<MINOR>.<PATCH>-<SHORT_GIT_HASH>` (e.g., `v1.2.3-abc1234`).
3. **Given** E2E tests fail during the CI run, **When** the pipeline evaluates the result, **Then** the image is NOT published and the pipeline reports failure.

---

### User Story 4 - Identify Exact Source of a Running Image (Priority: P4)

An on-call engineer investigating an incident needs to determine exactly which commit produced the container image currently in production. By inspecting the image tag, they can immediately map it back to a specific git commit.

**Why this priority**: Traceability from running image to source code is critical for incident response and auditing.

**Independent Test**: Given a running container, read the image tag and use the git hash suffix to locate the exact commit in the repository.

**Acceptance Scenarios**:

1. **Given** a running container with tag `v1.2.3-abc1234`, **When** an engineer runs `git log abc1234`, **Then** the commit exists in the repository and matches the build.
2. **Given** the published image in GCR, **When** a user lists image tags, **Then** every tag contains a git hash suffix that corresponds to a valid commit.

---

### Edge Cases

- What happens when the CI pipeline is triggered but Docker Buildx cache is cold (first build)? The build should still succeed, just take longer.
- What happens when a semver tag is re-pushed (force-pushed)? The pipeline should rebuild and publish the new image, overwriting the previous tag if the git hash changes.
- What happens when network access to GCR is unavailable during publish? The pipeline should fail with a clear error message and not leave partial/untagged images.
- What happens when the build is triggered on a non-semver branch (e.g., `main` or a feature branch)? The image should NOT be published to GCR; only semver tags trigger a publish.
- What happens when the binary fails to compile for one architecture? The entire multi-arch build should fail and no image should be published.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The container image MUST use a distroless base image to minimise attack surface and image size.
- **FR-002**: The plugin binary MUST be statically compiled (no dynamically linked C libraries) so it runs in the distroless container without external dependencies.
- **FR-003**: The container MUST set the plugin binary as its entrypoint so the image is directly runnable without specifying a command.
- **FR-004**: The container MUST expose gRPC port 7777 and the plugin MUST listen on that port by default.
- **FR-005**: The plugin MUST accept the same configuration mechanisms as other CloudQuery source plugins (environment variables, CLI flags, spec file passed via gRPC).
- **FR-006**: The build process MUST produce a multi-arch OCI image supporting at minimum linux/amd64 and linux/arm64.
- **FR-007**: The build process MUST use Docker Buildx with layer caching enabled to reduce rebuild times.
- **FR-008**: The image MUST be published to Google Container Registry (GCR) and registered as a GitHub package.
- **FR-009**: Image tags MUST follow semver with a git hash suffix in the format `v<MAJOR>.<MINOR>.<PATCH>-<SHORT_GIT_HASH>` (e.g., `v1.2.3-abc1234`).
- **FR-010**: The CI pipeline MUST run the existing end-to-end test suite (against LocalStack) before publishing the image.
- **FR-011**: The CI pipeline MUST NOT publish the image if any tests (unit or E2E) fail.
- **FR-012**: The CI pipeline MUST be triggered by semver git tags (e.g., `v*.*.*`).
- **FR-013**: The build MUST cross-compile from the CI runner (linux/amd64) for all target architectures without requiring native ARM hardware.
- **FR-014**: The CI pipeline MUST create a GitHub Release for the semver tag, including the image digest, image pull commands, and auto-generated release notes.

### Key Entities

- **Container Image**: The OCI-compliant Docker image containing the statically compiled plugin binary on a distroless base. Tagged with semver+hash. Published to GCR.
- **Image Tag**: A version identifier combining semantic version and git short hash, providing both human-readable version ordering and exact commit traceability.
- **CI Pipeline**: The GitHub Actions workflow that orchestrates build, test, and publish. Triggered by semver tags. Gates publishing on test success.
- **Multi-Arch Manifest**: An OCI image index listing platform-specific image variants (linux/amd64, linux/arm64), allowing container runtimes to auto-select the correct architecture.
- **GitHub Release**: A tagged release on the GitHub Releases page containing image coordinates, digest, pull commands, and auto-generated changelog from commits since the previous tag.

## Assumptions

- The existing `serve.Plugin()` call in `main.go` already supports listening on a configurable gRPC port (CloudQuery plugin SDK default is 7777). No application code changes are required for port configuration.
- GCR authentication will be handled via a GCP service account key or Workload Identity Federation, stored as a GitHub Actions secret (`GCR_JSON_KEY` or similar).
- The GitHub repository has permission to create GitHub packages for container images.
- The Go toolchain supports cross-compilation for linux/amd64 and linux/arm64 via `GOOS`/`GOARCH` environment variables with `CGO_ENABLED=0`.
- The semver tags are created manually by maintainers (not auto-generated), and follow the format `v<MAJOR>.<MINOR>.<PATCH>`.
- Docker Buildx is available on the GitHub Actions runner (Ubuntu latest includes it by default).
- E2E tests use LocalStack (already configured in the existing CI workflow) and do not require real AWS credentials.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A user can pull and start the container image in under 30 seconds on a standard connection, and the plugin is ready to accept gRPC connections on port 7777 within 5 seconds of container start.
- **SC-002**: The published image runs correctly on both amd64 and arm64 hosts without any per-architecture configuration.
- **SC-003**: Every published image tag contains a git hash suffix that maps to a valid, existing commit in the repository.
- **SC-004**: The complete CI pipeline (build + E2E test + publish) completes in under 15 minutes for a typical run with warm cache.
- **SC-005**: 100% of published images have passed the full E2E test suite—no untested image is ever pushed to the registry.
- **SC-006**: The final container image size is under 50 MB (compressed) due to distroless base and static binary.
- **SC-007**: Developers can reproduce the container build locally using the same build commands referenced in the CI pipeline.
- **SC-008**: Every semver tag push creates a GitHub Release page with image coordinates, digest, and auto-generated release notes.
