#
# Go build rules.
#
# If included, your Makefile looks something like
# this:
#
# # BEGIN EXAMPLE
#
# PROJECT_ROOT = code.uber.internal/path/to/project
#
# # define the list of thrift files the service depends on
# THRIFT_SRCS = thrift-repo/services/myservice1.thrift \
#		thrift-repo/shared/meta.thrift
#
# # list all executables to be built for deployment
# PROGS = myservice1 myotherservice
#
# # add any extra binaries that will be `make`-able but not built by default
# EXTRA_PROGS = example/client
#
# Other bins targets that are NOT go build targets
# EXTRA_BINS = other-bins-deps
#
# # define which go files cause a rebuild of which binary
# myservice1: appcfg/config.go handler.go main.go
# example/client: appcfg/config.go $(wildcard client/*.go)
#
# include go-build/rules.mk
#
# go-build/rules.mk:
#	git submodule update --init
#
# # END EXAMPLE
#
# Make sure the include statement is at the BOTTOM of
# your Makefile!
#
# All GNU Make globbing/file operations are permitted for
# defining what .go files binaries/packages depend on.  See
#
# http://t.uber.com/ckqzw
# and
# http://t.uber.com/wtmab
#
# for more functions
#

space :=
space +=
ifneq (,$(findstring $(space),$(CURDIR)))
$(error Golang project directory $(CURDIR) contains space it is unsupported by go-build, please rename)
endif

SHELL = /bin/bash -o pipefail

GOBUILD_DIR := $(patsubst %/,%,$(dir $(lastword $(MAKEFILE_LIST))))

# include.mk must always come first.
include $(GOBUILD_DIR)/include.mk

include $(GOBUILD_DIR)/colors.mk
include $(GOBUILD_DIR)/debian.mk
include $(GOBUILD_DIR)/dep.mk
include $(GOBUILD_DIR)/glide.mk
include $(GOBUILD_DIR)/thriftrw.mk
include $(GOBUILD_DIR)/updater.mk

THRIFT_GENDIR ?= .gen
THRIFTRW_GENDIR ?= $(THRIFT_GENDIR)
PROTOC_GENDIR ?= .gen/proto/go
SERVICES ?= $(subst .git,,$(notdir $(PROJECT_ROOT)))

# Names of all protoc plugins specified by the user.
PROTOC_PLUGIN_NAMES = $(patsubst protoc-gen-%, %, $(notdir $(PROTOC_PLUGINS)))

BUILD_DIR ?= .tmp

# _GOPATH is the new GOPATH for the project
_GOPATH = $(CURDIR)/$(BUILD_DIR)/.goroot

# OLDGOPATH is the original GOPATH, even in sub make
ifeq ($(OLDGOPATH),)
OLDGOPATH := $(GOPATH)
export OLDGOPATH
endif

# you can't have Make depend on a directory as the tstamp is updated
# when a file is written to it.  Instead, touch a file that says you
# created the directory
FAUXFILE := $(BUILD_DIR)/.faux
FAUXROOT := $(_GOPATH)/src/$(PROJECT_ROOT)

# clang supports more features on precise than gcc
ifeq ($(shell uname),Linux)
	ifeq ($(shell lsb_release -cs),precise)
		export CC=clang
	endif
endif

# Test for a broken root symlink, which can happen when syncing projects
# across machines/VMs. -h tests for a symlink, ! -e means the file does not exist.
ifeq ($(shell [ -h $(FAUXROOT) ] && [ ! -e $(FAUXROOT) ] && echo "b"),b)
$(warning Broken root detecting, cleaning symlink)
$(shell unlink $(FAUXROOT))
$(shell rm $(FAUXFILE))
endif

# Sentinel file to detect whether dependencies are installed
FAUX_VENDOR := $(BUILD_DIR)/.faux.vendor

# Sentinel file to represent all Thrift outputs
FAUX_GENCODE := $(BUILD_DIR)/.faux.gencode
FAUX_THRIFT := $(BUILD_DIR)/.faux.thrift
FAUX_THRIFTRW := $(BUILD_DIR)/.faux.thriftrw
FAUX_THRIFTGEN := $(BUILD_DIR)/.faux.thriftgen

FAUX_PROTOC := $(BUILD_DIR)/.faux.protoc

GOPATH := $(_GOPATH)

# so people in their makefiles can depend on vendor
$(VENDOR_DIR): $(FAUX_VENDOR)

