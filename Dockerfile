FROM --platform=$BUILDPLATFORM golang:1.23 AS builder
SHELL ["/bin/bash", "-c"]

WORKDIR /workspace
# Copy the Go modules manifests.
COPY go.mod go.mod
COPY go.sum go.sum
# Cache deps before building and copying source so that we don't need to
# re-download as much and so that source changes don't invalidate our downloaded
# layer.
COPY vendor/ vendor/
RUN [[ $(ls -1 vendor/ | wc -l) -gt 0 ]] || (echo "Expected 'vendor' dependencies to be present, call 'go mod vendor' before building Docker image" && exit 1)

# Copy the go source
COPY cmd/ cmd/
COPY pkg/ pkg/

COPY custom_ranges.json /etc/kubenetmon/custom_ranges.json

# Build
ARG VERSION=dev
ARG GIT_COMMIT=unspecified
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -a -o bin/ ./...
RUN CGO_ENABLED=1 GOOS=linux go test -v -race -coverprofile cover.out -tags '!integration' ./...

# Use distroless as minimal base image to package the kubenetmon binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/bin .
USER 65532:65532
