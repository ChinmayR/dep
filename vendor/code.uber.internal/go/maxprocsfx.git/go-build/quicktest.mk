# Quicktest makefile.
# Questions?  Comments?  Ideas?  Problems?  Ping @stevenl!
#
# In a nutshell, this tries to detect changed files -> rerun only related tests.
# It's not intended to be super reliable or super intelligent about doing so, but if
# you find it useful like 90% of the time, its goal has been achieved!
#
# To control, there are two major options:
# - V=1 to show debugging output
# - QT_BASE='origin/master' (or any other `git diff`-able string) to control the diff-base.
#   The default is 'HEAD^' because it has been a good base for the Python quicktest implementation,
#   which can be found here (and it can be broken out of supply_gateways, it just hasn't needed it yet):
#   https://code.uberinternal.com/diffusion/SUSUPP/browse/master/supply_lib/fabric/quicktest.py

# Git tree-like to diff against to select tests.
# Should work with SHAs, ranges, branches, anything.
QT_BASE ?= HEAD^

# This piece:
# - finds changed files since the diff-base
# - finds their unique parent directories
# - removes the top-level "./" dir
# - converts "the_dir/" to "./the_dir/" because that's how TEST_DIRS looks / go-build works
QT_CHANGED_DIRS = $(addprefix ./,$(filter-out ./,$(sort $(dir $(shell git diff '$(QT_BASE)' --name-only)))))

# Retains only changed-dirs that are also test-dirs, to prevent "testing" e.g. docs.
QT_TEST_DIRS = $(filter $(TEST_DIRS),$(QT_CHANGED_DIRS))

# V=1 isn't particularly useful with such large commands, so here's a helper.
.PHONY: _qt_debug
_qt_debug:
	@echo 'quicktest debug:'
	@echo '	Diff against $(QT_BASE): $(shell git diff $(QT_BASE) --name-only)'
	@echo '	TEST_DIRS: $(TEST_DIRS)'
	@echo '	Changed dirs: $(QT_CHANGED_DIRS)'
	@echo '	Dirs to test: $(QT_TEST_DIRS)'

# Quicktest implementation is a bit ugly right now, but serves as a useful proof-of-concept at the very least.
# - First line makes sure we have detected tests, otherwise `make test` just hangs.
# - Lines 2-3 print out what it found, which has proven useful in the Python version, as otherwise it can be hard to
#   understand if it's doing what you expect it to or not.
.PHONY: qt
qt: $(if $(filter-out 0,$(V)),_qt_debug)  ## Run tests only on changed files.  Set QT_BASE to change the relative commit to diff against.
	$(ECHO_V) $(eval QT_CACHE := $$(QT_TEST_DIRS))
	$(ECHO_V) \
	if test -z '$(QT_CACHE)'; then \
		echo 'No test files detected, use V=1 to debug if this seems wrong'; \
	else \
		echo 'Found packages:'; \
		echo '$(QT_CACHE)' | tr ' ' '\n' | sed -e 's/^/   /'; \
		$(MAKE) test TEST_DIRS='$(QT_CACHE)'; \
	fi