# If Gopkg.toml exists, assume they are using dep. Otherwise, assume glide.
DEP_MANAGER = ""
ifeq ($(shell ( [ -f $(DEP_TOML) ]) && echo y),y)
	# Email dep-support-group@uber for issues regarding dep
	DEP_MANAGER = dep
else
	DEP_MANAGER = glide
endif

$(FAUX_VENDOR):: $(FAUXFILE)

ifeq ($(DEP_MANAGER),glide)
$(GLIDE_LOCK):
	@[ ! -f $(GLIDE_YAML) ] || (echo "$(GLIDE_YAML) present, but missing $(GLIDE_LOCK) file. Make sure it's checked in." && exit 1)

# allows to override the verb from `install` to `upgrade`, if building from live if preferable
GLIDE_VERB ?= install
$(FAUX_VENDOR):: $(FAUXFILE) $(GLIDE) $(GLIDE_LOCK)
	@# Retry the glide install a few times to avoid flaky Jenkins failures
	@[ ! -f $(GLIDE_YAML) ] || (cd $(FAUXROOT) && \
	    for i in 1 2 3; do \
		    $(GLIDE_ENV) $(GLIDE) $(GLIDE_NO_COLOR) $(GLIDE_VERB) && break; \
		    $(GLIDE) cache-clear; \
	    done)
	@touch $@

else
$(DEP_LOCK):
	@[ ! -f $(DEP_TOML) ] || (echo "$(DEP_TOML) present, but missing $(DEP_LOCK) file. Make sure it's checked in." && exit 1)

DEP_VERB ?= ensure
$(FAUX_VENDOR):: $(FAUXFILE) $(DEP) $(DEP_LOCK)
	@# Retry the dep ensure a few times to avoid flaky Jenkins failures
	@[ ! -f $(DEP_TOML) ] || (cd $(FAUXROOT) && \
	    for i in 1 2 3; do \
		    $(DEP_ENV) $(DEP) $(DEP_VERB) -v && break; \
	    done)
	@touch $@
endif

export GOPATH

# FAUXTEST is used as a dependency for tests to ensure that we build
# dependencies once (using go test -i) before trying to run each test.
# Otherwise, multiple go test invocations will not save any build output
# and dependencies are recompiled multiple times.
FAUXTEST := $(BUILD_DIR)/faux.test

# These are flags that can be overridden by the including Makefile

# Strip out the fake gopath by default. Makes stacktraces consistent across dev/prod/jenkins.
BUILD_GC_FLAGS ?= -gcflags "-trimpath=$(GOPATH)/src"
BUILD_FLAGS ?=
BUILD_ENV ?=
FMT_FLAGS ?= -s
JENKINS_PARALLEL_FLAG ?=-j5

# Set this flag to 0 in your Makefile if you're using ginkgo, whose test log output
# will cause the go-junit-reporter to think your tests are failed (since they don't conform
# to the stdlib testing stuff like testify/assert does)
ENABLE_JUNIT_XML ?= 1

# Pass linker flags to set build version info
version_pkg = $(PROJECT_ROOT)/vendor/code.uber.internal/go-common.git/version
new_version_pkg = $(PROJECT_ROOT)/vendor/code.uber.internal/go/version.git
BUILD_VARS += vpkg=$(version_pkg)
BUILD_VARS += nvpkg=$(new_version_pkg)
BUILD_LDFLAGS ?=
build_host = $(shell hostname | tr -C -d '[0-9a-zA-Z]')

ifneq ($(GIT_DESCRIBE),)
  BUILD_HASH := $(GIT_DESCRIBE)
endif
# Use the same `git describe` command as uBuild.
BUILD_HASH ?= $(shell git describe --always --tags --dirty 2>/dev/null)

BUILD_VERSION_FLAGS := -ldflags "\
	$(BUILD_LDFLAGS) \
	-X $$vpkg.BuildHash=$(BUILD_HASH) \
	-X $$vpkg.buildUnixSeconds=$(shell date +%s) \
	-X $$vpkg.BuildUserHost=$(USER)@$(build_host) \
	-X $$nvpkg.BuildHash=$(BUILD_HASH) \
	-X $$nvpkg.buildUnixSeconds=$(shell date +%s) \
	-X $$nvpkg.BuildUserHost=$(USER)@$(build_host) \
	"

TEST_ENV ?= UBER_ENVIRONMENT=test UBER_CONFIG_DIR=$(abspath $(FAUXROOT))/config


