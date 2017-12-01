Set up

  $ set -e
  $ cp -R $TESTDIR/$(basename $TESTFILE .t)/ .
  $ export GOPATH=$(pwd)
  $ GLIDE_EXEC="$(dirname $TESTDIR)/glide-exec --no-color"

  $ mkdir .glide-home
  $ export GLIDE_HOME=$(pwd)/.glide-home

We should support vendored executables that have their own glide.lock and
dependencies.

  $ cd src/t
  $ mkdir mybin

  $ $GLIDE_EXEC -d mybin -x myorg/helloworldwithflags/cmd/helloworld helloworld -m derp
  [WARN]	Lock file may be out of date. Hash check of YAML failed. You may need to run 'update'
  [INFO]	Downloading dependencies. Please wait...
  [INFO]	--> Fetching github.com/jessevdk/go-flags.
  [UBER]  Gitolite GitHub mirror gitolite@code.uber.internal:github/jessevdk/go-flags already exists
  [UBER]  Rewrite github.com https://github.com/jessevdk/go-flags to gitolite@code.uber.internal:github/jessevdk/go-flags
  [INFO]	Setting references.
  [INFO]	--> Setting version for github.com/jessevdk/go-flags to a8cab0163d48558ffd77076c9c99388529766f63.
  [INFO]	Exporting resolved dependencies...
  [INFO]	--> Exporting github.com/jessevdk/go-flags
  [INFO]	Replacing existing vendor dependencies
  derp

  $ $GLIDE_EXEC -d mybin -x myorg/helloworldwithflags/cmd/helloworld helloworld -m derp
  derp

  $ ( ls vendor/myorg/helloworldwithflags/vendor 2>/dev/null && \
  > echo "Vendored package must not have its own vendor directory" && \
  > exit 1 ) || true

  $ cat mybin/.helloworld-version
  somethingelse (no-eol)

Change the version and run again.

  $ sed -i.bak -e 's/version: somethingelse/version: newversion/' glide.lock
  $ $GLIDE_EXEC -d mybin -x myorg/helloworldwithflags/cmd/helloworld helloworld -m derp
  [WARN]	Lock file may be out of date. Hash check of YAML failed. You may need to run 'update'
  [INFO]	Downloading dependencies. Please wait...
  [INFO]	--> Found desired version locally github.com/jessevdk/go-flags a8cab0163d48558ffd77076c9c99388529766f63!
  [INFO]	Setting references.
  [INFO]	--> Setting version for github.com/jessevdk/go-flags to a8cab0163d48558ffd77076c9c99388529766f63.
  [INFO]	Exporting resolved dependencies...
  [INFO]	--> Exporting github.com/jessevdk/go-flags
  [INFO]	Replacing existing vendor dependencies
  derp

  $ $GLIDE_EXEC -d mybin -x myorg/helloworldwithflags/cmd/helloworld helloworld -m derp
  derp
