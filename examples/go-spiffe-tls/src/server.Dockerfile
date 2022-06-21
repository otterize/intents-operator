# syntax=docker/dockerfile:1

FROM golang:1.18-alpine as builder
RUN apk add build-base git mercurial

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY .. .
RUN go build -o /server ./server

# Common base
FROM alpine AS base


# Server
FROM base AS server
COPY --from=builder /server /server
EXPOSE 55555
RUN chmod +x /server
CMD [ "/server" ]
