FROM golang:1.18 as builder

RUN ls -lh
WORKDIR /workspace
COPY go.mod go.mod
COPY go.sum go.sum

RUN go mod download

COPY watcher/cmd/main.go main.go
COPY shared/api shared/api/
COPY watcher/ watcher/

RUN go test ./watcher/... && go build -a -o /watcher main.go


FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/watcher .

ENTRYPOINT ["/watcher"]