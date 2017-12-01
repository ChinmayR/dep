`glide-exec` integrates with glide to provide support for building and running
vendored executables. The problem it attempts to alleviate is this: Your
project vendors a specific version of a dependency which provides an executable
you use while developing but you have a different (possibly incompatible)
version of that executable installed globally on your system. Rather than
overwriting your replacing installed version, with `glide-exec`, you can simply
do,

    glide exec -d mybin -x import/path/of/executable executable -flag1 arg2

This will build the executable at the given import path if necessary, store it
in `mybin` along with versioning information, and run the given command with
the given arguments. If the version of that package changes in `glide.lock`,
the executable will be rebuilt.

Any number of executables may be passed in using the `-x` flag. All of them
will be available on `$PATH` to the given command.