# TEST_PKG_TIMEOUT is the -timeout flag passed to go test. This timeout applies
# to a single package, after which go will kill the test and dump stacktraces.
TEST_PKG_TIMEOUT ?= 1m

# Prefix with a space so that our duplicate detection below doesn't
# miss flags specified at the beginning.
TEST_FLAGS := $(space)$(TEST_FLAGS) $(BUILD_GC_FLAGS)

# Add in flags to TEST_FLAGS that shouldn't be duplicated, like -race
ifeq (,$(findstring $(space)$(RACE), $(TEST_FLAGS)))
	TEST_FLAGS += $(RACE)
endif
ifeq (,$(findstring $(space)$(TEST_VERBOSITY_FLAG), $(TEST_FLAGS)))
	TEST_FLAGS += $(TEST_VERBOSITY_FLAG)
endif
ifeq (,$(findstring $(space)-timeout, $(TEST_FLAGS)))
	TEST_FLAGS += $(space)-timeout $(TEST_PKG_TIMEOUT)
endif

# BENCH_FLAGS are the flags to the benchmark tests. Note that -benchmem
# is already set.
BENCH_FLAGS ?=

# Any other test dependencies necessary to build the binaries.
TEST_DEPS ?=

# Any other benchmark dependencies necessary to build the binaries.
BENCH_DEPS ?=

# Any other dependencies necessary to build the binaries.
BIN_DEPS ?=

# mock files will be removed from test coverage reports
COVER_IGNORE_SRCS ?= _mock\.go _string\.go mocks/.*\.go mock_.*\.go

# mysqlman rules
ifdef MYSQLMAN_DB_NAME
	include $(GOBUILD_DIR)/mysqlman.mk
endif

# schemaless/lessman rules
ifdef SCHEMALESS_INSTANCE
	include $(GOBUILD_DIR)/lessman.mk
endif

# source of truth for thrift includes
THRIFTINC = "github.com/apache/thrift/lib/go/thrift"

THRIFT_GOFLAGS = -r --gen go:thrift_import=$(THRIFTINC)
THRIFT_PYFLAGS = -r --gen py:utf8strings
THRIFT_JSFLAGS = -r --gen js:node

THRIFT_TEMPLATES := $(addprefix $(PROJECT_ROOT)/vendor/,$(THRIFT_TEMPLATES))

# Test coverage target files
PHAB_COMMENT ?= /dev/null
HTML_REPORT := coverage.html
COVERAGE_XML := coverage.xml
JUNIT_XML := junit.xml
COVERFILE := $(BUILD_DIR)/cover.out
CHECK_COVERAGE_LOG := $(BUILD_DIR)/check_coverage.out
TEST_LOG := $(BUILD_DIR)/test.log
LINT_LOG := $(BUILD_DIR)/lint.log
FMT_LOG := $(BUILD_DIR)/fmt.log
VET_LOG := $(BUILD_DIR)/vet.log
ERRCHECK_LOG := $(BUILD_DIR)/errcheck.log
UNUSED_LOG := $(BUILD_DIR)/unused.log
STATICCHECK_LOG := $(BUILD_DIR)/staticcheck.log

# all .go files that don't exist in hidden directories
ALL_SRC := $(shell find . -name "*.go" -path "*$(PKG)*" | grep -v -e Godeps -e vendor -e go-build \
	-e ".*/\..*" \
	-e ".*/_.*" \
	-e ".*/testdata/.*" \
	-e ".*/node_modules/.*" \
	-e ".*/mocks.*")

# !!!IMPORTANT!!!  This must be = and not := as this rule must be delayed until
# after FAUXFILE has been executed.  Only use this in rules that depend on
# FAUXFILE!
ALL_PKGS = $(filter-out %/testdata _/% %/_% %/.%, $(shell (cd $(FAUXROOT); \
	   GOPATH=$(GOPATH) $(GO) list $(sort $(dir $(ALL_SRC))))))

# all directories with *_test.go files in them
TEST_DIRS := $(sort $(dir $(filter %_test.go,$(ALL_SRC))))

# filter TEST_DIRS by TEST_DIR
ifneq ($(TEST_DIR),)
TEST_DIRS := $(filter $(TEST_DIR), $(TEST_DIRS))
endif

