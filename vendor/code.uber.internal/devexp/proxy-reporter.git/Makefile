PROJECT_ROOT = code.uber.internal/devexp/proxy-reporter.git

# List of executables supplied by your project.
PROGS = proxy-reporter

# Modifying any non-vendor go files will rebuild PROGS
proxy-reporter: $(shell find . -not -path "./vendor/*" -name "*.go") glide.lock

-include go-build/rules.mk

lint:: errcheck staticcheck unused

go-build/rules.mk:
	git submodule update --init

.PHONY: run
run: bins
	@./proxy-reporter
