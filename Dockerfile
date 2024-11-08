FROM --platform=${BUILDPLATFORM:-linux/amd64} golang:1.23.1 AS builder

WORKDIR /go/src/pika

COPY . .

RUN go build -o bin/pika cmd/pika/main.go

FROM --platform=${TARGETPLATFORM:-linux/amd64} gcr.io/distroless/static AS base

COPY --from=builder /go/src/pika/bin/pika /usr/bin

USER 1000:1000

ENTRYPOINT ["pika"]
