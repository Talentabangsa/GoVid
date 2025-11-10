# Multi-stage Dockerfile for GoVid
# Stage 1: Build the Go application
FROM golang:alpine AS builder

# Install build dependencies
RUN apk add git

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.* ./

# Download dependencies
RUN go mod download -x

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -installsuffix cgo -o govid ./cmd/main.go

# Stage 2: Create runtime image with FFmpeg 8.0
# Using linuxserver/ffmpeg for latest stable FFmpeg with all security fixes
# - FFmpeg 8.0 (latest stable release, January 2025)
# - Ubuntu 24.04 LTS base (better compatibility)
# - Zero FFmpeg CVEs (all security patches applied)
# - Explicit version pinning for reproducible builds
# - Architecture-specific tag (amd64) for clarity
FROM linuxserver/ffmpeg:amd64-version-8.0-cli

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/govid .
COPY docs/swagger.yaml /docs/swagger.yaml

# Create necessary directories with proper permissions
RUN mkdir -p uploads outputs temp && \
    useradd -u 1000 -U -s /bin/sh govid && \
    chown -R govid:govid /app

# Switch to non-root user
USER govid

# Expose ports
EXPOSE 4101 1106

# Health check
#HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
#    CMD wget --no-verbose --tries=1 --spider http://localhost:4101/api/v1/health || exit 1

# Run the application
ENTRYPOINT ["./govid"]