# profile.cov targets for test coverage
TEST_PROFILE_TGTS := $(addprefix $(BUILD_DIR)/, $(addsuffix profile.cov, $(TEST_DIRS)))
TEST_PROFILE_TMP_TGTS := $(addsuffix .tmp,$(TEST_PROFILE_TGTS))
TEST_TMP_LOGS := $(addprefix $(BUILD_DIR)/, $(addsuffix test.log.tmp, $(TEST_DIRS)))

# Generates a make target for every directory in the project where there could be
# benchmark tests in.
BENCHMARK_PACKAGE_TGTS := $(addprefix BENCHMARK/, $(TEST_DIRS))

# default make objective
all: bins test


# we template this out for every thrift file since the destination
# loop over all THRIFT_SRCS and add new actions to FAUX_THRIFTGEN.
define thriftrule

$(FAUX_THRIFTGEN):: $1 $(FAUXFILE) $(THRIFT) $(THRIFT_GEN) $(THRIFT_PY)
	@mkdir -p $(THRIFT_GENDIR)/go $(THRIFT_GENDIR)/js $(THRIFT_GENDIR)/py
	$(ECHO_V)$(THRIFT_PY) $(THRIFT_PYFLAGS) -out $(THRIFT_GENDIR)/py $$<
	@echo $(THRIFTC_LABEL) $(notdir $1)
	$(ECHO_V)$(THRIFT) $(THRIFT_JSFLAGS) -out $(THRIFT_GENDIR)/js $$<
	$(ECHO_V)$(THRIFT_GEN) --generateThrift --packagePrefix $(PROJECT_ROOT)/$(THRIFT_GENDIR)/go/ --inputFile $1 --outputDir $(THRIFT_GENDIR)/go \
		$(foreach template,$(THRIFT_TEMPLATES), --template $(template))
endef

$(foreach tsrc,$(THRIFT_SRCS),$(eval $(call thriftrule,$(tsrc))))
# Touch the faux file last after everything else is successful.
$(FAUX_THRIFTGEN):: $(FAUXFILE) $(THRIFT_SRCS)
	@touch $@


define thriftrwrule

$(FAUX_THRIFTRW):: $1 $(GLIDE) $(GLIDE_EXEC) $(FAUX_VENDOR)
	@echo $(THRIFTRW_LABEL) $(notdir $1)
	@mkdir -p $(_GOPATH)/bin
	$(ECHO_V)$(GLIDE_EXEC) --glide $(GLIDE) --bin $(_GOPATH)/bin \
		--exe $(THRIFTRW_IMPORT_PATH) $(foreach p, $(THRIFTRW_PLUGINS), --exe $(p)) \
		$(notdir $(THRIFTRW_IMPORT_PATH)) \
			--out $(THRIFTRW_GENDIR)/go --thrift-root $(THRIFTRW_SRCS_ROOT) \
			--pkg-prefix $(PROJECT_ROOT)/$(THRIFTRW_GENDIR)/go \
			$(foreach p, $(THRIFTRW_PLUGIN_NAMES), --plugin $(p)) $$<
endef

$(foreach tsrc,$(THRIFTRW_SRCS),$(eval $(call thriftrwrule,$(tsrc))))
# Touch the faux file last after everything else is successful.
$(FAUX_THRIFTRW):: $(FAUXFILE) $(THRIFTRW_SRCS)
	@touch $@

# Make on Linux has an issue when wildcard rules depend on a double-colon rule
# directly. We want users to be able to add extra rules after Thrift code
# generation using "$(FAUX_THRIFT)::" but other targets cannot depend on
# this double-colon rule, so instead we add a layer of indirection.
$(FAUX_GENCODE): $(FAUXFILE) $(FAUX_THRIFTRW) $(FAUX_THRIFTGEN) $(FAUX_THRIFT) $(FAUX_PROTOC)
	@touch $@

$(FAUX_THRIFT):: $(FAUXFILE) $(FAUX_THRIFTRW) $(FAUX_THRIFTGEN)
	@touch $@

# See README.md for Protocol Buffers usage.

# Do not allow users to set PROTOC_EXTRA_MODIFIERS for now
# If this feature is later needed, this can be deleted and it will be enabled
PROTOC_EXTRA_MODIFIERS :=

