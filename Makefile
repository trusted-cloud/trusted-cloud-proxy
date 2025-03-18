
.PHONY: build
build:
	@go build -ldflags="-extldflags=-static" -o tmp/goproxy cmd/goproxy.go


.PHONY: run
run:
	@./tmp/goproxy