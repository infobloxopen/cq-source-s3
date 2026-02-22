# Research: Docker Build & Publish

**Branch**: `002-docker-build-publish` | **Date**: 2026-02-21 | **Spec**: [spec.md](spec.md) | **Plan**: [plan.md](plan.md)

---

## Topic 1: Distroless Base Image Selection

### Decision

Use `gcr.io/distroless/static-debian12:nonroot` as the runtime base image.

### Rationale

| Image | Contents | Size (~) | Multi-Arch | Use Case |
|---|---|---|---|---|
| `gcr.io/distroless/static-debian12` | CA certs, `/etc/passwd`, tzdata, static glibc | ~2 MiB | amd64, arm64, arm, s390x, ppc64le | Statically compiled Go binaries (`CGO_ENABLED=0`) |
| `gcr.io/distroless/base-debian12` | Everything in `static` **+ glibc** | ~20 MiB | amd64, arm64, arm, s390x, ppc64le | Binaries that dynamically link glibc (e.g., CGO-enabled Go, Rust) |
| `gcr.io/distroless/cc-debian12` | Everything in `base` **+ libstdc++** | ~25 MiB | amd64, arm64, arm, s390x, ppc64le | C++ apps, CGO with C++ deps |

**Why `static-debian12`?**
- Our Go binary is compiled with `CGO_ENABLED=0`, producing a fully static binary with no glibc dependency.
- `static-debian12` is the **smallest distroless image** (~2 MiB, ~50% of Alpine).
- It includes CA certificates (needed for TLS to AWS S3) and timezone data.
- It does NOT include a shell, package manager, or libc — minimal attack surface.

**Why `:nonroot` tag?**
- The `:nonroot` tag sets the container user to UID 65534 (`nonroot`), group GID 65534.
- The default `:latest` tag runs as root (UID 0), which is a security anti-pattern.
- Using `:nonroot` satisfies Pod Security Standards (restricted profile) and is a best practice for production containers.
- The `nonroot` user is defined in `/etc/passwd` within the image.

**Tag convention**: Always use the explicit Debian version suffix (`-debian12`) to prevent breaking builds when distroless defaults change to a newer Debian release (per official recommendation).

### Alternatives Considered

| Alternative | Why Rejected |
|---|---|
| `gcr.io/distroless/static:nonroot` (no Debian suffix) | Currently aliases `static-debian12:nonroot`, but will change when Debian 13 becomes default. Explicit suffix is safer. |
| `gcr.io/distroless/base-debian12` | Adds ~18 MiB of glibc we don't need. Only required if CGO is enabled. |
| `alpine:latest` | ~5 MiB (larger than static-debian12). Includes shell and package manager — larger attack surface. |
| `scratch` | Zero bytes but lacks CA certificates and `/etc/passwd`. Would require manually copying certs into the image. |
| `gcr.io/distroless/static-debian13` | Available but Debian 13 (trixie) is newer; Debian 12 (bookworm) is the current stable. Prefer proven stability. |

---

## Topic 2: Multi-Arch Docker Buildx Best Practices

### Decision

Use a multi-stage Dockerfile with `BUILDPLATFORM`/`TARGETARCH` build args, combined with `docker/build-push-action@v6` and `docker/setup-qemu-action@v3` in GitHub Actions.

### Rationale

**Dockerfile Pattern:**

```dockerfile
# syntax=docker/dockerfile:1

# ── Stage 1: Build ──────────────────────────────────────────
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

# TARGETOS and TARGETARCH are automatically set by BuildKit
ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

# Cache dependency downloads
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Build static binary for the target platform
COPY . .
ARG VERSION=development
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w -X github.com/infobloxopen/cq-source-s3/plugin.Version=${VERSION}" \
    -o /out/cq-source-s3 .

# ── Stage 2: Runtime ────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /out/cq-source-s3 /cq-source-s3

EXPOSE 7777

ENTRYPOINT ["/cq-source-s3"]
CMD ["serve", "--address", "[::]:7777"]
```

