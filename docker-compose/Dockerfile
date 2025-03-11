FROM golang:1.22.4-alpine AS builder


WORKDIR /app

RUN apk add git make

RUN git clone https://github.com/goproxyio/goproxy.git && \
    cd goproxy && \
    make


RUN mkdir -p /run/nginx && \
    mkdir -p /etc/nginx/certs && \
    apk add --no-cache nginx

COPY nginx.conf /etc/nginx/nginx.conf
COPY pegasus.crt /etc/nginx/certs/pegasus.crt
COPY pegasus.key /etc/nginx/certs/pegasus.key

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

CMD ["/entrypoint.sh"]

