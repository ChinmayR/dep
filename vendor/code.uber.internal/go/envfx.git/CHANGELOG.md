# Changelog

## v1.3.0 (2017-12-11)

- Add an IsMesos method to more easily check whether the process is running on
  the compute cluster.

## v1.2.1 (2017-11-15)

- Fix LookupEnv wrapper to handle unset environment variables properly.

## v1.2.0 (2017-11-15)

- Add canonical import path directive. This will provide better error messages
  if incorrect import paths are used to import envfx.
- Add LookupEnv wrapper for os.LookupEnv. See docs for more information.

## v1.1.0 (2017-08-13)

- Add `RuntimeEnvironment`.

## v1.0.0 (2017-07-31)

- No changes.

## v1.0.0-rc4 (2017-07-21)

- Add `ApplicationID`, `Pipeline`, `Cluster`, and `Pod` to `Context`.

## v1.0.0-rc3 (2017-07-19)

- Fix errors for reading values from a Puppet-managed file.

## v1.0.0-rc2 (2017-07-11)

- Export constructor as `New`.

## v1.0.0-rc1 (2017-06-20)

- First public release.
