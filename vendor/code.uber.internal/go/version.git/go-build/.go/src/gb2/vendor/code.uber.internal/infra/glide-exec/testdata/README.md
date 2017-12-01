Tests are driven by [cram](https://bitheap.org/cram/). For each test,

-   Create a `$testname.t` file. This defines the behavior of the test.
    Document it adequately so it's obvious what is happening.
-   Create a directory `$testname` with the data for the test. Most tests will
    just copy the contents of the directory as-is and treat that as the
    `GOPATH`.
-   If you need to use `glide`, pass `--no-color` to keep the output readable
    inside the `.t` file. Also, remember to set `GLIDE_HOME` to a temporary
    directory so that the global glide cache is not used.

Setup for most tests will be:

```
  $ set -e
  $ cp -R $TESTDIR/$(basename $TESTFILE .t)/ .
  $ export GOPATH=$(pwd)
  $ GLIDE_EXEC="$(dirname $TESTDIR)/glide-exec --no-color"
```

Use `$GLIDE_EXEC` to call the `glide-exec` tool.

If the test will call `glide`, also add:

```
  $ mkdir .glide-home
  $ export GLIDE_HOME=$(pwd)/.glide-home
  $ GLIDE="glide --no-color"
```

Use `$GLIDE` to call `glide` directly.
