# Upgraded Go version? Make sure to upgrade it in the GitHub Actions setup, the Dockerfile and the go.mod as well, so the linter and tests run the same version.
FROM golang:1.21 as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY . .
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -a -o manager ./operator/main.go

ARG VERSION
RUN echo -n $VERSION > /version

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:debug
WORKDIR /
COPY --from=builder /workspace/manager .
COPY --from=builder /version .
USER 65532:65532

ENTRYPOINT ["/manager"]
