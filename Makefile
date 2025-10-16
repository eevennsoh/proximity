
VERSION = 0.0.5
ENVVAR ?= CGO_ENABLED=0

# ARCH=$(if $(TARGETPLATFORM),$(lastword $(subst /, ,$(TARGETPLATFORM))),amd64)
ARCH=arm64

CONFIG = $(shell cat config.yaml | base64 | tr -d "\n")
CONFIG_DEV = $(shell cat config-dev.yaml | base64 | tr -d "\n")

MODELS = $(shell cat models.json | base64 | tr -d "\n")

BUILD_LD_FLAGS = -X 'main.Config=$(CONFIG)' -X 'main.TemplateVariables=$(MODELS)' -X 'main.Port=29576'
BUILD_LD_FLAGS_DEV = -X 'main.Config=$(CONFIG_DEV)' -X 'main.TemplateVariables=$(MODELS)' -X 'main.Port=29575'

.PHONY: run build package
.DEFAULT_GOAL := run

refresh-models:
	./generate_models_json.sh

run:
	wails dev -skipbindings -ldflags "$(BUILD_LD_FLAGS_DEV)"

build: refresh-models
	wails build -skipbindings -clean -platform darwin/$(ARCH) -ldflags "$(BUILD_LD_FLAGS)"

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

	mv proximity-$(ARCH)-$(VERSION).tar.gz proximity-$(ARCH)-latest.tar.gz

	atlas statlas put \
		-n vportella \
		-f proximity-$(ARCH)-latest.tar.gz \
		--auth-group eng-compute-orchestration-kitt \
		-s proximity/

	rm proximity-$(ARCH)-latest.tar.gz

# test:
# 	go test -cover ./pkg/...
# 	go test -cover ./cmd/...

# lint:
# 	golangci-lint run
