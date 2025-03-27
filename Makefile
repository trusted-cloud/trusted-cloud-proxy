OWNER        ?= ogre0403
IMAGE_NAME   ?= goproxy
REPO_TOKEN   ?= replace-your-token-from-repo
SRC_REPO     ?= pegasus-cloud.com/aes
DEST_REPO    ?= github.com/trusted-cloud
NETWORK_NAME ?= proxy-network

.PHONY: go-build
go-build:
	@go build -ldflags="-extldflags=-static" -o tmp/goproxy cmd/goproxy.go

.PHONY: run
run:
	@./tmp/goproxy


.PHONY: release-image
release-image:
	@docker build -t $(OWNER)/$(IMAGE_NAME) .

.PHONY: create-network
create-network:
	@docker network create $(NETWORK_NAME) || true


.PHONY: run-proxy
run-proxy: create-network
	@docker run -ti --rm \
	--name goproxy \
	-e REPO_TOKEN=$(REPO_TOKEN) \
	-e SRC_REPO=$(SRC_REPO) \
	-e DEST_REPO=$(DEST_REPO) \
	-p 8078:8078 \
	--network=$(NETWORK_NAME) \
	$(OWNER)/$(IMAGE_NAME)

.PHONY: clean
clean:
	@docker network rm $(NETWORK_NAME)