**Build Args Explained:**

| Arg | Set By | Value Example | Purpose |
|---|---|---|---|
| `BUILDPLATFORM` | BuildKit (auto) | `linux/amd64` | Platform of the CI runner — build stage runs natively here |
| `TARGETPLATFORM` | BuildKit (auto) | `linux/arm64` | Full target platform string |
| `TARGETOS` | BuildKit (auto) | `linux` | OS component of the target |
| `TARGETARCH` | BuildKit (auto) | `arm64` | Architecture component — passed to `GOARCH` |

**Key pattern:** `FROM --platform=$BUILDPLATFORM` ensures the build stage runs on the CI runner's native architecture (amd64). Go cross-compiles to the `TARGETARCH` natively via `GOOS`/`GOARCH`. This is **dramatically faster** than emulating the entire build under QEMU.

QEMU (via `setup-qemu-action`) is only needed for the final stage when BuildKit needs to create the platform-specific image layer (copying the binary), which is trivial.

**`--mount=type=cache` for Go:**
- `--mount=type=cache,target=/go/pkg/mod` — caches downloaded modules across builds
- `--mount=type=cache,target=/root/.cache/go-build` — caches compiled packages across builds
- These cache mounts are internal to BuildKit and persist across builds on the same builder instance. In GitHub Actions, they are ephemeral per-job unless combined with `type=gha` cache export (see Topic 7).

**CLI equivalent (for local development):**

```bash
# Create a multi-arch builder (one-time setup)
docker buildx create --name multiarch --driver docker-container --use

# Build and push multi-arch image
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --build-arg VERSION=v1.0.0-abc1234 \
  --tag gcr.io/PROJECT_ID/cq-source-s3:v1.0.0-abc1234 \
  --tag gcr.io/PROJECT_ID/cq-source-s3:latest \
  --push \
  .
```

### Alternatives Considered

| Alternative | Why Rejected |
|---|---|
| Single-arch build + `docker manifest create` | More complex, requires separate builds and manifest assembly. Buildx handles it natively. |
| Build inside QEMU for each arch | 10-30x slower than Go cross-compilation. `FROM --platform=$BUILDPLATFORM` avoids this. |
| `golang:1.25` (Debian-based builder) | Works but `golang:1.25-alpine` is smaller and downloads faster in CI. Builder image size doesn't affect final image. |
| `--mount=type=cache` with `id=` per arch | Useful to prevent cache conflicts between architectures. Worth adding if builds collide: `--mount=type=cache,target=/root/.cache/go-build,id=go-build-${TARGETARCH}` |

---

## Topic 3: GCR Authentication from GitHub Actions

### Decision

Support **two authentication methods** in order of preference:

1. **Workload Identity Federation** (preferred, no long-lived secrets)
2. **Service Account Key JSON** (fallback, simpler initial setup)

### Rationale

**Approach 1: Workload Identity Federation (Recommended)**

```yaml
jobs:
  release:
    permissions:
      contents: read
      id-token: write       # Required for OIDC token
      packages: write       # Required for GitHub Packages

    steps:
      - uses: actions/checkout@v4

      - uses: google-github-actions/auth@v3
        with:
          project_id: 'my-project'
          workload_identity_provider: 'projects/123456789/locations/global/workloadIdentityPools/my-pool/providers/my-provider'

      - uses: docker/login-action@v3
        with:
          registry: gcr.io
          username: oauth2accesstoken
          password: ${{ steps.auth.outputs.auth_token }}
```

**Approach 2: Service Account Key JSON (Simpler)**

```yaml
      - uses: docker/login-action@v3
        with:
          registry: gcr.io
          username: _json_key
          password: ${{ secrets.GCR_JSON_KEY }}
```

**Secrets Required:**

| Method | Secret Name | Value |
|---|---|---|
| Workload Identity Fed. | (none — uses OIDC) | Configure `workload_identity_provider` as a variable |
| Service Account Key | `GCR_JSON_KEY` | Minified JSON of a GCP service account key with `roles/storage.admin` on the GCR bucket |

