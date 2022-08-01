# syntax=docker/dockerfile:1

FROM golang:1.18-alpine as builder
RUN apk add build-base git mercurial

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY ../.. .
RUN go build -o /client ./

# Common base
FROM alpine AS base


# client
FROM base AS client
COPY --from=builder /client /client
RUN chmod +x /client
CMD [ "/client" ]
