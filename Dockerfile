FROM golang:1.22.4-alpine AS builder

WORKDIR /app

COPY . .

RUN go build -ldflags="-extldflags=-static" -o tmp/goproxy cmd/goproxy.go

# ---- Final Stage ----
FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/tmp/goproxy /app
RUN apk add --no-cache git

CMD ["/app/goproxy"]
