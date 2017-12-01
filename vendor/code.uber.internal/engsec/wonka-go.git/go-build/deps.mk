# deps.mk sets up targets for the build tools which are built
# from source in the go-build repo.
# The build tools use the $(GO_BUILD_ROOT)/.go as the GOPATH
# which follows a standard GOPATH structure.
# We do not use the fake GOPATH as we do not want the build tools
# to be rebuilt when the user calls "make clean".

# DEPS_PACKAGES defines the binaries that must be built from scratch
# and the make variable that will be set up to the binary.
DEPS_PACKAGES = \
	ERRCHECK:github.com/kisielk/errcheck \
	UNUSED:honnef.co/go/tools/cmd/unused \
	FFJSON:github.com/pquerna/ffjson \
	GLIDE:github.com/Masterminds/glide \
	GO_BINDATA:github.com/jteeuwen/go-bindata/go-bindata \
	GOCOV:github.com/axw/gocov/gocov \
	GOCOV_XML:github.com/AlekSi/gocov-xml \
	GOCOV_HTML:github.com/matm/gocov-html \
	GODEP:github.com/tools/godep \
	GOLINT:github.com/golang/lint/golint \
	JUNIT_REPORT:github.com/jstemmer/go-junit-report \
	MOCKERY:github.com/vektra/mockery/cmd/mockery \
	MOCKGEN:github.com/golang/mock/mockgen \
	STATICCHECK:honnef.co/go/tools/cmd/staticcheck \
	THRIFT_GEN:github.com/uber/tchannel-go/thrift/thrift-gen \
	HOMEBREW_BOTTLER:code.uber.internal/devexp/homebrew_bottler \
	GLIDE_EXEC:code.uber.internal/infra/glide-exec \
	GOCOVMERGE:github.com/wadey/gocovmerge \
	PROTOC_GEN_GO:github.com/golang/protobuf/protoc-gen-go \
	PROTOC_GEN_GOGOFASTER:github.com/gogo/protobuf/protoc-gen-gogofaster \
	PROTOC_GEN_GOGOSLICK:github.com/gogo/protobuf/protoc-gen-gogoslick \
	GO_BUILD_PROTOC_HELPER:code.uber.internal/infra/prototools.git/cmd/go-build-protoc-helper

# The GOPATH used when building the build path.
DEPS_GOROOT := $(CURDIR)/$(GO_BUILD_ROOT)/.go
DEPS_GOPATH := $(DEPS_GOROOT)/src/gb2

_GO_DEPS_ARCH := $(shell echo "$(shell uname -s)-$(shell uname -m)" | tr '[A-Z]' '[a-z]')

# The directory where the build tools are built to.
GO_DEPS_BIN := $(GO_BUILD_ROOT)/.go/bin/$(_GO_DEPS_ARCH)

# A variable containins all the build tools (only used for internal rules).
DEPS_PROGS =

# deprule takes 2 arguments:
# 1: The variable to define for the binary
# 2: The package to build.
define deprule
$1 := $(GO_DEPS_BIN)/$(notdir $2)
DEPS_PROGS += $$($1)
$$($1): $(DEPS_GOPATH)/glide.lock $(wildcard $(DEPS_GOPATH)/vendor/$2/*.go)
	$(ECHO_V)GOPATH=$(DEPS_GOPATH) $(GO) build -i -o $$@ $2
endef

# Define variables for each binary in DEPS_PACKAGES, and set up rules
# to build them.
$(foreach deppkg,$(DEPS_PACKAGES), $(eval $(call deprule \
	,$(firstword $(subst :, ,$(deppkg))),$(lastword $(subst :, ,$(deppkg))))))

# Python utf8strings flag not respected by the C-based implementation of the binary protocol
# https://issues.apache.org/jira/browse/THRIFT-1229
THRIFT_PY = $(GO_DEPS_BIN)/thrift_py
THRIFT_PY_VERSIONED = $(GO_DEPS_BIN)/thrift_py-1
THRIFT = $(GO_DEPS_BIN)/thrift
THRIFT_VERSIONED = $(GO_DEPS_BIN)/thrift-1
# The thrift version is set in this file, so when it changes, we need to
# redownload the Thrift file.
$(THRIFT_VERSIONED): $(CURDIR)/$(GO_BUILD_ROOT)/deps.mk
	$(ECHO_V)$(CURDIR)/$(GO_BUILD_ROOT)/s3-downloader $(notdir $(THRIFT)) $(notdir $(THRIFT_VERSIONED))
$(THRIFT_PY_VERSIONED): $(CURDIR)/$(GO_BUILD_ROOT)/deps.mk
	$(ECHO_V)$(CURDIR)/$(GO_BUILD_ROOT)/s3-downloader $(notdir $(THRIFT_PY)) $(notdir $(THRIFT_PY_VERSIONED))

$(THRIFT): $(THRIFT_VERSIONED)
	$(ECHO_V)cp $< $@
$(THRIFT_PY): $(THRIFT_PY_VERSIONED)
	$(ECHO_V)cp $< $@

# Protobuf compiler
PROTOC = $(GO_DEPS_BIN)/protoc
PROTOC_VERSIONED = $(GO_DEPS_BIN)/protoc-3.3.0
$(PROTOC_VERSIONED): $(CURDIR)/$(GO_BUILD_ROOT)/deps.mk
	$(ECHO_V)$(CURDIR)/$(GO_BUILD_ROOT)/s3-downloader $(notdir $(PROTOC)) $(notdir $(PROTOC_VERSIONED))

$(PROTOC): $(PROTOC_VERSIONED)
	cp $< $@

# Internal beta versions of glide patches
GLIDE_BETA = $(GO_DEPS_BIN)/glide-beta
GLIDE_BETA_VERSIONED = $(GO_DEPS_BIN)/glide-beta-68dcb39
$(GLIDE_BETA_VERSIONED): $(CURDIR)/$(GO_BUILD_ROOT)/deps.mk
	$(ECHO_V)$(CURDIR)/$(GO_BUILD_ROOT)/s3-downloader $(notdir $(GLIDE_BETA)) $(notdir $(GLIDE_BETA_VERSIONED))

$(GLIDE_BETA): $(GLIDE_BETA_VERSIONED)
	cp $< $@

# List out all of the executables, so rules like `make clean` are correct
DEPS_PROGS += $(THRIFT) $(THRIFT_VERSIONED) $(THRIFT_PY) $(THRIFT_PY_VERSIONED) \
	$(PROTOC) $(PROTOC_VERSIONED) $(GLIDE_BETA) $(GLIDE_BETA_VERSIONED)

.PHONY: gobuild-clean
gobuild-clean: ## Internal: Remove build tools (such as glide, golint, etc).
	rm -rf $(DEPS_GOPATH)/bin $(DEPS_GOPATH)/pkg $(DEPS_GOROOT)/bin $(DEPS_GOROOT)/pkg
	rm -rf $(DEPS_PROGS)

# Internal rule to build all build deps binaries.
# This is mostly used for ensuring that all dependencies are checked in.
.PHONY: gobuild-bins
gobuild-bins: $(DEPS_PROGS)
