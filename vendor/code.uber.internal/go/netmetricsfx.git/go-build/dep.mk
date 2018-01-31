# Dep dependency management specific makefile rules

DEP_TOML := $(CURDIR)/Gopkg.toml
DEP_LOCK := $(CURDIR)/Gopkg.lock

# This enables gopkg.in to pull straight from gitolite (without respecting
# version tags). It should only be used in Jenkins/uBuild where we already know
# that a dep lock (Gopkg.lock) exists, so we don't need the flag.
DURABLE_DEP_ENV = UBER_GOPKG_FROM_GITOLITE=yes
NO_AUTOCREATE_DEP_ENV = UBER_NO_GITOLITE_AUTOCREATE=yes

ifeq ($(IS_UBUILD),1)
  DEP_ENV ?= $(DURABLE_DEP_ENV)
endif

# Always set this flag to make builds faster and hammer gitolite less.
DEP_ENV += $(NO_AUTOCREATE_DEP_ENV)
