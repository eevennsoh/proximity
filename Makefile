
VERSION = 0.0.1
ENVVAR ?= CGO_ENABLED=0

# ARCH=$(if $(TARGETPLATFORM),$(lastword $(subst /, ,$(TARGETPLATFORM))),amd64)
ARCH=arm64

CONFIG = $(shell cat config.yaml | base64 | tr -d "\n")
CONFIG_DEV = $(shell cat config.yaml | base64 | tr -d "\n")

BUILD_LD_FLAGS = -X 'main.Config=${CONFIG}'
BUILD_LD_FLAGS_DEV = -X 'main.Config=${CONFIG_DEV}'

.PHONY: run build package
.DEFAULT_GOAL := run

run:
	wails dev -ldflags "$(BUILD_LD_FLAGS_DEV)"

build:
	wails build -clean -platform darwin/$(ARCH) -ldflags "$(BUILD_LD_FLAGS)"

package: build
	@mkdir -p dist
	@set -e; \
	app_bundle=$$(ls -d build/bin/*.app | head -n 1); \
	if [ -z "$$app_bundle" ]; then echo "No .app bundle found under build/bin"; exit 1; fi; \
	out="proximity-$(ARCH)-$(VERSION).tar.gz"; \
	tar -C build/bin -czf "$$out" "$$(basename "$$app_bundle")"; \
	echo "Created $$out"

upload:
	atlas statlas put \
		-n vportella \
		-f proximity-$(ARCH)-$(VERSION).tar.gz \
		--auth-group eng-compute-orchestration-kitt \
		-s proximity/

# test:
# 	go test -cover ./pkg/...
# 	go test -cover ./cmd/...

# lint:
# 	golangci-lint run
