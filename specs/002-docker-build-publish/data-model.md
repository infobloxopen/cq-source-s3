# Data Model: Docker Build & Publish

**Branch**: `002-docker-build-publish` | **Date**: 2026-02-21 | **Spec**: [spec.md](spec.md)

---

## Overview

This feature introduces no new runtime data entities or persistent storage. The "data model" consists of build-time artifacts and CI configuration that govern how the container image is produced, tagged, and published.

## Entities

### 1. Container Image

| Attribute | Description | Source |
|-----------|-------------|--------|
| Base Image | `gcr.io/distroless/static-debian12:nonroot` | Dockerfile `FROM` |
| Binary | `/cq-source-s3` (static Go binary, `CGO_ENABLED=0`) | Dockerfile `COPY --from=builder` |
| Entrypoint | `["/cq-source-s3"]` | Dockerfile `ENTRYPOINT` |
| Default command | `["serve", "--address", "[::]:7777"]` | Dockerfile `CMD` |
| Exposed port | 7777 (gRPC) | Dockerfile `EXPOSE` |
| User | `nonroot` (UID 65534) | Base image `:nonroot` tag |
| Platforms | linux/amd64, linux/arm64 | Buildx `--platform` |

**Validation Rules**:
- Binary must be statically linked (verified by building with `CGO_ENABLED=0`)
- Image must contain no shell or package manager (enforced by distroless base)
- Image compressed size must be < 50 MB

### 2. Image Tag

| Attribute | Format | Example |
|-----------|--------|---------|
| Primary tag | `v<MAJOR>.<MINOR>.<PATCH>-<SHORT_SHA>` | `v1.2.3-abc1234` |
| Semver tag | `v<MAJOR>.<MINOR>.<PATCH>` | `v1.2.3` |
| Minor tag | `v<MAJOR>.<MINOR>` | `v1.2` |
| Latest tag | `latest` | `latest` |

**Derivation**:
- `<MAJOR>.<MINOR>.<PATCH>` extracted from git tag `refs/tags/v*.*.*`
- `<SHORT_SHA>` is the 7-character git commit hash (`git rev-parse --short HEAD`)

**Validation Rules**:
- Git tag must match `v[0-9]+.[0-9]+.[0-9]+` (strict semver with `v` prefix)
- Short SHA must correspond to a valid commit in the repository
- Primary tag uniquely identifies both version and exact source commit

### 3. CI Pipeline (Release Workflow)

| Attribute | Value |
|-----------|-------|
| Trigger | Push of tag matching `v*.*.*` |
| Runner | `ubuntu-latest` |
| Services | LocalStack (S3-compatible, port 4566) |
| Stages | lint → test (unit + E2E) → build & push |
| Gate | Image publish blocked if any test fails |
| Registries | GCR (`gcr.io/<PROJECT>/cq-source-s3`) + GHCR (`ghcr.io/infobloxopen/cq-source-s3`) |

**State Transitions**:

```
tag pushed → lint → test (unit + E2E) → build image → push to registries
                                 ↓ (failure)
                          pipeline fails, no publish
```

### 4. Build Configuration

| Attribute | Value |
|-----------|-------|
| Go version | Read from `go.mod` (`go 1.25.x`) |
| CGO | Disabled (`CGO_ENABLED=0`) |
| ldflags | `-s -w -X github.com/infobloxopen/cq-source-s3/plugin.Version=${VERSION}` |
| Build flags | `-trimpath` |
| Buildx cache | `type=gha,mode=max` |
| Target architectures | linux/amd64, linux/arm64 |

## Relationships

```
┌────────────┐     builds     ┌──────────────────┐
│  Release    │───────────────▶│  Container Image │
│  Workflow   │                │  (multi-arch)    │
│  (CI)       │                └──────┬───────────┘
└──────┬──────┘                       │
       │                              │ tagged with
       │ triggered by                 ▼
       │                       ┌──────────────────┐
       ▼                       │  Image Tags      │
┌──────────────┐               │  (semver+hash)   │
│  Git Tag     │               └──────────────────┘
│  (v1.2.3)    │                       │
└──────────────┘                       │ published to
                                       ▼
                               ┌──────────────────┐
                               │  GCR + GHCR      │
                               │  (registries)    │
                               └──────────────────┘
```

## Files Produced by This Feature

| File | Purpose |
|------|---------|
| `Dockerfile` | Multi-stage build definition |
| `.dockerignore` | Exclude non-essential files from build context |
| `.github/workflows/release.yml` | CI pipeline for build, test, publish |
| `Makefile` updates | `docker-build` and `docker-push` targets |
| `README.md` updates | Container usage documentation |
