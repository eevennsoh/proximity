
VERSION = 1.0.0
ENVVAR ?= CGO_ENABLED=0

# ARCH=$(if $(TARGETPLATFORM),$(lastword $(subst /, ,$(TARGETPLATFORM))),amd64)
ARCH=arm64

CONFIG = $(shell cat config.yaml | base64 | tr -d "\n")
CONFIG_DEV = $(shell cat config-dev.yaml | base64 | tr -d "\n")

# Multiple extensions tried by Proximity
SETTINGS_PATH = /.config/proximity/settings
SETTINGS_PATH_DEV = /.config/proximity/settings-dev

VERSION_URL = "https://statlas.prod.atl-paas.net/vportella/proximity/version.json"

BUILD_LD_FLAGS_COMMON = -X 'main.Version=$(VERSION)' \
	-X 'bitbucket.org/atlassian-developers/proximity/internal/update.versionUrl=$(VERSION_URL)'

BUILD_LD_FLAGS     = $(BUILD_LD_FLAGS_COMMON) -X 'main.Config=$(CONFIG)' -X 'main.Port=29576' -X 'main.SettingsPath=$(SETTINGS_PATH)'
BUILD_LD_FLAGS_DEV = $(BUILD_LD_FLAGS_COMMON) -X 'main.Config=$(CONFIG)' -X 'main.Port=29575' -X 'main.SettingsPath=$(SETTINGS_PATH_DEV)'

vendor:
	go env -w GOPRIVATE="*.atlassian.com,bitbucket.org/observability,bitbucket.org/atlassian,bitbucket.org/hipchat"
	go mod vendor

run:
	wails dev -skipbindings -ldflags "$(BUILD_LD_FLAGS_DEV)"

build-app:
	wails build -skipbindings -clean -platform darwin/$(ARCH) -ldflags "$(BUILD_LD_FLAGS)"

package: build-app
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

	rm version.json
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

test:
	go test -cover ./internal/...

###################################################################################################

# TOP gives us the root of the repository
TOP  = $(shell git rev-parse --show-toplevel)

# BIN is the location of generated files
BIN  = $(TOP)/bin

# DIST is the location of files for distribution
DIST = $(BIN)/dist

GOARCH      ?= $(shell go env GOARCH)
GOOS        ?= $(shell go env GOOS)
GOPATH      ?= $(shell go env GOPATH)
GOEXE       ?= $(shell GOOS=$(GOOS) go env GOEXE)

###################################################################################################

VERSION    ?= $(shell git describe --tags 2>/dev/null || echo "dev")
BUILD_DATE = $(shell date +%Y%m%d.%H%M)
# Hardcoded to avoid `go list` which parses all .go files and validates embed directives
PACKAGE    = bitbucket.org/atlassian-developers/proximity
NAME       = proximity
EXEC       = proximity$(GOEXE)
BUNDLE     = $(NAME)-$(VERSION)-$(GOOS)-$(GOARCH).tar.gz
BUNDLE_SHA = $(BUNDLE).sha256

BIN_DIR     = $(TOP)/bin/$(GOOS)-$(GOARCH)

PLUGIN_PLATFORMS    = darwin-amd64 darwin-arm64
PLUGIN_DIST         = $(DIST)/$(NAME)
PLUGIN_RELEASE      = $(PLUGIN_DIST)/$(VERSION)

PLUGIN_EXECUTABLE   = $(TOP)/bin/$(GOOS)-$(GOARCH)/$(EXEC)

MANIFEST            = manifest.toml
PLUGIN_MANIFEST     = $(PLUGIN_DIST)/manifest.toml
PLUGIN_MANIFEST_SHA = $(PLUGIN_MANIFEST).sha256

PLUGIN_BUNDLE       = $(PLUGIN_RELEASE)/$(GOOS)-$(GOARCH).tar.gz
PLUGIN_BUNDLE_SHA   = $(PLUGIN_BUNDLE).sha256

PLUGIN_REPOSITORY   = https://statlas.prod.atl-paas.net/atlas-cli-plugin-proximity

PLUGIN_BUNDLES      = $(addsuffix .tar.gz, $(PLUGIN_PLATFORMS))
PLUGIN_MANIFESTS    = $(addsuffix .sha256, $(PLUGIN_BUNDLES))

PLUGIN_RELEASE_BUNDLE = $(PLUGIN_RELEASE).tar.gz

###################################################################################################

$(BIN):
	mkdir -p $@

$(DIST):
	mkdir -p $@

###################################################################################################

$(PLUGIN_RELEASE):
	mkdir -p $@

$(BIN_DIR):
	mkdir -p $@

