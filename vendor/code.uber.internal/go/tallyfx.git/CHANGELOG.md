# Changelog

## v1.2.2 (2017-11-15)

- Release off master branch.

## v1.2.1 (2017-11-15)

- Bump max queue size to 4096 to match Go tally.

## v1.2.0 (2017-11-06)

- Add canonical import path directive. This will provide better error messages
  if incorrect import paths are used to import the package.

## v1.1.2 (2017-09-25)

- Rename runtime environment tag to runtime_env to match
  "Staging in prod" RFC.

## v1.1.1 (2017-09-21)

- Fix a panic in configuration parsing.

## v1.1.0 (2017-09-20)

- Automatically tag all metrics with the runtime environment.

## v1.0.0 (2017-07-31)

- No changes.

## v1.0.0-rc2 (2017-07-21)

- Export constructor as `New`.
- Update lifecycle hooks to satisfy new Fx APIs.

## v1.0.0-rc1 (2017-06-21)

- First public release.
