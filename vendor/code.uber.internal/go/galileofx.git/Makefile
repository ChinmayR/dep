PROJECT_ROOT = code.uber.internal/go/galileofx.git
PACKAGES = $(shell go list ./... | grep -v '/\(vendor\|go-build\)/')

LINT_EXCLUDES = mock.*.go

-include go-build/rules.mk

lint:: staticcheck unused

go-build/rules.mk:
	git submodule update --init
