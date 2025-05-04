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
	@docker-compose -f ./docker-compose/docker-compose.yaml up -d 


.PHONY: proxy-down
proxy-down:
	@docker-compose -f ./docker-compose/docker-compose.yaml down