**GCR Image Naming Convention:**

```
gcr.io/<PROJECT_ID>/<IMAGE_NAME>:<TAG>
gcr.io/my-gcp-project/cq-source-s3:v1.2.3-abc1234
```

Regional variants: `us.gcr.io/`, `eu.gcr.io/`, `asia.gcr.io/` — use `gcr.io/` for multi-region (US-hosted).

**Why `google-github-actions/auth@v3`?**
- v3 is the latest release (Sep 2025), uses Node 24.
- Workload Identity Federation eliminates long-lived credentials — no key rotation needed.
- The action exports `GOOGLE_APPLICATION_CREDENTIALS` and `CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE` for downstream tools.

### Alternatives Considered

| Alternative | Why Rejected |
|---|---|
| `gcloud auth configure-docker` | Requires installing gcloud CLI. `docker/login-action` is lighter. |
| Direct `docker login` with `_json_key` | Works but doesn't integrate with WIF's automatic token exchange. |
| Artifact Registry (`pkg.dev`) instead of GCR | GCR is the spec requirement. Note: GCR infrastructure has been migrated to Artifact Registry under the hood, so `gcr.io` is still supported and benefits from AR's infrastructure. |

---

## Topic 4: Semver Tag Extraction in GitHub Actions

### Decision

Use `docker/metadata-action@v5` with `type=semver` and `type=sha` tag types, plus a `type=raw` tag combining semver+hash per the spec requirement.

### Rationale

**Tag Format per Spec:** `v<MAJOR>.<MINOR>.<PATCH>-<SHORT_GIT_HASH>` (e.g., `v1.2.3-abc1234`)

**Recommended Configuration:**

```yaml
- name: Docker meta
  id: meta
  uses: docker/metadata-action@v5
  with:
    images: |
      gcr.io/${{ vars.GCP_PROJECT_ID }}/cq-source-s3
    tags: |
      # v1.2.3-abc1234 (primary tag per spec)
      type=raw,value=v{{version}}-{{sha}},enable=${{ startsWith(github.ref, 'refs/tags/v') }}
      # v1.2.3 (standard semver)
      type=semver,pattern=v{{version}}
      # v1.2 (minor tracking)
      type=semver,pattern=v{{major}}.{{minor}}
      # latest
      type=raw,value=latest,enable=${{ startsWith(github.ref, 'refs/tags/v') }}
```

**Manual Extraction (if not using metadata-action):**

```yaml
- name: Extract version info
  id: version
  run: |
    # Extract semver from tag ref: refs/tags/v1.2.3 → v1.2.3
    VERSION=${GITHUB_REF#refs/tags/}
    echo "version=${VERSION}" >> $GITHUB_OUTPUT

    # Short git hash (7 chars)
    SHORT_SHA=$(git rev-parse --short HEAD)
    echo "short_sha=${SHORT_SHA}" >> $GITHUB_OUTPUT

    # Combined tag: v1.2.3-abc1234
    echo "tag=${VERSION}-${SHORT_SHA}" >> $GITHUB_OUTPUT
```

**`docker/metadata-action` Template Expressions:**
- `{{version}}` — semver without `v` prefix: `1.2.3`
- `{{raw}}` — full tag as-is: `v1.2.3`
- `{{sha}}` — short commit SHA (7 chars): `90dd603`
- `{{major}}`, `{{minor}}`, `{{patch}}` — individual components

**`latest` Tag Behavior:**
- `docker/metadata-action` auto-generates `latest` for `type=semver` and `type=ref,event=tag` by default (`flavor: latest=auto`).
- We use an explicit `type=raw,value=latest` with `enable=` guard for clarity and control.

**Resulting Tags for `refs/tags/v1.2.3` with commit `abc1234`:**