$(BIN_DIR)/$(EXEC): $(BIN_DIR)
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=0 go build -ldflags "-X 'main.Version=$(VERSION)'" -o $@ ./cmd/main.go

.PHONY: build
build: $(BIN_DIR)/$(EXEC)

###################################################################################################

$(BUNDLE): build
	tar -C $(BIN_DIR) -czf $(BUNDLE) $(EXEC)

$(BUNDLE_SHA): $(BUNDLE)
	openssl sha256 -hex -r < $(BUNDLE) | cut -f 1 -d " " > $(BUNDLE_SHA)

.PHONY: bundle
bundle: $(BUNDLE) $(BUNDLE_SHA)
###################################################################################################

manifest.toml.sha256: manifest.toml
	openssl sha256 -hex -r < $^ | cut -f 1 -d " " > $@

$(PLUGIN_MANIFEST): $(PLUGIN_RELEASE) manifest.toml
	cp manifest.toml $(PLUGIN_MANIFEST)

$(PLUGIN_MANIFEST_SHA): $(PLUGIN_MANIFEST)
	openssl sha256 -hex -r < $(PLUGIN_MANIFEST) | cut -f 1 -d " " > $(PLUGIN_MANIFEST_SHA)

$(PLUGIN_BUNDLE): $(PLUGIN_RELEASE) $(PLUGIN_EXECUTABLE)
	tar -C $(dir $(PLUGIN_EXECUTABLE)) -czf $@ $(EXEC)

$(PLUGIN_BUNDLE_SHA): $(PLUGIN_BUNDLE)
	openssl sha256 -hex -r < $(PLUGIN_BUNDLE) | cut -f 1 -d " " > $(PLUGIN_BUNDLE_SHA)


###################################################################################################

.PHONY: $(PLUGIN_BUNDLES)

darwin-amd64.tar.gz: GOOS=darwin
darwin-amd64.tar.gz: GOARCH=amd64
darwin-amd64.tar.gz:
	GOOS=$(GOOS) GOARCH=$(GOARCH) $(MAKE) bundle

darwin-arm64.tar.gz: GOOS=darwin
darwin-arm64.tar.gz: GOARCH=arm64
darwin-arm64.tar.gz:
	GOOS=$(GOOS) GOARCH=$(GOARCH) $(MAKE) bundle

###################################################################################################

.PHONY: bundle bundle-all bundle-release

bundle: $(PLUGIN_BUNDLE) $(PLUGIN_BUNDLE_SHA)

bundle-all: $(PLUGIN_BUNDLES)

$(PLUGIN_RELEASE_BUNDLE): bundle-all
	tar -C $(dir $(PLUGIN_RELEASE)) -czf $@ $(addprefix $(VERSION)/,$(PLUGIN_BUNDLES)) $(addprefix $(VERSION)/,$(PLUGIN_MANIFESTS))

bundle-release: $(PLUGIN_RELEASE_BUNDLE)

###################################################################################################

.PHONY: manifest

manifest: $(PLUGIN_MANIFEST) $(PLUGIN_MANIFEST_SHA)

###################################################################################################

.PHONY: release

release: bundle-release

###################################################################################################

.PHONY: publish-manifest

publish-manifest: check-token | manifest
	curl -X PUT -H 'authorization: bearer $(bamboo_JWT_TOKEN)' -T $(PLUGIN_MANIFEST) $(PLUGIN_REPOSITORY)/$(notdir $(PLUGIN_MANIFEST))
	curl -X PUT -H 'authorization: bearer $(bamboo_JWT_TOKEN)' -T $(PLUGIN_MANIFEST_SHA) $(PLUGIN_REPOSITORY)/$(notdir $(PLUGIN_MANIFEST_SHA))

publish-release: check-token | bundle-release
	curl -X POST -H 'authorization: bearer $(bamboo_JWT_TOKEN)' -T $(PLUGIN_RELEASE_BUNDLE) $(PLUGIN_REPOSITORY)/releases

.PHONY: check-token
check-token:
	@if [ -z "$(bamboo_JWT_TOKEN)" ]; then \
		(>&2 echo "error: missing envar bamboo_JWT_TOKEN");\
		exit 1; \
	fi

###################################################################################################

.PHONY: bump-manifest update-manifest commit-manifest
bump-manifest:
	python2 $(TOP)/scripts/bump-manifest.py --version $(VERSION) --manifest manifest.toml

commit-manifest:
	git remote rm actual || true
	git add manifest.toml
	git commit -m "Bump #manifest to version $(VERSION)"
	[ `git rev-parse --abbrev-ref HEAD` = master ] && \
	git remote add actual ${bamboo_planRepository_1_repositoryUrl} && \
	git push actual master:master && \
	git push --tags actual master:master

update-manifest: bump-manifest publish-manifest commit-manifest
