# Go Build Rules and Tools #

This repository contains all the tools and Makefiles you should need (aside
from the Go tool chain itself) to build services written in Go for Linux and
OS X.  The goals of this repository are to provide a common set of versioned
build tools that everyone can use as well as build infrastructure to make
building reliable and repeatable.

## What does this provide? ##

include.mk provides a GNU Make variable for precompiled binaries (such as
thrift and thrift-gen) to use in your own makefiles.

The rules.mk Makefile, however, provides much much more.  If you include the
rules.mk Makefile at the bottom of your Makefile, you will reap the following
benefits:

- Automatic rules for generating Go code from Thrift IDL
- Parallelized build rules for Thrift, service binaries, and Tests
- Sandboxed builds that only look for Go package dependencies in Godeps
- A build infrastructure that can run the same locally as it does on Jenkins.
- No additional shell scripts needed.

The repository is laid out as follows:

    linux-x86_64/    # executables for Linux
      ./thrift
      ./thrift-gen
      ./gocov
      ...
    darwin-x86_64/   # executables for OS X
      ./thrift
      ./thrift-gen
      ./gocov
      ...
    Makefile         # GNU Make rules to install new build tools
    include.mk       # GNU Make variables for the build tools (i.e. $(THRIFT))
    rules.mk         # GNU Make build rules for go services.
    <symlinks>       # Symbolic links to run the binary for the current pfm.

## Great!  I'm sold on this rules.mk thing.  How do I use it? ##

For an existing service, you need to add go-build as a submodule of your
project:

    cd <to_your_project>
    git submodule add -b master gitolite@code.uber.internal:go-build go-build/
    git submodule update --init

After you've done that, you're going to need to edit your Makefile to use
rules.mk.  Depending on how much customization you've made, this may be a bit
involved.  For the common case, it should be fairly trivial.  Your Makefile
should look something like this when you have finished:

```lang=make
# Flags to pass to go build
BUILD_FLAGS =

# Environment variables to set before go build
BUILD_ENV=

# Flags to pass to go test
TEST_FLAGS =

# Extra dependencies that the tests use
TEST_DEPS =

# regex of files which will be removed from test coverage numbers (eg. mock object files)
COVER_IGNORE_SRCS = _(mock|string).go

# Where to find your project
PROJECT_ROOT = code.uber.internal/path/to/project

# Tells udeploy what your service name is (set to $(notdir of PROJECT_ROOT))
# by default
# SERVICES = project

# define the list of thrift files the service depends on
# (if you have some)
THRIFT_SRCS = idl/code.uber.internal/rt/service1/service1.thrift \
		idl/code.uber.internal/infra/service2/service2.thrift

# define thrift file of the service if you want to use thriftsync target. more: t.uber.com/thriftsync
SERVICE_IDLS = idl/code.uber.internal/rt/service1/service1.thrift

# list all executables
PROGS = myservice1 example/client

# define which go files cause a rebuild of which binary
myservice1: appcfg/config.go handler.go main.go
example/client: appcfg/config.go $(wildcard client/*.go)

include go-build/rules.mk

go-build/rules.mk:
	git submodule update --init
```

You should then test your Makefile to make sure it works.  The following
targets have already been defined for you:

- bins: builds everything listed in the PROGS variable
- jenkins: runs the tests, and the lint rules
- test: runs the tests
- test-xml: runs the tests, and outputs an XML test coverage report
- test-html: runs the tests, and outputs an HTML test coverage report
- coverage-browse: runs the tests with coverage, then opens an HTML test coverage report in your browser
- testxml: alias for test-xml
- testhtml: alias for test-html
- thriftc: generates code for all .thrift files
- clean: delete all the files created as part of the build
- lint: run the various linters on your code

It's possible that your service may need to set environment variables or
compile/test flags to the various build steps.  Use the BUILD_FLAGS,
BUILD_ENV, TEST_FLAGS, and TEST_ENV variables to do this.

After you have tested that things build correctly, you should remove all build
scripts in .jenkins from your repository and modify your Jenkins project to use
make instead of those scripts.  For most services, you can set the build step
to:

    make jenkins

You will also need to tell Jenkins to recursively update all submodules in
your project.  You do this by going to your project and into
Configure > Source Control Management > Additional Behaviors > Advanced
sub-module behaviors and click both recursively update submodules and update
tracking submodules to tip of branch.  Don't forget to apply your changes!

Finally, if you have Arcanist configured to run tests before committing, you
need to update your .arcconfig to set the unit test command appropriately:

    "unit.golang.command": "make clean && make -j5 test-xml JUNIT_XML=%s COVERAGE_XML=%s"

Congratulations!  You've now completed the transition and everything should
work!

## Exclude files from the linter ##

You can specify regex patterns to exclude from lint using the `LINT_EXCLUDES`
variable. See [this example](https://code.uberinternal.com/diffusion/GOBUIL/browse/master/pass/lint_exclude/)
for how you can exclude specific files.

## I don't want it to work!  Get me out of here! ##

If rules.mk doesn't work for you, at least include the include.mk file and
start using the build tool variables to make use of the versioned build
binaries that are part of the repository.

## This works great!  I need more rules though! ##

Great!  Well, the good news is that your Makefile is still just that: a
Makefile.  You can add additional rules as you see fit.  You can also extend
existing rules by making them depend on something you add.
