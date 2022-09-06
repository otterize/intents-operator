FROM golang:1.18-alpine as builder
RUN apk add --no-cache ca-certificates git protoc
RUN apk add build-base

WORKDIR /workspace

COPY go.mod go.sum ./
ARG GITHUB_TOKEN
RUN --mount=type=secret,id=github_token \
    if [ -f /run/secrets/github_token ]; then export GITHUB_TOKEN=$(cat /run/secrets/github_token); fi && \
    git config --global url."https://$GITHUB_TOKEN@github.com/".insteadOf "https://github.com/" &&  \
    go mod download &&  \
    git config --global --unset url."https://$GITHUB_TOKEN@github.com/".insteadOf

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o manager ./operator/main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/operator"]