# Glide dependency management specific makefile rules

GLIDE_YAML := $(CURDIR)/glide.yaml
GLIDE_LOCK := $(CURDIR)/glide.lock

# This enables gopkg.in to pull straight from gitolite (without respecting
# version tags). It should only be used in Jenkins/uBuild where we already know
# that a glide.lock exists, so we don't need the flag.
DURABLE_GLIDE_ENV = UBER_GOPKG_FROM_GITOLITE=yes
NO_AUTOCREATE_GLIDE_ENV = UBER_NO_GITOLITE_AUTOCREATE=yes

# Add --no-color to glide when running under an uncolored terminal.
# Only necessary until https://github.com/Masterminds/glide/issues/358
# is resolved upstream
ifeq ($(TERMINAL_HAS_COLORS),1)
  GLIDE_NO_COLOR ?=
else
  GLIDE_NO_COLOR ?= --no-color
endif

ifeq ($(IS_UBUILD),1)
  GLIDE_ENV ?= $(DURABLE_GLIDE_ENV)
endif

# Always set this flag to make builds faster and hammer gitolite less.
# The only time repos are autocreated now is when somebody runs "go-build/glide get ..."
# or "go-build/glide up".
GLIDE_ENV += $(NO_AUTOCREATE_GLIDE_ENV)
