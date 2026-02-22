# Contract: Container Image

**Type**: OCI Container Image
**Consumers**: Operators deploying cq-source-s3, CloudQuery CLI connecting via gRPC

---

## Image Coordinates

```
# Google Container Registry
gcr.io/<GCP_PROJECT_ID>/cq-source-s3:<TAG>

# GitHub Container Registry
ghcr.io/infobloxopen/cq-source-s3:<TAG>
```

## Tag Format

| Tag Pattern | Example | Description |
|-------------|---------|-------------|
| `v<MAJOR>.<MINOR>.<PATCH>-<SHA7>` | `v1.2.3-abc1234` | Primary: semver + exact commit |
| `v<MAJOR>.<MINOR>.<PATCH>` | `v1.2.3` | Standard semver |
| `v<MAJOR>.<MINOR>` | `v1.2` | Minor tracking (updated on patches) |
| `latest` | `latest` | Most recent release |

## Supported Platforms

| OS | Architecture |
|----|-------------|
| linux | amd64 |
| linux | arm64 |

## Container Behaviour

| Property | Value |
|----------|-------|
| Entrypoint | `/cq-source-s3` |
| Default args | `serve --address [::]:7777` |
| Exposed port | `7777/tcp` (gRPC) |
| User | `nonroot` (UID 65534) |
| Shell | None (distroless) |
| Writable filesystem | No (read-only root recommended) |

## Runtime Configuration

The plugin is configured via the CloudQuery plugin SDK's standard mechanisms:

| Mechanism | Example | Description |
|-----------|---------|-------------|
| gRPC spec | CloudQuery CLI sends spec JSON via gRPC | Primary configuration path |
| CLI flags | `--address [::]:8080` | Override listen address |
| Environment | `AWS_REGION`, `AWS_ACCESS_KEY_ID`, etc. | AWS credentials and region |

## Usage Examples

```bash
# Pull and run (default: gRPC on port 7777)
docker run -p 7777:7777 gcr.io/PROJECT/cq-source-s3:v1.2.3-abc1234

# Run with AWS credentials
docker run -p 7777:7777 \
  -e AWS_ACCESS_KEY_ID=AKIA... \
  -e AWS_SECRET_ACCESS_KEY=... \
  -e AWS_REGION=us-east-1 \
  gcr.io/PROJECT/cq-source-s3:v1.2.3-abc1234

# Override listen address
docker run -p 8080:8080 \
  gcr.io/PROJECT/cq-source-s3:v1.2.3-abc1234 \
  serve --address [::]:8080

# Inspect multi-arch manifest
docker manifest inspect gcr.io/PROJECT/cq-source-s3:v1.2.3-abc1234
```

## Health Check

The CloudQuery plugin SDK does not expose a built-in HTTP health endpoint. Container health can be verified by:

1. **TCP port check**: `nc -z localhost 7777` (gRPC port is listening)
2. **gRPC reflection**: `grpc_health_probe -addr=localhost:7777` (if gRPC health service is registered)
3. **Process status**: Container exits with non-zero code on fatal startup errors