| Tag | Purpose |
|---|---|
| `gcr.io/PROJECT/cq-source-s3:v1.2.3-abc1234` | Primary (spec requirement) — exact commit traceability |
| `gcr.io/PROJECT/cq-source-s3:v1.2.3` | Standard semver — version ordering |
| `gcr.io/PROJECT/cq-source-s3:v1.2` | Minor tracking — auto-updated on patch releases |
| `gcr.io/PROJECT/cq-source-s3:latest` | Convenience — always points to newest release |

### Alternatives Considered

| Alternative | Why Rejected |
|---|---|
| Manual bash extraction only | Works but less maintainable. `metadata-action` handles edge cases (sanitization, pre-release, etc.) |
| `type=sha` tag only | Produces `sha-abc1234` — doesn't include semver, doesn't match spec format |
| GitHub context `${{ github.sha }}` | Returns full 40-char SHA. Need `git rev-parse --short HEAD` or `{{sha}}` template for 7-char version. |

---

## Topic 5: GitHub Packages Registration

### Decision

**Dual-publish** to both GCR (`gcr.io`) and GHCR (`ghcr.io`). The spec says "published to GCR and registered as a package" — the most practical interpretation is pushing to both registries.

### Rationale

**GitHub Container Registry (GHCR)** is the native "GitHub Packages" for container images. There is no mechanism to "register" an image from an external registry (GCR) as a GitHub Package. The standard approach is to **push the same image to both registries**.

**Implementation:**

```yaml
# metadata-action lists both registries
- name: Docker meta
  id: meta
  uses: docker/metadata-action@v5
  with:
    images: |
      gcr.io/${{ vars.GCP_PROJECT_ID }}/cq-source-s3
      ghcr.io/${{ github.repository }}

# Login to both registries
- uses: docker/login-action@v3
  with:
    registry: gcr.io
    username: _json_key
    password: ${{ secrets.GCR_JSON_KEY }}

- uses: docker/login-action@v3
  with:
    registry: ghcr.io
    username: ${{ github.actor }}
    password: ${{ secrets.GITHUB_TOKEN }}

# Single build-push-action pushes to BOTH registries
- uses: docker/build-push-action@v6
  with:
    push: true
    tags: ${{ steps.meta.outputs.tags }}  # Contains tags for both registries
    labels: ${{ steps.meta.outputs.labels }}
```

**GHCR Authentication:** Uses `GITHUB_TOKEN` (automatic, no additional secrets needed). Requires `packages: write` permission in the job.

**GHCR Naming Convention:**
```
ghcr.io/<OWNER>/<REPO>:<TAG>
ghcr.io/infobloxopen/cq-source-s3:v1.2.3-abc1234
```

The `docker/build-push-action` natively supports pushing to multiple registries in a single build when `metadata-action` provides tags for both.

### Alternatives Considered

| Alternative | Why Rejected |
|---|---|
| GCR only | Doesn't satisfy "registered as a package" requirement. GHCR provides discoverability within GitHub. |
| GHCR only | Spec explicitly requires GCR. |
| `docker tag` + separate push | Unnecessary — `build-push-action` handles multi-registry push natively via multiple tags. |
| Linking external image as GitHub Package | Not supported by GitHub. Packages must be pushed to `ghcr.io`. |

---

## Topic 6: Go Static Compilation for Distroless

### Decision

Use `CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -ldflags="-s -w -X ..." -o /app .` — confirmed fully static, no CGO gotchas with our dependency set.

### Rationale

**Static Binary Confirmation:**

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH \
  go build -ldflags="-s -w -X github.com/infobloxopen/cq-source-s3/plugin.Version=${VERSION}" \
  -o /out/cq-source-s3 .
