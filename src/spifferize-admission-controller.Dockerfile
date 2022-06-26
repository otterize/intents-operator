# This dockerfile should be build from the repo root folder
FROM golang:1.18-alpine as builder
RUN apk add --no-cache ca-certificates git protoc
RUN apk add build-base

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN GOOS=linux GOARCH=amd64 go build -o /admission-controller ./admission_controller/cmd

FROM --platform=linux/amd64 alpine:latest

WORKDIR /
COPY --from=builder /admission-controller /admission-controller
RUN adduser -D nonroot nonroot
EXPOSE 8443
USER nonroot:nonroot
CMD /admission-controller
