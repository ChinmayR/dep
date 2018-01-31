# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](http://keepachangelog.com/en/1.0.0/)
and this project adheres to [Semantic Versioning](http://semver.org/spec/v2.0.0.html).

## v1.3.0 - 2018-01-08
### Changed
- Use httpfx 2.0 for the HTTP server.
- Rename log field `port` to `addr` to avoid type conflicts.

## v1.2.0 - 2017-11-06
### Added
- Add canonical import path directive. This will provide better error messages
  if incorrect import paths are used to import the package.

## v1.1.0 - 2017-08-23
### Changed
- Alter `Module` to always start the systemport server.
- Warn instead of failing when serving on an ephemeral port in production.

## v1.0.1 - 2017-08-02
### Changed
- Improve error handling in lifecycle hooks.

## v1.0.0 - 2017-07-31

- No changes.

## v1.0.0-rc2 - 2017-07-21
### Added
- Export constructor as `New`.
- Update lifecycle hooks to satisfy new Fx APIs.

## v1.0.0-rc1 - 2017-06-21
### Added
- First public release.
