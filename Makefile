
.PHONY: build
build:
	@go build -o tmp/goproxy cmd/goproxy.go



run:
	@./tmp/goproxy