# Changelog

## v1.3.0 (2017-11-15)

- Add canonical import path directive. This will provide better error messages
  if incorrect import paths are used to import configfx.
- Use the lookup function provided by envfx instead of os.LookupEnv. This
  makes interpolating the Uber environment, zone, and pipeline work correctly
  for processes that don't have environment variables managed by uDeploy.

## v1.2.0 (2017-10-23)

- Improve error message when no configuration files are found.
- Allow customization of config file loading via meta.yaml configuration file.

## v1.1.0 (2017-09-06)

- Use service runtime environment when loading YAML configuration files with
  fallback to the host environment.

## v1.0.2 (2017-08-18)

- Fix a bug in file loading that ignored the supplied environment variable
  lookup function.

## v1.0.1 (2017-08-01)

- Don't error on nonexistent files.

## v1.0.0 (2017-07-31)

- Use load package to get config.

## v1.0.0-rc2 (2017-07-21)

- Export constructor as `New`.

## v1.0.0-rc1 (2017-06-20)

- First public release.
