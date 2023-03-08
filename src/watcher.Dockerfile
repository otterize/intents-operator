FROM golang:1.18 as builder

WORKDIR /workspace
COPY go.mod go.mod
COPY go.sum go.sum

RUN go mod download

COPY watcher/cmd/main.go main.go
COPY operator/api operator/api/
COPY shared shared
COPY watcher/ watcher/

RUN go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
RUN KUBEBUILDER_ASSETS="setup-envtest use 1.24.1 -p path)" go test ./watcher/...

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o watcherbin main.go

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/watcherbin main
USER 65532:65532

ENTRYPOINT ["/main"]