ifeq ($(PROTOC_INCLUDE_GOGO_WELL_KNOWN_TYPES),true)
PROTOC_INCLUDES := $(PROTOC_INCLUDES) go-build/.go/src/gb2/vendor/github.com/gogo/protobuf/protobuf
PROTOC_EXTRA_MODIFIERS := $(PROTOC_EXTRA_MODIFIERS) \
  google/protobuf/any.proto=github.com/gogo/protobuf/types \
  google/protobuf/duration.proto=github.com/gogo/protobuf/types \
  google/protobuf/empty.proto=github.com/gogo/protobuf/types \
  google/protobuf/field_mask.proto=github.com/gogo/protobuf/types \
  google/protobuf/struct.proto=github.com/gogo/protobuf/types \
  google/protobuf/timestamp.proto=github.com/gogo/protobuf/types \
  google/protobuf/wrappers.proto=github.com/gogo/protobuf/types
endif

ALL_PROTOC_SRCS := $(filter-out $(EXCLUDE_PROTOC_SRCS),$(shell find idl/code.uber.internal -name '*.proto' 2>/dev/null || true))

GO_BUILD_PROTOC_HELPER_FLAGS := \
	"--project-root=$(PROJECT_ROOT)" \
	"--input-dir=idl/code.uber.internal" \
	"--output-dir=$(PROTOC_GENDIR)" \
	"--extra-include=vendor" \
	$(foreach include, $(PROTOC_INCLUDES), "--extra-include=$(include)") \
	$(foreach modifier, $(PROTOC_EXTRA_MODIFIERS), "--extra-modifier=$(modifier)") \
	$(foreach exclude, $(EXCLUDE_PROTOC_SRCS), "--exclude=$(exclude)")

ifneq ($(PROTOC_INCLUDE_GRPC_GO),true)
GO_BUILD_PROTOC_HELPER_FLAGS := $(GO_BUILD_PROTOC_HELPER_FLAGS) --no-grpc
endif

ifneq ($(ECHO_V),@)
GO_BUILD_PROTOC_HELPER_FLAGS := $(GO_BUILD_PROTOC_HELPER_FLAGS) --verbose
endif

ifneq ($(ALL_PROTOC_SRCS),)
$(FAUX_PROTOC):: $(ALL_PROTOC_SRCS) $(PROTOC) $(PROTOC_GEN_GOGOSLICK) $(GO_BUILD_PROTOC_HELPER) $(GLIDE) $(GLIDE_EXEC) $(FAUX_VENDOR)
	@echo $(PROTOC_LABEL) $(ALL_PROTOC_SRCS)
	@mkdir -p $(PROTOC_GENDIR)
ifneq ($(PROTOC_PLUGINS),)
	@mkdir -p $(_GOPATH)/bin
	$(ECHO_V)PATH=$(dir $(PROTOC_GEN_GOGOSLICK)):$$PATH $(GLIDE_EXEC) --glide $(GLIDE) --bin $(_GOPATH)/bin \
		$(foreach p, $(PROTOC_PLUGINS), --exe $(p)) \
		$(GO_BUILD_PROTOC_HELPER) \
			$(GO_BUILD_PROTOC_HELPER_FLAGS) \
			$(foreach p, $(PROTOC_PLUGIN_NAMES), "--extra-go-plugin=$(p)")
else
		$(ECHO_V)PATH=$(dir $(PROTOC_GEN_GOGOSLICK)):$$PATH $(GO_BUILD_PROTOC_HELPER) \
			$(GO_BUILD_PROTOC_HELPER_FLAGS)
endif
endif

# Touch the faux file last after everything else is successful.
$(FAUX_PROTOC):: $(FAUXFILE) $(ALL_PROTOC_SRCS)
	@touch $@

$(PROGS) $(EXTRA_PROGS): $(FAUXFILE) $(FAUX_VENDOR) $(FAUX_GENCODE) $(BIN_DEPS)
	@echo $(GOBUILD_LABEL) $@
	$(ECHO_V)$(BUILD_VARS); $(BUILD_ENV) $(GO) build -o $@ $(BUILD_FLAGS) $(BUILD_GC_FLAGS) $(BUILD_VERSION_FLAGS) $(PROJECT_ROOT)/$(dir $@)

$(TEST_LOG): $(FAUXFILE) $(TEST_PROFILE_TMP_TGTS)
	@cat /dev/null $(TEST_TMP_LOGS) > $(TEST_LOG)

