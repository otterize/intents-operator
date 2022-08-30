# syntax=docker/dockerfile:1

FROM golang:1.18-alpine as builder
RUN apk add build-base git mercurial

WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY main.go .
RUN go build -o /server ./main.go

# Common base
FROM alpine AS base


# Server
FROM base AS server
COPY --from=builder /server /server
EXPOSE 55555
RUN chmod +x /server
CMD [ "/server" ]
