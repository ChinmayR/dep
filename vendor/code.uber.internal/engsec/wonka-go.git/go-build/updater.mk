_GO_BUILD_DIR := $(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))

_GO_BUILD_VERSION := $(shell cd $(_GO_BUILD_DIR) && git describe --always)

FORCE_UPDATE ?= 0

.PHONY: update-gobuild
update-gobuild: ## Updates go-build to the latest version
	$(ECHO_V)echo "Updating $(_GO_BUILD_DIR) to latest HEAD of master from $(_GO_BUILD_VERSION)"
	$(ECHO_V)[ $(FORCE_UPDATE) -eq 1 -o -z "$$(git status --porcelain)" ] || (echo "Uncommitted changes, please stash or commit" && exit 1)
	$(ECHO_V)cd $(_GO_BUILD_DIR) && \
		git fetch && \
		git reset --hard origin/master
	$(ECHO_V)git checkout -b update-go-build
	$(ECHO_V)git add $(_GO_BUILD_DIR)
	$(ECHO_V)COMMIT_MESSAGE_TEMP=$(shell mktemp) && \
		echo "Update go-build to $$(cd $(_GO_BUILD_DIR) && git describe --always)" >> $$COMMIT_MESSAGE_TEMP && \
		echo >> $$COMMIT_MESSAGE_TEMP && \
		echo "Upstream changes:" >> $$COMMIT_MESSAGE_TEMP && \
		echo >> $$COMMIT_MESSAGE_TEMP && \
		$$(cd $(_GO_BUILD_DIR) && git log --pretty='  * %s' $(_GO_BUILD_VERSION)..HEAD >> $$COMMIT_MESSAGE_TEMP) && \
		cat $$COMMIT_MESSAGE_TEMP && \
		git commit --file $$COMMIT_MESSAGE_TEMP && \
		rm $$COMMIT_MESSAGE_TEMP
