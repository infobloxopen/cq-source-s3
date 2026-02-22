# Quickstart: Docker Build & Publish

**Branch**: `002-docker-build-publish` | **Spec**: [spec.md](spec.md) | **Plan**: [plan.md](plan.md)

---

## For Operators: Running the Container

### Pull and run

```bash
# Using GCR
docker run -p 7777:7777 gcr.io/<PROJECT_ID>/cq-source-s3:latest

# Using GHCR
docker run -p 7777:7777 ghcr.io/infobloxopen/cq-source-s3:latest
```

The plugin starts and listens on gRPC port 7777. Connect using CloudQuery CLI or any gRPC client.

### With AWS credentials

```bash
docker run -p 7777:7777 \
  -e AWS_ACCESS_KEY_ID=AKIA... \
  -e AWS_SECRET_ACCESS_KEY=... \
  -e AWS_REGION=us-east-1 \
  gcr.io/<PROJECT_ID>/cq-source-s3:v1.2.3-abc1234
```

### Verify multi-arch support

```bash
docker manifest inspect gcr.io/<PROJECT_ID>/cq-source-s3:v1.2.3-abc1234
```

Output should list `linux/amd64` and `linux/arm64` platforms.

---

## For Developers: Building Locally

### Prerequisites

- Docker with Buildx enabled (Docker Desktop includes it)
- Go 1.25+ (for running tests)

### Build the image locally

```bash
# Single-arch (current platform)
make docker-build

# Multi-arch (requires buildx builder)
docker buildx create --name multiarch --driver docker-container --use
make docker-build-multiarch
```

### Run tests, then build

```bash
# Run unit tests
make test

# Run E2E tests (requires LocalStack)
make e2e

# Build Docker image
make docker-build
```

---

## For Maintainers: Publishing a Release

### 1. Tag the release

```bash
git tag v1.2.3
git push origin v1.2.3
```

### 2. CI handles the rest

The release workflow automatically:
1. Runs lint
2. Runs unit tests + E2E tests (against LocalStack)
3. Builds multi-arch image (amd64 + arm64)
4. Pushes to GCR and GHCR with tags:
   - `v1.2.3-<short-sha>` (primary)
   - `v1.2.3` (semver)
   - `v1.2` (minor)
   - `latest`

### 3. Verify the published image

```bash
# Check the tag was pushed
docker pull gcr.io/<PROJECT_ID>/cq-source-s3:v1.2.3-abc1234

# Verify architectures
docker manifest inspect gcr.io/<PROJECT_ID>/cq-source-s3:v1.2.3-abc1234

# Run and test
docker run -p 7777:7777 gcr.io/<PROJECT_ID>/cq-source-s3:v1.2.3-abc1234
```

---

## Required Setup (One-Time)

### GitHub repository secrets & variables

| Name | Type | Value |
|------|------|-------|
| `GCR_JSON_KEY` | Secret | GCP service account key JSON |
| `GCP_PROJECT_ID` | Variable | Your GCP project ID |

GHCR authentication uses the automatic `GITHUB_TOKEN` — no additional setup required.

### GCP service account permissions

The service account referenced by `GCR_JSON_KEY` needs:
- `roles/storage.admin` on the `artifacts.<PROJECT_ID>.appspot.com` bucket (GCR storage)

Or, for a more locked-down setup, use Workload Identity Federation (see [research.md](research.md#topic-3-gcr-authentication-from-github-actions)).