$(COVERFILE): $(FAUXFILE) $(FAUX_VENDOR) $(TEST_PROFILE_TGTS) $(GOCOVMERGE)
	$(GOCOVMERGE) $(TEST_PROFILE_TGTS) > $(COVERFILE)
	$(ECHO_V)if [ -n "${COVER_IGNORE_SRCS}" ]; then sed -i\"\" -E "$(foreach pattern, $(COVER_IGNORE_SRCS),\#$(pattern)#d; )" $(COVERFILE); fi


$(FAUXTEST): $(FAUXFILE) $(ALL_SRC) $(TEST_DEPS) $(FAUX_GENCODE) $(FAUX_VENDOR)
	@echo "$(COLOR_BOLD)Building all dependencies for tests$(COLOR_RESET)"
	$(ECHO_V)cd $(FAUXROOT); $(TEST_ENV)	\
		$(GO) test -i $(TEST_FLAGS) $(TEST_DIRS)
	@touch $@

# TODO remove this indirection; it's used to find if a test failed
$(BUILD_DIR)/%/profile.cov: $(BUILD_DIR)/%/profile.cov.tmp
	@([ -f $< ] && cp $< $@) || (echo "Test $1 failed" && false)

$(BUILD_DIR)/%/profile.cov.tmp: $(ALL_SRC) $(TEST_DEPS) $(FAUX_GENCODE) $(FAUXTEST)
	@mkdir -p $(dir $@)
	@touch $@
	$(ECHO_V)cd $(FAUXROOT); $(TEST_ENV)	\
		$(GO) test $(TEST_FLAGS) -coverprofile=$@ $*/  | \
		tee $(BUILD_DIR)/$*/test.log.tmp || \
			(cat $(BUILD_DIR)/$*/test.log.tmp >> $(PHAB_COMMENT) && rm -rf $@)

# arc's Go test engine creates the destination XML file before running which
# leads to a newer mtime, so use PHONY to ignore the mtime and always recreate
# the XML files.
.PHONY: $(JUNIT_XML)
ifeq ($(ENABLE_JUNIT_XML),1)

$(JUNIT_XML): $(JUNIT_REPORT) $(TEST_LOG)
	$(JUNIT_REPORT) < $(TEST_LOG) > $@

else

$(JUNIT_XML):
	@echo JUnit reporting disabled by ENABLE_JUNIT_XML=$(ENABLE_JUNIT_XML) flag

endif

# This target is for running the benchmark tests inside a single go packagee.
# The target does not produce a file, so the developer can run it again an again.
# The target follows the naming scheme given by BENCHMARK_PACKAGE_TGTS
# (prefixed by BENCHMARK to namespace it slightly).
.PHONY: BENCHMARK/%
BENCHMARK/%: $(ALL_SRC) $(TEST_DEPS) $(FAUX_GENCODE) $(FAUXTEST) $(BENCH_DEPS)
	$(ECHO_V)cd $(FAUXROOT); $(TEST_ENV) \
	  $(GO) test  $(TEST_FLAGS) -bench=. -run=^$$ -benchmem $(BENCH_FLAGS) $*

.PHONY: $(COVERAGE_XML)
$(COVERAGE_XML): $(GOCOV) $(GOCOV_XML) $(COVERFILE)
	$(GOCOV) convert $(COVERFILE) | $(GOCOV_XML) | sed 's|filename=".*$(PROJECT_ROOT)/|filename="|' > $@

$(HTML_REPORT): $(GOCOV) $(GOCOV_HTML) $(COVERFILE)
	$(GOCOV) convert $(COVERFILE) | $(GOCOV_HTML) > $@

$(FAUXFILE):
	@mkdir -p $(_GOPATH)/src/$(dir $(PROJECT_ROOT))
	@ln -s $(CURDIR) $(_GOPATH)/src/$(PROJECT_ROOT)
	@touch $(FAUXFILE)

# All of the following are PHONY targets for convenience.

buns:
	@echo "(Assuming 'buns' is a British spelling of 'bins')"
	$(MAKE) bins

bins: $(PROGS) $(EXTRA_BINS) ## Build all binaries specified in the PROGS variable.

test:: $(GOCOV) $(COVERFILE) $(TEST_LOG) ## Runs tests for all subpackages.
	@$(GOCOV) convert $(COVERFILE) | $(GOCOV) report

test-xml: $(COVERAGE_XML) $(JUNIT_XML)

test-html: $(HTML_REPORT)

coverage-browse: $(COVERFILE)
	$(GO) tool cover -html=$(COVERFILE)

jenkins:: ## Helper rule for jenkins, which calls clean, glide deps, installs subpackages, lint, build bins and runs tests.
	-git clean -fd -f -x vendor
	$(MAKE) clean
	@# run this first because it can't be parallelized
ifeq ($(DEP_MANAGER),Glide)
	@$(MAKE) $(FAUXFILE) $(FAUX_VENDOR) GLIDE_ENV="$(DURABLE_GLIDE_ENV) $(NO_AUTOCREATE_GLIDE_ENV)"
else ifeq ($(DEP_MANAGER),Dep)
	@$(MAKE) $(FAUXFILE) $(FAUX_VENDOR) DEP_ENV="$(DURABLE_DEP_ENV) $(NO_AUTOCREATE_DEP_ENV)"
endif
	@# lint needs installed packages to avoid spurious errors
	$(MAKE) install-all
	$(MAKE) lint PHAB_COMMENT=.phabricator-comment
	$(MAKE) bins
	$(MAKE) $(JENKINS_PARALLEL_FLAG) -k \
		test-xml \
		RACE="-race" \
		TEST_VERBOSITY_FLAG="-v" \
		PHAB_COMMENT=.phabricator-comment
	$(MAKE) check-coverage PHAB_COMMENT=.phabricator-comment

# mysqlman jenkins cleanup
ifdef MYSQLMAN_DB_NAME
jenkins::
	$(MAKE) cleanup-mysqlman
endif

# schemaless/lessman jenkins cleanup
ifdef SCHEMALESS_INSTANCE
jenkins::
	$(MAKE) cleanup-lessman
endif

testhtml: test-html

testxml: test-xml

# bench is a convenience target for running all benchmark tests given in
# BENCHMARK_PACKAGE_TGTS
bench: $(BENCHMARK_PACKAGE_TGTS) ## Run benchmark tests for all subpackages.

service-names: ## Displays service names configured for this repository.
	@echo $(SERVICES)

thriftc: $(FAUX_THRIFT) ## Builds all generated code for Thrift.

protoc: $(FAUX_PROTOC) ## Builds all generated code for Protobuf.

clean:: ## Removes all build objects, including binaries, generated code, and test output files.
	@rm -rf $(PROGS) $(BUILD_DIR) *.html *.xml .phabricator-comment Godeps/_workspace/pkg Godeps/_workspace/bin
	@git clean -fdx $(THRIFT_GENDIR) 2>/dev/null || rm -rf $(THRIFT_GENDIR)
	@git clean -fdx $(PROTOC_GENDIR) 2>/dev/null || rm -rf $(PROTOC_GENDIR)

install-all: thriftc protoc $(PROGS) ## Installs all subpackages in this project as well as their dependencies.
	$(GO) install $(BUILD_FLAGS) $(ALL_PKGS)

# Skip the "possible formatting directive in Error call" error from testify
# due to https://code.uberinternal.com/T490457
LINT_SKIP_ERROR=grep -v -e "possible formatting directive in Error call"

# Create a pipeline filter for go vet/golint. Patterns specified in LINT_EXCLUDES are
# converted to a grep -v pipeline. If there are no filters, cat is used.
FILTER_LINT := $(if $(LINT_EXCLUDES), grep -v $(foreach file, $(LINT_EXCLUDES),-e $(file)),cat) | $(LINT_SKIP_ERROR)

vet: $(FAUXFILE) ## Runs all go code through "go vet".
	@# Skip the last line of the vet output if it contains "exit status"
	$(ECHO_V)cd $(FAUXROOT); $(GO) vet $(ALL_PKGS) 2>&1 | sed '/exit status 1/d' | $(FILTER_LINT) > $(VET_LOG) || true
	@[ ! -s "$(VET_LOG)" ] || (echo "Go Vet Failures" | cat - $(VET_LOG) | tee -a $(PHAB_COMMENT) && false)

golint: $(GOLINT) $(FAUXFILE) ## Runs all go code through "golint".
	@rm -f $(LINT_LOG)
	$(ECHO_V)cd $(FAUXROOT); echo $(ALL_PKGS) | xargs -x -L 1 -n 1 $(GOLINT) | $(FILTER_LINT) >> $(LINT_LOG) || true
	@[ ! -s "$(LINT_LOG)" ] || (echo "Lint Failures" | cat - $(LINT_LOG) | tee -a $(PHAB_COMMENT) && false)

gofmt: $(FAUXFILE) ## Ensures files are formatted using "gofmt".
	$(ECHO_V)$(GOFMT) -e -s -l $(ALL_SRC) | $(FILTER_LINT) > $(FMT_LOG) || true
	@[ ! -s "$(FMT_LOG)" ] || (echo "Go Fmt Failures, run 'make fmt'" | cat - $(FMT_LOG) | tee -a $(PHAB_COMMENT) && false)

ERRCHECK_IGNOREFILE = $(GOBUILD_DIR)/config/errcheck
ERRCHECK_FLAGS ?= -exclude $(ERRCHECK_IGNOREFILE) -ignoretests -blank

errcheck: $(ERRCHECK) $(FAUXFILE) ## Runs all go code through "errcheck".
	@rm -f $(ERRCHECK_LOG)
	$(ECHO_V)cd $(FAUXROOT); $(ERRCHECK) $(ERRCHECK_FLAGS) $(ALL_PKGS) | $(FILTER_LINT) >> $(ERRCHECK_LOG) || true;
	@[ ! -s "$(ERRCHECK_LOG)" ] || (echo "Errcheck Failures" | cat - $(ERRCHECK_LOG) | tee -a $(PHAB_COMMENT) && false)

unused: $(UNUSED) $(FAUXFILE) ## Runs all go code through "unused".
	@rm -f $(UNUSED_LOG)
	$(ECHO_V)cd $(FAUXROOT); $(UNUSED) $(UNUSED_FLAGS) $(ALL_PKGS) | $(FILTER_LINT) >> $(UNUSED_LOG) || true;
	@[ ! -s "$(UNUSED_LOG)" ] || (echo "Unused Failures" | cat - $(UNUSED_LOG) | tee -a $(PHAB_COMMENT) && false)

STATICCHECK_FLAGS ?=

staticcheck: $(STATICCHECK) $(FAUXFILE) ## Runs all go code through "staticcheck".
	@rm -f $(STATICCHECK_LOG)
	$(ECHO_V)cd $(FAUXROOT);$(STATICCHECK) $(STATICCHECK_FLAGS) $(ALL_PKGS) | $(FILTER_LINT) >> $(STATICCHECK_LOG) || true;
	@[ ! -s "$(STATICCHECK_LOG)" ] || (echo "Staticcheck Failures" | cat - $(STATICCHECK_LOG) | tee -a $(PHAB_COMMENT) && false)

# errcheck, staticcheck and unused are not here as they was added later into go-build
# and we didn't want to break existing projects, to add in your own project, do:
# 	lint:: errcheck staticcheck unused
lint:: vet golint gofmt ## Executes an assortment of linters.

check-coverage: $(COVERAGE_XML) ## Checks that all packages have tests, and meet a coverage bar.
	@rm -f $(CHECK_COVERAGE_LOG)
	$(ECHO_V)python $(GOBUILD_DIR)/check_coverage.py --cover_ignore_srcs="$(COVER_IGNORE_SRCS)" 2>> $(CHECK_COVERAGE_LOG) || true;
	@[ ! -s "$(CHECK_COVERAGE_LOG)" ] || (echo "Coverage Failures" | cat - $(CHECK_COVERAGE_LOG) | tee -a $(PHAB_COMMENT) && false)

# We only want to format the files that are not ignored by FILTER_LINT.
FMT_SRC:=$(shell echo "$(ALL_SRC)" | tr ' ' '\n' | $(FILTER_LINT))
fmt: ## Runs "gofmt $(FMT_FLAGS) -w" to reformat all Go files.
	gofmt $(FMT_FLAGS) -w $(FMT_SRC)

help: ## Prints a help message that shows rules and any comments for rules.
	@cat $(MAKEFILE_LIST) | grep -e "^[a-zA-Z_\-]*: *.*## *" | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'
	@echo
	@echo "Useful Variables:"
	@echo "  V    enable a more verbose build, when you want to see how the sausage is made."
	@echo
	@echo "Among other methods, you can pass these variables on the command line, e.g.:"
	@echo "    make V=1"


.DEFAULT_GOAL = all
.PHONY: \
	all \
	bench \
	bins \
	buns \
	coverage-browse \
	clean \
	errcheck \
	fmt \
	gofmt \
	golint \
	help \
	install-all \
	jenkins \
	lint \
	protoc \
	service-names \
	staticcheck \
	test \
	test-html \
	testhtml \
	test-xml \
	testxml \
	thriftc \
	unused \
	vendor \
	vet
