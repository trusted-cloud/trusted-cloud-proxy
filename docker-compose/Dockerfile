FROM golang:1.22.4-alpine AS builder


WORKDIR /app

RUN apk add git make


RUN git clone https://github.com/goproxyio/goproxy.git && \
    cd goproxy && \
    make

RUN git clone https://github.com/GoogleCloudPlatform/govanityurls.git && \
    cd govanityurls && \
    go build -ldflags="-extldflags=-static"


FROM alpine:3.21
COPY --from=builder /app/goproxy/bin/goproxy /usr/local/bin/goproxy
COPY --from=builder /app/govanityurls/govanityurls /usr/local/bin/govanityurls


# golang and git are required for goproxy
RUN mkdir -p /run/nginx && \
    apk add go git nginx

COPY nginx.conf /etc/nginx/nginx.conf


COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

CMD ["/entrypoint.sh"]
