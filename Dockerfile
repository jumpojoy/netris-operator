# syntax=docker/dockerfile:experimental
# Build arguments provided by buildx
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG TARGETPLATFORM=linux/amd64
ARG BUILDPLATFORM=linux/amd64

# Build the manager binary
FROM --platform=$BUILDPLATFORM golang:1.18 as builder
ARG TARGETOS
ARG TARGETARCH
ARG TARGETPLATFORM

WORKDIR /workspace


# TODO(vsaienko): Remove when patch accepted in upstream
ARG DOWNLOAD_NETRISWEBAPI=true
COPY netriswebapi/ netriswebapi/
RUN if [ ${DOWNLOAD_NETRISWEBAPI} == "true" ]; then git clone -b extensions https://github.com/jumpojoy/netriswebapi.git /tmp/netriswebapi; else mv netriswebapi/ /tmp/; fi

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN --mount=type=ssh,id=ssh_private_key_ci go mod download

# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY controllers/ controllers/
COPY configloader/ configloader/
COPY lbwatcher/ lbwatcher/
COPY calicowatcher/ calicowatcher/
COPY netrisstorage/ netrisstorage/

# Build
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} GO111MODULE=on go build -a -o manager main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM --platform=$TARGETPLATFORM gcr.io/distroless/static:nonroot
ARG TARGETPLATFORM
WORKDIR /
COPY --from=builder /workspace/manager .
USER nonroot:nonroot

ENTRYPOINT ["/manager"]
