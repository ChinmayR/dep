Changelog
=========

v1.0.1 (22 Jun 2017)
--------------------

- When `BuildHash` isn't set at build time, fall back to `$GIT_DESCRIBE`
  instead of `$GIT_REF`. This matches `go-build`'s logic and pulls in a richer
  set of version information.

v1.0.0 (21 Jun 2017)
--------------------

- Initial stable release.
