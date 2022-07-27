FROM golang:1.18 as builder

RUN ls -lh
WORKDIR /workspace
COPY go.mod go.mod
COPY go.sum go.sum

RUN go mod download

COPY watcher/cmd/main.go main.go
COPY shared/api shared/api/
COPY watcher/ watcher/

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o /watcher main.go


ENTRYPOINT ["/watcher"]