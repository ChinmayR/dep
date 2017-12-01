HOMEBREW_TARGETS ?= $(PROGS)
PROJECT_DIR = $(CURDIR)
BOTTLE_NAME ?= $(shell basename $(PROJECT_ROOT))
BOTTLE_TEMPLATE_FILE ?= $(PROJECT_DIR)/bottle.tmpl
MACOS_VERSIONS = yosemite el_capitan sierra
BOTTLE_VERSION := $(shell git --git-dir=$(PROJECT_DIR)/.git describe --dirty --always --tags)
BOTTLE_ROOT := .tmp/bottle/
BOTTLE_PATH := $(BOTTLE_ROOT)/$(BOTTLE_NAME)/$(BOTTLE_VERSION)
BOTTLE_JSON := $(BOTTLE_PATH)/INSTALL_RECEIPT.json
BOTTLE_README := $(BOTTLE_PATH)/README.md
BOTTLE_TGZS := $(foreach os, $(MACOS_VERSIONS), $(BOTTLE_ROOT)/$(BOTTLE_NAME)-$(BOTTLE_VERSION).$(os).bottle.tar.gz)
INSTALL_RECEIPT_TEMPLATE = $(DEPS_GOPATH)/bottler/bottle_json_template.json

ci-bottle: clean bottles upload-bottles $(HOMEBREW_BOTTLER)
	$(HOMEBREW_BOTTLER) $(BOTTLE_TEMPLATE_FILE) $(PROJECT_DIR)/.tmp/bottle

# need to find a way to make this work on loonix
test-bottle:
	brew install $(BOTTLE_NAME).rb
	brew reinstall $(BOTTLE_NAME)

bottles: $(BOTTLE_TGZS)

bins-osx: $(BOTTLE_PATH)/bin
	@$(MAKE) thriftc vendor
	@$(MAKE) $(GO_BINDATA)
	@GOOS=darwin GOARCH=amd64 $(MAKE) $(HOMEBREW_TARGETS)
	cp $(PROJECT_DIR)/$(HOMEBREW_TARGETS) $(BOTTLE_PATH)/bin/

$(BOTTLE_TGZS): bins-osx $(BOTTLE_JSON) $(BOTTLE_README) $(BOTTLE_BINS)
	pushd $(BOTTLE_PATH)/../.. ; \
	tar cvfz $(CURDIR)/$@ --exclude "*.bottle.tar.gz" $(BOTTLE_NAME)/* ; \
	popd

$(BOTTLE_PATH)/bin:
	mkdir -p $@

$(BOTTLE_JSON): $(BOTTLE_PATH)/bin
	printf '$(shell cat $(INSTALL_RECEIPT_TEMPLATE))' $(shell date "+%s") > $(BOTTLE_JSON)

$(BOTTLE_README): $(BOTTLE_PATH)/bin
	cp README.md $(BOTTLE_README)

upload-bottles: bottles
	for tgz in $(BOTTLE_TGZS) ; do \
		aws s3 cp $$tgz s3://uber-devtools/$(BOTTLE_NAME)/ --acl public-read ; \
	done

.PHONY: bottles bins-osx ci-bottle test-bottle upload-bottles
