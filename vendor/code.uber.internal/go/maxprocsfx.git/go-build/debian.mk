# debian.mk sets up targets that are useful for Debian builds.
# For a debian build, use the generic-debian Jenkins flow
# and set "make debian-pre" as the PRE_BUILD_COMMAND.
# User-overridable variables:
#   VERSION_FILE: Defaults to version.git, rule that includes the version information.

VERSION_FILE ?= version.git

.PHONY: $(VERSION_FILE)
$(VERSION_FILE):
	rm -f $@
	[ -z "$(BUILD_NUMBER)" ] || echo -n "$(BUILD_NUMBER)." > $@
	git describe --always --tags --dirty 2>/dev/null >> $@

# Set the BUILD_HASH if it's unset and we find a VERSION_FILE
ifneq ("$(wildcard $(VERSION_FILE))","")
  BUILD_HASH ?= $(shell cat $(VERSION_FILE))
endif

# Debian builds are done in a chroot without the .git folder and without
# configs to access our internal repositories.
# Before we enter the chroot, we extract version information from the Git
# repository, install all dependencies and check in these files.
.PHONY: debian-pre
debian-pre:: ## Checks in version file and depdendencies before running the Debian build inside of the chroot.
	$(MAKE) clean
	rm -rf vendor
	$(MAKE) vendor $(VERSION_FILE)
	make thriftc protoc
	rm -rf glide.yaml $(shell find vendor -iname .git)
	git add -f $(VERSION_FILE) vendor $(shell test -d .gen/go && echo .gen/go )
	git commit -m 'Add version and dependencies'
