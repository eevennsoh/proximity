
VERSION = 0.0.10
ENVVAR ?= CGO_ENABLED=0

# ARCH=$(if $(TARGETPLATFORM),$(lastword $(subst /, ,$(TARGETPLATFORM))),amd64)
ARCH=arm64

CONFIG = $(shell cat config.yaml | base64 | tr -d "\n")
CONFIG_DEV = $(shell cat config-dev.yaml | base64 | tr -d "\n")

# Multiple extensions tried by Proximity
SETTINGS_PATH = /.config/proximity/settings
SETTINGS_PATH_DEV = /.config/proximity/settings-dev

MODELS = $(shell cat models.json | base64 | tr -d "\n")

VERSION_URL = "https://statlas.prod.atl-paas.net/vportella/proximity/version.json"

BUILD_LD_FLAGS_COMMON = -X 'main.TemplateVariables=$(MODELS)'\
	-X 'main.Version=$(VERSION)' \
	-X 'bitbucket.org/atlassian-developers/proximity/internal/update.versionUrl=$(VERSION_URL)'

BUILD_LD_FLAGS        = $(BUILD_LD_FLAGS_COMMON) -X 'main.Config=$(CONFIG)' -X 'main.Port=29576' -X 'main.SettingsPath=$(SETTINGS_PATH)'
BUILD_LD_FLAGS_DEV    = $(BUILD_LD_FLAGS_COMMON) -X 'main.Config=$(CONFIG_DEV)' -X 'main.Port=29575' -X 'main.SettingsPath=$(SETTINGS_PATH_DEV)'

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

publish:
	# Generate version.json
	@echo '{"version":"$(VERSION)","published_at":"'$$(date -u +%Y-%m-%dT%H:%M:%SZ)'"}' > version.json

	# Upload the versioned package
	atlas statlas put \
		-n vportella \
		-f proximity-$(ARCH)-$(VERSION).tar.gz \
		--auth-group eng-compute-orchestration-kitt \
		-s proximity/

	# Upload as latest
	mv proximity-$(ARCH)-$(VERSION).tar.gz proximity-$(ARCH)-latest.tar.gz

	atlas statlas put \
		-n vportella \
		-f proximity-$(ARCH)-latest.tar.gz \
		--auth-group eng-compute-orchestration-kitt \
		-s proximity/

	# Upload version manifest
	atlas statlas put \
		-n vportella \
		-f version.json \
		--auth-group eng-compute-orchestration-kitt \
		-s proximity/

	rm proximity-$(ARCH)-latest.tar.gz

reset-changelog-history:
	@echo "Resetting Proximity changelog history (simulates existing user with dummy version)..."
	@db_path=$$(ls ~/Library/WebKit/com.wails.Proximity/WebsiteData/Default/*/*/LocalStorage/localstorage.sqlite3 2>/dev/null | head -1); \
	if [ -n "$$db_path" ]; then \
		sqlite3 "$$db_path" "DELETE FROM ItemTable WHERE key = 'proximity_seen_changelog_versions'; INSERT INTO ItemTable (key, value) VALUES ('proximity_seen_changelog_versions', X'5B00220030002E0030002E00300022005D00');"; \
		echo "Done. localStorage now has dummy version 0.0.0. Restart the app to see the changelog modal."; \
	else \
		echo "No localStorage database found. Run the app first to create it."; \
	fi

# test:
# 	go test -cover ./pkg/...
# 	go test -cover ./cmd/...

# lint:
# 	golangci-lint run
