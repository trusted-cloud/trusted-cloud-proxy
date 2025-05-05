OWNER        ?= ogre0403
IMAGE_NAME   ?= goproxy

.PHONY: go-build
go-build:
	@go build -ldflags="-extldflags=-static" -o tmp/goproxy cmd/goproxy.go

.PHONY: run
run:
	@./tmp/goproxy


.PHONY: release-image
release-image:
	@docker build -t $(OWNER)/$(IMAGE_NAME) .


.PHONY: proxy-up
proxy-up:
	cd docker-compose; \
	cat docker-compose.yaml  | docker-compose -f - up -d


.PHONY: proxy-up-pegasus-network
proxy-up-pegasus-network:
	cd docker-compose; \
	cat docker-compose.yaml | yq e '.services.trust-cloud-proxy.networks += {"pegasus-cloud-network": {}}' | docker-compose -f - up -d

.PHONY: proxy-down
proxy-down:
	@docker-compose -f ./docker-compose/docker-compose.yaml down


