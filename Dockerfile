FROM golang:1.22.4-alpine AS builder

WORKDIR /app

COPY cmd/ cmd/
COPY go.mod go.mod
COPY go.sum go.sum

RUN go build -ldflags="-extldflags=-static" -o tmp/goproxy cmd/goproxy.go

# ---- Final Stage ----
FROM alpine:latest

WORKDIR /app
RUN apk add --no-cache git
COPY --from=builder /app/tmp/goproxy /app

CMD ["/app/goproxy"]
