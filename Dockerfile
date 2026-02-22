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
    go build -trimpath \
    -ldflags="-s -w -X github.com/infobloxopen/cq-source-s3/plugin.Version=${VERSION}" \
    -o /out/cq-source-s3 .

# ── Stage 2: Runtime ────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /out/cq-source-s3 /cq-source-s3

EXPOSE 7777

ENTRYPOINT ["/cq-source-s3"]
CMD ["serve", "--address", "[::]:7777"]
