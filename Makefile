OWNER ?= ogre0403
IMAGE_NAME ?= goproxy
REPO_TOKEN ?= replace-your-token-from-repo

.PHONY: go-build
go-build:
	@go build -ldflags="-extldflags=-static" -o tmp/goproxy cmd/goproxy.go

.PHONY: run
run:
	@./tmp/goproxy


.PHONY: release-image
release-image:
	@docker build -t $(OWNER)/$(IMAGE_NAME) .


.PHONY: run-proxy
run-proxy: 
	@docker run -ti --rm \
	-e REPO_TOKEN=$(REPO_TOKEN) \
	-e SRC_REPO=pegasus-cloud.com/aes \
	-e DEST_REPO=github.com/trusted-cloud \
	-p 8078:8078 \
	$(OWNER)/$(IMAGE_NAME)

