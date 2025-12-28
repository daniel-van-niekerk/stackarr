# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Get version from git tags or use "dev"
ARG VERSION=dev
RUN echo "Building version: ${VERSION}"

# Build the application with version injected
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.Version=${VERSION}" \
    -o stackarr ./cmd/stackarr

# Runtime stage
FROM alpine:latest

# Install ca-certificates and docker cli for HTTPS requests
RUN apk --no-cache add ca-certificates docker-cli

# Create data directory
RUN mkdir -p /var/lib/stackarr/compose

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/stackarr .

# Copy web assets
COPY --from=builder /build/web ./web

# Expose port
EXPOSE 8877

# Set environment variables
ENV SERVER_HOST=0.0.0.0
ENV SERVER_PORT=8877
ENV DATABASE_PATH=/var/lib/stackarr/stackarr.db

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8877/ || exit 1

# Run the application
CMD ["./stackarr"]