```

- `CGO_ENABLED=0` — disables cgo entirely. The Go compiler uses pure-Go implementations of the `net`, `os/user`, and `crypto` packages instead of calling into system C libraries.
- The resulting binary is fully statically linked — no dependencies on glibc, libpthread, or any shared libraries.
- Verified compatible with `gcr.io/distroless/static-debian12` which has no libc.

**CGO Dependency Analysis for This Project:**

| Dependency | CGO Required? | Notes |
|---|---|---|
| `github.com/cloudquery/plugin-sdk/v4` | **No** | Pure Go. gRPC server uses pure-Go `net` package. |
| `github.com/aws/aws-sdk-go-v2` | **No** | Pure Go. HTTP client, JSON parsing — all standard library. |
| `github.com/apache/arrow-go/v18` | **No** | Pure Go. Parquet reader uses pure-Go implementations. |
| `github.com/rs/zerolog` | **No** | Pure Go. Zero-allocation structured logging. |

**No CGO gotchas** — all dependencies in `go.mod` are pure Go. The AWS SDK v2 specifically avoids CGO (unlike the V1 SDK which had optional CGO dependencies for performance).

**ldflags Breakdown:**

| Flag | Purpose |
|---|---|
| `-s` | Omit the symbol table — reduces binary size ~20% |
| `-w` | Omit DWARF debugging info — further reduces binary size |
| `-X github.com/infobloxopen/cq-source-s3/plugin.Version=${VERSION}` | Injects version string at build time into `plugin.Version` variable |

The `plugin.Version` variable is already declared in [plugin/plugin.go](plugin/plugin.go):
```go
var Version = "development"
```

This is the standard CloudQuery plugin pattern — the SDK uses `Version` for plugin registration and the gRPC handshake.

**Expected Binary Size:** ~25-35 MiB (with `-s -w`), resulting in a final container image of ~27-37 MiB compressed — well under the 50 MiB target.

### Alternatives Considered

| Alternative | Why Rejected |
|---|---|
| `CGO_ENABLED=1` + `base-debian12` | No dependencies require CGO. Would add ~18 MiB to the image for no benefit. |
| `-trimpath` flag | Good practice for reproducible builds (removes local file paths from binary). Recommended addition: `go build -trimpath -ldflags=...` |
| `-buildvcs=false` | Prevents embedding VCS info. Not needed since we control version via ldflags. Could add for reproducibility. |
| UPX binary compression | Can reduce binary size ~60% but causes slower startup and issues with some security scanners. Not worth the trade-off. |

**Recommendation:** Add `-trimpath` to the build flags for reproducible builds:
```bash
go build -trimpath -ldflags="-s -w -X ..." -o /out/cq-source-s3 .
```

---

## Topic 7: GitHub Actions Buildx Cache Strategies

### Decision

Use `type=gha` (GitHub Actions Cache backend API) as the primary cache strategy.

### Rationale

**Cache Backend Comparison:**

| Strategy | Mechanism | Pros | Cons | Best For |
|---|---|---|---|---|
| `type=gha` | GitHub Actions Cache API v2 | Zero config, automatic cleanup, 10 GB limit per repo, native integration | 10 GB limit can be tight for multi-arch | GitHub Actions (recommended) |
| `type=registry` | Push cache layers to registry | Unlimited size, shared across workflows, survives runner changes | Requires registry auth, doubles push time, costs storage | Self-hosted runners, large images |
| `type=local` | Local filesystem + `actions/cache` | Full control over cache location | Manual cache rotation bug workaround needed, 10 GB limit | Legacy setups |
| `type=inline` | Embed cache metadata in image | Simple, no extra storage | Only supports `min` mode (final layers only, not intermediate) | Simple single-arch builds |

**Why `type=gha`?**
- **Simplest configuration** — just two lines in `build-push-action`.
- **`mode=max`** caches ALL build layers (not just final), which is critical for multi-stage builds where the Go build layer changes frequently.
- GitHub Actions Cache API v2 is the only supported version (v1 was sunset April 15, 2025).
- Works out of the box with `docker/setup-buildx-action@v3` (which installs Buildx >= v0.21.0).

**Configuration:**

```yaml
- name: Set up Docker Buildx
  uses: docker/setup-buildx-action@v3

