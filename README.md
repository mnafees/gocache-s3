# gocache-s3

A [GOCACHEPROG](https://pkg.go.dev/cmd/go/internal/cacheprog) implementation that uses Amazon S3 (or any S3-compatible storage) as a shared, distributed Go build cache.

## Why

Go 1.24 introduced `GOCACHEPROG`, a protocol that lets the Go toolchain delegate build caching to an external program. This enables shared build caches that persist across machines and ephemeral CI environments, drastically reducing redundant compilation across your team or fleet.

## Installation

```bash
go install github.com/mnafees/gocache-s3@latest
```

## Quick start

```bash
export GOCACHE_S3_BUCKET=my-build-cache
export GOCACHEPROG=gocache-s3
go build ./...
```

Every `go build`, `go test`, and `go run` will now read from and write to S3.

## Configuration

### Flags / environment variables

| Flag | Env var | Required | Description |
|------|---------|----------|-------------|
| `-bucket` | `GOCACHE_S3_BUCKET` | yes | S3 bucket name |
| `-prefix` | `GOCACHE_S3_PREFIX` | no | Key prefix within the bucket |
| `-path-style` | `GOCACHE_S3_PATH_STYLE` | no | Use path-style addressing (set to `true` or `1`) |

Flags override their corresponding environment variable. When using env vars, simply set `GOCACHEPROG=gocache-s3`. When using flags, pass them through `GOCACHEPROG`:

```bash
export GOCACHEPROG="gocache-s3 -bucket my-cache -prefix myproject"
```

### AWS credentials and region

All standard AWS SDK configuration is supported: environment variables (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_REGION`), shared config files (`~/.aws/config`), IAM instance roles, and IRSA for EKS.

For custom S3 endpoints, set `AWS_ENDPOINT_URL`.

## S3-compatible storage

Any storage engine that speaks the S3 API works out of the box.

### LocalStack (local development)

Start LocalStack:

```bash
docker compose up -d
```

Create a bucket and point gocache-s3 at it:

```bash
aws --endpoint-url=http://localhost:4566 s3 mb s3://build-cache

export AWS_ENDPOINT_URL=http://localhost:4566
export AWS_ACCESS_KEY_ID=test
export AWS_SECRET_ACCESS_KEY=test
export AWS_REGION=us-east-1
export GOCACHE_S3_BUCKET=build-cache
export GOCACHE_S3_PATH_STYLE=true
export GOCACHEPROG=gocache-s3
go build ./...
```

### MinIO

```bash
export AWS_ENDPOINT_URL=http://localhost:9000
export AWS_ACCESS_KEY_ID=minioadmin
export AWS_SECRET_ACCESS_KEY=minioadmin
export GOCACHE_S3_BUCKET=build-cache
export GOCACHE_S3_PATH_STYLE=true
export GOCACHEPROG=gocache-s3
```

### Cloudflare R2

```bash
export AWS_ENDPOINT_URL=https://<account-id>.r2.cloudflarestorage.com
export AWS_ACCESS_KEY_ID=<r2-access-key>
export AWS_SECRET_ACCESS_KEY=<r2-secret-key>
export GOCACHE_S3_BUCKET=build-cache
export GOCACHEPROG=gocache-s3
```

### Google Cloud Storage (S3-compatible)

```bash
export AWS_ENDPOINT_URL=https://storage.googleapis.com
export AWS_ACCESS_KEY_ID=<hmac-access-id>
export AWS_SECRET_ACCESS_KEY=<hmac-secret>
export GOCACHE_S3_BUCKET=build-cache
export GOCACHEPROG=gocache-s3
```

## How it works

The Go toolchain spawns `gocache-s3` as a subprocess and communicates over stdin/stdout using line-delimited JSON. Three operations are supported:

- **get** -- look up a cache entry by its action ID. On hit, the object is downloaded from S3 and written to a temporary file whose path is returned to the Go toolchain.
- **put** -- store a build artifact. The body is written to both a temporary file (for immediate use by the toolchain) and S3 (for sharing with other machines).
- **close** -- clean up temporary files and exit.

Cache keys are hex-encoded action IDs. Output IDs and timestamps are stored as S3 object metadata, so no secondary index or database is needed.
