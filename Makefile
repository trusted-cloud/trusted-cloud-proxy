
.PHONY: build
build:
	@go build -mod=mod -o tmp/goproxy cmd/goproxy.go



run:
	@./tmp/goproxy