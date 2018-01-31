PROJECT_ROOT = code.uber.internal/engsec/galileo-go.git

# define the list of thrift files the service depends on
# (if you have some)
#THRIFT_SRCS = idl/code.uber.internal/engsec/foober/foober.thrift

LINT_EXCLUDES = .tmp vendor

GALILEO_FILES = $(wildcard *.go) $(wildcard galileotest/*.go) \
				$(wildcard internal/*/*.go) \
				$(wildcard internal/*.go)

lint:: errcheck staticcheck unused

# list all executables
PROGS =
# samples/client/galileo-client \
#  samples/server/galileo-server \
#   samples/personnel/personnel # \
#	samples/gurl/gurl

# samples/client/galileo-client: $(GALILEO_FILES) $(wildcard samples/client/*.go)
# samples/server/galileo-server: $(GALILEO_FILES) $(wildcard samples/server/*.go)
# samples/gurl/gurl: $(GALILEO_FILES) $(wildcard samples/gurl/*.go)
# samples/personnel/personnel: $(GALILEO_FILES) $(wildcard samples/personnel/*.go)

-include go-build/rules.mk

go-build/rules.mk:
	git submodule update --init

# See README.md for release process and instructions.
release: jenkins
	./scripts/release
