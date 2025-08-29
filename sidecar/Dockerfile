# Build the sidecar binary
FROM golang:1.24 AS builder
WORKDIR /workspace

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# Cache deps before building and copying source
RUN go mod download

# Copy the sidecar source code
COPY sidecar/ sidecar/

# Build the sidecar binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o pprof-sidecar sidecar/cmd/main.go

# Use distroless as minimal base image to package the sidecar binary
FROM alpine
WORKDIR /
COPY --from=builder /workspace/pprof-sidecar .
USER 65532:65532

ENTRYPOINT ["/pprof-sidecar"]