- name: Build and push
  uses: docker/build-push-action@v6
  with:
    context: .
    platforms: linux/amd64,linux/arm64
    push: true
    tags: ${{ steps.meta.outputs.tags }}
    labels: ${{ steps.meta.outputs.labels }}
    cache-from: type=gha
    cache-to: type=gha,mode=max
```

**Multi-Arch Cache Consideration:**
- With multi-arch builds, BuildKit creates separate cache entries per platform.
- The 10 GB GitHub Actions cache limit is sufficient for a Go binary project (~2-3 GB typical usage for two architectures).
- If cache pressure becomes an issue, can add `scope` parameter: `cache-to: type=gha,mode=max,scope=buildx-${{ github.ref_name }}` to scope cache per branch.

**Go Module Cache in Dockerfile:**
- The `--mount=type=cache,target=/go/pkg/mod` and `--mount=type=cache,target=/root/.cache/go-build` in the Dockerfile are BuildKit-internal cache mounts.
- These are **separate** from the `type=gha` layer cache. They cache within a single builder instance.
- For cross-job persistence of Go module cache, the `type=gha` layer cache captures the build layers that include module downloads, so modules are effectively cached.
- If additional Go-specific caching is needed, `reproducible-containers/buildkit-cache-dance` can extract BuildKit cache mounts into GitHub Actions cache, but this adds complexity and is typically unnecessary.

### Alternatives Considered

| Alternative | Why Rejected |
|---|---|
| `type=registry` as primary | Doubles push time (cache layers + image layers). Adds complexity with registry auth for cache. Reserve as fallback if `type=gha` 10 GB limit is hit. |
| `type=local` with `actions/cache` | Requires manual cache rotation workaround (move cache step). `type=gha` handles this natively. |
| `type=inline` | Only supports `min` mode — won't cache the Go build stage, only the final `COPY` layer. Useless for our multi-stage build. |
| `buildkit-cache-dance` for Go caches | Adds ~3 extra workflow steps and an external action dependency. The `type=gha` layer cache already captures Go module downloads. Only worth it if build times are unacceptable. |
| `type=gha` + `type=registry` (dual) | `cache-from` can list multiple sources (fallback chain). Possible future optimization: `cache-from: type=gha\ntype=registry,ref=gcr.io/PROJECT/cq-source-s3:buildcache`. Adds complexity. |

---

## Summary of Decisions

| Topic | Decision |
|---|---|
| **Base Image** | `gcr.io/distroless/static-debian12:nonroot` |
| **Multi-Arch Build** | `FROM --platform=$BUILDPLATFORM` + Go cross-compilation via `GOOS`/`GOARCH` |
| **GCR Auth** | `google-github-actions/auth@v3` (WIF preferred) or `docker/login-action@v3` with `_json_key` |
| **Tag Strategy** | `docker/metadata-action@v5` with `type=raw` for `v{version}-{sha}`, `type=semver`, and `latest` |
| **GitHub Packages** | Dual-publish to `gcr.io` and `ghcr.io` via multi-tag `build-push-action` |
| **Static Compilation** | `CGO_ENABLED=0` + `-ldflags="-s -w -X ...Version=${VERSION}"` + `-trimpath` |
| **Cache Strategy** | `type=gha,mode=max` (GitHub Actions Cache API v2) |

## Action Item Versions (pinned)

| Action | Version | Notes |
|---|---|---|
| `actions/checkout` | `v4` | |
| `docker/setup-qemu-action` | `v3` | Required for arm64 emulation in final stage |
| `docker/setup-buildx-action` | `v3` | Creates docker-container builder with Buildx >= v0.21.0 |
| `docker/metadata-action` | `v5` | Tag/label computation |
| `docker/build-push-action` | `v6` | Build + push (v6.19.2 latest) |
| `docker/login-action` | `v3` | Registry authentication |
| `google-github-actions/auth` | `v3` | GCP Workload Identity Federation (v3, Node 24) |
