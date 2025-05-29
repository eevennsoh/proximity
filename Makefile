
VERSION = 0.0.1
IMAGE = docker.atl-paas.net/vportella/central-ai-proxy:$(VERSION)
ENVVAR ?= CGO_ENABLED=0

# ARCH=$(if $(TARGETPLATFORM),$(lastword $(subst /, ,$(TARGETPLATFORM))),amd64)
ARCH=arm64

BASE_PACKAGE = bitbucket.org/atlassian-developers/mini-proxy

CONFIG = $(shell cat config/central-ai-config.yaml | base64 | tr -d "\n")
CONFIG_DOCKER = $(shell cat config/central-ai-config-docker.yaml | base64 | tr -d "\n")

BUILD_LD_FLAGS = -X 'main.Config=${CONFIG}'
BUILD_LD_FLAGS_DOCKER = -X 'main.Config=${CONFIG_DOCKER}'

BIN = mini-proxy

.PHONY: build docker build-linux
.DEFAULT_GOAL := build

build:
	go build -o bin/${BIN} -ldflags="${BUILD_LD_FLAGS}" cmd/main.go

build-linux:
	$(ENVVAR) GOOS=linux GOARCH=$(ARCH) go build -a -installsuffix cgo -o bin/linux/${BIN} -ldflags="${BUILD_LD_FLAGS_DOCKER}" cmd/main.go

# test:
# 	go test -cover ./pkg/...
# 	go test -cover ./cmd/...

# lint:
# 	golangci-lint run

docker:
	docker buildx build --build-arg ENVVAR="$(ENVVAR)" -t $(IMAGE) --platform linux/$(ARCH) .

docker-push:
	docker push $(IMAGE)
