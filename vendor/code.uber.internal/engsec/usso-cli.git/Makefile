PROJECT_ROOT = code.uber.internal/engsec/usso-cli.git

PROGS = usso
SERVICES = usso-cli

THRIFTRW_PLUGINS = go.uber.org/yarpc/encoding/thrift/thriftrw-plugin-yarpc
THRIFTRW_SRCS = $(shell find idl -name \*.thrift)

# Modifying any non-vendor go files will rebuild PROGS
usso: $(shell find . -not -path "./vendor/*" -name "*.go") Gopkg.lock

-include go-build/rules.mk

lint:: errcheck staticcheck unused

go-build/rules.mk:
	git submodule update --init

.PHONY: run
run: bins
	@./usso
