PROJECT_ROOT = code.uber.internal/go/health.git
LINT_EXCLUDES = internal/meta

-include go-build/rules.mk

lint:: staticcheck unused

go-build/rules.mk:
	git submodule update --init
