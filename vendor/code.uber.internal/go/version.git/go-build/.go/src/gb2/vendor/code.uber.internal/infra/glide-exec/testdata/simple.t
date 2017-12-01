Set up

  $ set -e
  $ cp -R $TESTDIR/$(basename $TESTFILE .t)/ .
  $ export GOPATH=$(pwd)
  $ GLIDE_EXEC="$(dirname $TESTDIR)/glide-exec --no-color"

We should support plain vendored executables with no dependencies.

  $ cd src/t
  $ mkdir mybin

  $ $GLIDE_EXEC -d mybin -x helloworld helloworld
  hello world

  $ cat mybin/.helloworld-version
  something (no-eol)

  $ $GLIDE_EXEC -d mybin -x helloworld helloworld
  hello world
