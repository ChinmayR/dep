# Magnus Sirupsen, the maintainer of logrus, changed his GitHub username a while ago. This had the
# intended effect of changing the import path for logrus from `github.com/Sirupsen/logrus` to
# `github.com/sirupsen/logrus`. As the Go community updates packages, this leads to import path
# conflicts - some of your dependencies want `Sirupsen/logrus`, and others want `sirupsen/logrus`.
#
# Including this make fragment after including rules.mk will fix vendor imports to match what's currently published on github.

FAUX_VENDOR_REWRITE := $(BUILD_DIR)/.faux.vendor.rewrite
$(FAUX_VENDOR_REWRITE):
	@echo Rewriting obsolete logrus imports...
	@find vendor/ -name '*.go' | xargs gofmt -w -r '"github.com/Sirupsen/logrus" -> "github.com/sirupsen/logrus"' || true
	@touch $@

$(FAUX_VENDOR):: $(FAUX_VENDOR_REWRITE)
