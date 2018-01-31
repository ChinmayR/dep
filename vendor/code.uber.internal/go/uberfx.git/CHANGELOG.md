# Changelog

## v1.2.0 (2018-01-08)

- Add canonical import path directive. This will provide better error messages
  if incorrect import paths are used to import uberfx.
- Add `netmetricsfx`, a telemetry client designed for the software networking
  team.

## v1.1.0 (2017-09-20)

- Add `galileofx`, which automatically adds support for authenticated RPCs.
- Add `debugfx`, which automatically registers profiling handlers on the
  systemport server.
- Add `maxprocsfx`, which automatically adjusts `GOMAXPROCS` to match any
  configured Linux CPU quota.

## v1.0.0 (2017-08-01)

- First stable release: no breaking changes will be made in the 1.x series.

## v1.0.0-rc4 (2017-07-21)

- Include runtimefx for Go runtime metrics
- Upgrade all other modules to latest release candidates

## v1.0.0-rc3 (2017-07-06)

- Use fx.Option from configfx

## v1.0.0-rc2 (2017-07-06)

- Initial re-branding from `go/fxstack`
