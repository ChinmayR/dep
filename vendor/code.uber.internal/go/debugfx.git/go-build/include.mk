# Projects that only want to use pre-compiled binaries from
# go-build should add the following to the top of their Makefile:
#     include ./gobuild/include.mk
# They can use variables to refer to binaries (e.g. $(THRIFT)).

# Get the path of the directory containing the include.mk
GO_BUILD_ROOT := $(dir $(lastword $(MAKEFILE_LIST)))
BINARIES_DIR:=$(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))

# Update the PATH so when thrift-gen runs thrift, it uses
# the precompiled binary.
OLDPATH := $(PATH)
export PATH=$(GO_BUILD_ROOT):$(OLDPATH)

# Make the directory relative (for better output when running commands).
BINARIES_DIR:=$(BINARIES_DIR:$(CURDIR)%=.%)

# Constants that the user cannot override.
GO            := go
GOFMT         := gofmt
VENDOR_DIR    := vendor

include $(dir $(lastword $(MAKEFILE_LIST)))/verbosity.mk

# Go binaries are built using deps.mk
include $(dir $(lastword $(MAKEFILE_LIST)))/deps.mk

# Semi-clever quicker test runner
include $(dir $(lastword $(MAKEFILE_LIST)))/quicktest.mk
