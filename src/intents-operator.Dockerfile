# Build the manager binary
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

RUN go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
RUN cd ./operator # FIXME: && make test

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o manager ./operator/main.go

ARG VERSION
RUN echo -n $VERSION > /version

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/manager .
COPY --from=builder /version .
USER 65532:65532

ENTRYPOINT ["/manager"]
