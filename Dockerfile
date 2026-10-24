# Multi-stage build for kypulse
# Base images are pinned by index digest; Dependabot (docker ecosystem) moves tag and digest together.

# Stage 1: Build React PWA Frontend
FROM node:26.12.1-alpine@sha256:40cd011b55415bc894e45b02fdd5a5dfde056921b7d23a64c74f80497af48797 AS frontend-builder
WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Stage 2: Build Go Standalone Binary
FROM golang:1.27.3-alpine@sha256:f2c872bcee949b7be9c490efd02b48a8a2fbf947cdf533f1c58525ec9d8170fd AS backend-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
COPY --from=frontend-builder /app/web/dist ./web/dist
# Release builds pass the tag's version (X.Y.Z or X.Y.Z.N); other builds report internal/appversion/VERSION.
# -trimpath keeps the build directory out of the binary, so the same commit, Go version and
# VERSION give the same bytes wherever it is built (docs/VERIFYING-RELEASES.md).
ARG VERSION=
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags="-s -w -X github.com/Busnes-app/kypulse-server/internal/appversion.release=${VERSION}" \
    -o kypulse ./cmd/server

# Stage 3: Minimal Production Container
FROM alpine:3.24@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=backend-builder /app/kypulse /app/kypulse
# Runs as a fixed unprivileged uid; bind-mounted host directories must be owned by it
# (README, Upgrading to a non-root container). /app/backups is the optional mount for sealed
# local capsules; KYPULSE_BACKUP_DIR is set by the operator (compose does), so an image run
# bare keeps no local copies. /app/secrets holds the audit key apart from the database.
RUN addgroup -S -g 10001 kypulse && adduser -S -D -H -u 10001 -G kypulse kypulse \
    && mkdir -p /app/data /app/backups /app/secrets \
    && chown 10001:10001 /app/data /app/backups /app/secrets \
    && chmod 700 /app/data /app/backups /app/secrets
USER 10001:10001

ENV KYPULSE_ENV=production
ENV KYPULSE_PORT=8080
ENV KYPULSE_HOST=0.0.0.0
ENV KYPULSE_DATA_DIR=/app/data

EXPOSE 8080
VOLUME ["/app/data", "/app/backups"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --start-interval=2s --retries=3 \
    CMD wget -q -O /dev/null "http://127.0.0.1:${KYPULSE_PORT}/healthz" || exit 1

ENTRYPOINT ["/app/kypulse"]
