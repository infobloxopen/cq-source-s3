# Contract: Release Workflow

**Type**: GitHub Actions Workflow
**Consumers**: Maintainers pushing semver tags, CI infrastructure

---

## Trigger

```yaml
on:
  push:
    tags:
      - 'v[0-9]+.[0-9]+.[0-9]+'
```

Only semver tags (e.g., `v1.2.3`) trigger the workflow. Pre-release suffixes (`-rc.1`, `-beta`) are NOT matched.

## Required Secrets & Variables

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `GCR_JSON_KEY` | Secret | Yes (unless using WIF) | GCP service account key JSON for GCR push |
| `GCP_PROJECT_ID` | Variable | Yes | GCP project ID for GCR image path |
| `GITHUB_TOKEN` | Automatic | Yes | Auto-provided; used for GHCR push |

## Pipeline Stages

```
┌─────────┐    ┌──────────────────┐    ┌───────────────┐    ┌───────────────┐
│  lint   │───▶│  test            │───▶│  build-push   │───▶│   release     │
│         │    │  (unit + E2E)    │    │  (multi-arch) │    │  (GH Release) │
└─────────┘    └──────────────────┘    └───────────────┘    └───────────────┘
                     │                        │                     │
                     │ failure                │ success             │ success
                     ▼                        ▼                     ▼
               pipeline FAILS           push to GCR + GHCR   GitHub Release
               (no publish)                                  with digest &
                                                             release notes
```

## Stage Details

### lint
- Runner: `ubuntu-latest`
- Steps: `golangci-lint` (version: latest)

### test
- Runner: `ubuntu-latest`
- Services: LocalStack (`localstack/localstack:latest`, port 4566)
- Steps: `go vet`, unit tests (`./client/... ./internal/... ./plugin/...`), E2E tests (`./test/...`)
- Gate: Must pass before `build-push` runs

### build-push
- Runner: `ubuntu-latest`
- Steps: QEMU setup, Buildx setup, GCR login, GHCR login, metadata computation, build & push
- Platforms: `linux/amd64,linux/arm64`
- Cache: `type=gha,mode=max`

### release
- Runner: `ubuntu-latest`
- Depends on: `build-push` (needs image digest output)
- Steps: Create GitHub Release via `softprops/action-gh-release@v2`
  - Tag: The triggering semver tag (e.g., `v1.2.3`)
  - Name: `v1.2.3`
  - Body: Auto-generated release notes (GitHub `generate_release_notes: true`) plus appended container pull commands and image digest
  - Draft: `false`
  - Prerelease: `false`
- Gate: Only runs after successful `build-push`

## Outputs

| Output | Description |
|--------|-------------|
| Image tags | All computed tags (semver+hash, semver, minor, latest) |
| Image digest | SHA256 digest of the published manifest |
| GitHub Release URL | URL of the created GitHub Release page |

## Permissions Required

```yaml
permissions:
  contents: write      # GitHub Release creation
  packages: write      # GHCR push
  id-token: write      # GCP Workload Identity Federation (if used)
```

## Failure Modes

| Scenario | Behaviour |
|----------|-----------|
| Lint fails | Pipeline stops, no tests run, no publish |
| Unit tests fail | Pipeline stops, no publish |
| E2E tests fail | Pipeline stops, no publish |
| GCR push fails | Pipeline fails with error, GHCR may or may not have been pushed |
| GHCR push fails | Pipeline fails with error, GCR may or may not have been pushed |
| Build fails for one arch | Entire build fails, no publish to any registry |
| Release creation fails | Images are published but no GitHub Release page is created; pipeline reports failure |
